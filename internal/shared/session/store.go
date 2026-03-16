package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hms/go-backend/internal/config"

	redisdriver "github.com/redis/go-redis/v9"
)

const (
	sessionMetaPrefix = "session:meta:v1"
	userSessionsPrefix = "session:user:v1"
)

type HostelSummary struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type UserSnapshot struct {
	ID         string         `json:"_id"`
	Email      string         `json:"email"`
	Role       string         `json:"role"`
	SubRole    *string        `json:"subRole"`
	Authz      map[string]any `json:"authz"`
	Hostel     *HostelSummary `json:"hostel"`
	PinnedTabs []string       `json:"pinnedTabs"`
}

type Record struct {
	Cookie   SessionCookie `json:"cookie"`
	UserID   string        `json:"userId"`
	UserData UserSnapshot  `json:"userData"`
	Role     string        `json:"role"`
	Email    string        `json:"email"`
}

type SessionCookie struct {
	OriginalMaxAge int64  `json:"originalMaxAge"`
	Expires        string `json:"expires"`
	Secure         bool   `json:"secure"`
	HTTPOnly       bool   `json:"httpOnly"`
	Path           string `json:"path"`
	SameSite       string `json:"sameSite,omitempty"`
}

type DeviceMetadata struct {
	UserID     string    `json:"userId"`
	SessionID  string    `json:"sessionId"`
	UserAgent  string    `json:"userAgent"`
	IP         string    `json:"ip"`
	DeviceName string    `json:"deviceName"`
	LoginTime  time.Time `json:"loginTime"`
	LastActive time.Time `json:"lastActive"`
	IsCurrent  bool      `json:"isCurrent,omitempty"`
}

type Manager struct {
	client *redisdriver.Client
	cfg    config.SessionConfig
}

func NewManager(client *redisdriver.Client, cfg config.SessionConfig) *Manager {
	return &Manager{client: client, cfg: cfg}
}

func (m *Manager) Create(ctx context.Context, record Record, meta DeviceMetadata) (string, error) {
	sessionID, err := generateID()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	record.Cookie = m.newSessionCookie(now)
	meta.SessionID = sessionID
	meta.UserID = record.UserID
	meta.LoginTime = now
	meta.LastActive = now

	if err := m.Save(ctx, sessionID, record); err != nil {
		return "", err
	}

	if err := m.saveMeta(ctx, meta); err != nil {
		return "", err
	}

	return sessionID, nil
}

func (m *Manager) Save(ctx context.Context, sessionID string, record Record) error {
	record.Cookie = m.newSessionCookie(time.Now().UTC())

	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := m.client.Set(ctx, m.sessionKey(sessionID), payload, m.cfg.TTL).Err(); err != nil {
		return fmt.Errorf("persist session: %w", err)
	}

	return nil
}

func (m *Manager) Get(ctx context.Context, sessionID string) (*Record, error) {
	raw, err := m.client.Get(ctx, m.sessionKey(sessionID)).Result()
	if errors.Is(err, redisdriver.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	var record Record
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}

	return &record, nil
}

func (m *Manager) Exists(ctx context.Context, sessionID string) (bool, error) {
	count, err := m.client.Exists(ctx, m.sessionKey(sessionID)).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (m *Manager) Touch(ctx context.Context, sessionID, userID string, at time.Time) error {
	record, err := m.Get(ctx, sessionID)
	if err != nil {
		return err
	}

	if record != nil {
		record.Cookie = m.newSessionCookie(at)

		payload, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return fmt.Errorf("marshal touched session: %w", marshalErr)
		}

		if err := m.client.Set(ctx, m.sessionKey(sessionID), payload, m.cfg.TTL).Err(); err != nil {
			return err
		}
	}

	pipe := m.client.TxPipeline()
	pipe.HSet(ctx, m.metaKey(sessionID), "lastActive", at.UTC().Format(time.RFC3339))
	pipe.Expire(ctx, m.metaKey(sessionID), m.cfg.TTL)
	pipe.ZAdd(ctx, m.userSessionsKey(userID), redisdriver.Z{
		Score:  float64(at.UTC().UnixMilli()),
		Member: sessionID,
	})
	pipe.Expire(ctx, m.userSessionsKey(userID), m.cfg.TTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (m *Manager) Delete(ctx context.Context, sessionID, userID string) error {
	pipe := m.client.TxPipeline()
	pipe.Del(ctx, m.sessionKey(sessionID))
	pipe.Del(ctx, m.metaKey(sessionID))
	if userID != "" {
		pipe.ZRem(ctx, m.userSessionsKey(userID), sessionID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (m *Manager) ListUserSessions(ctx context.Context, userID string) ([]DeviceMetadata, error) {
	sessionIDs, err := m.client.ZRevRange(ctx, m.userSessionsKey(userID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("list user sessions: %w", err)
	}

	if len(sessionIDs) == 0 {
		return []DeviceMetadata{}, nil
	}

	pipe := m.client.Pipeline()
	results := make([]*redisdriver.MapStringStringCmd, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		results = append(results, pipe.HGetAll(ctx, m.metaKey(sessionID)))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redisdriver.Nil) {
		return nil, fmt.Errorf("fetch session metadata: %w", err)
	}

	sessions := make([]DeviceMetadata, 0, len(sessionIDs))
	for index, result := range results {
		row := result.Val()
		if len(row) == 0 {
			continue
		}

		sessions = append(sessions, DeviceMetadata{
			UserID:     row["userId"],
			SessionID:  sessionIDs[index],
			UserAgent:  row["userAgent"],
			IP:         row["ip"],
			DeviceName: fallbackString(row["deviceName"], "Unknown device"),
			LoginTime:  parseTime(row["loginTime"]),
			LastActive: parseTime(row["lastActive"]),
		})
	}

	return sessions, nil
}

func (m *Manager) WriteCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    signSessionID(sessionID, m.cfg.Secret),
		Path:     "/",
		Domain:   m.cfg.CookieDomain,
		MaxAge:   int(m.cfg.TTL.Seconds()),
		HttpOnly: true,
		Secure:   m.cfg.Secure,
		SameSite: m.cfg.SameSite,
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   m.cfg.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.cfg.Secure,
		SameSite: m.cfg.SameSite,
	})
}

func (m *Manager) ReadSessionID(r *http.Request) (string, error) {
	cookie, err := r.Cookie(m.cfg.CookieName)
	if err != nil {
		return "", err
	}

	sessionID, err := unsignSessionID(cookie.Value, m.cfg.Secret)
	if err != nil {
		return "", err
	}

	return sessionID, nil
}

func (m *Manager) saveMeta(ctx context.Context, meta DeviceMetadata) error {
	pipe := m.client.TxPipeline()
	pipe.HSet(ctx, m.metaKey(meta.SessionID), map[string]any{
		"userId":     meta.UserID,
		"sessionId":  meta.SessionID,
		"userAgent":  meta.UserAgent,
		"ip":         meta.IP,
		"deviceName": meta.DeviceName,
		"loginTime":  meta.LoginTime.UTC().Format(time.RFC3339),
		"lastActive": meta.LastActive.UTC().Format(time.RFC3339),
	})
	pipe.Expire(ctx, m.metaKey(meta.SessionID), m.cfg.TTL)
	pipe.ZAdd(ctx, m.userSessionsKey(meta.UserID), redisdriver.Z{
		Score:  float64(meta.LastActive.UTC().UnixMilli()),
		Member: meta.SessionID,
	})
	pipe.Expire(ctx, m.userSessionsKey(meta.UserID), m.cfg.TTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (m *Manager) sessionKey(sessionID string) string {
	return m.cfg.Prefix + sessionID
}

func (m *Manager) metaKey(sessionID string) string {
	return fmt.Sprintf("%s:%s", sessionMetaPrefix, sessionID)
}

func (m *Manager) userSessionsKey(userID string) string {
	return fmt.Sprintf("%s:%s", userSessionsPrefix, userID)
}

func generateID() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	return hex.EncodeToString(buffer), nil
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}

	return parsed.UTC()
}

func fallbackString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (m *Manager) newSessionCookie(now time.Time) SessionCookie {
	return SessionCookie{
		OriginalMaxAge: m.cfg.TTL.Milliseconds(),
		Expires:        now.Add(m.cfg.TTL).UTC().Format(time.RFC3339),
		Secure:         m.cfg.Secure,
		HTTPOnly:       true,
		Path:           "/",
		SameSite:       sameSiteString(m.cfg.SameSite),
	}
}

func sameSiteString(value http.SameSite) string {
	switch value {
	case http.SameSiteNoneMode:
		return "none"
	case http.SameSiteLaxMode:
		return "lax"
	case http.SameSiteStrictMode:
		return "strict"
	default:
		return ""
	}
}

func signSessionID(sessionID, secret string) string {
	signature := cookieSignature(sessionID, secret)
	return "s:" + sessionID + "." + signature
}

func unsignSessionID(cookieValue, secret string) (string, error) {
	if cookieValue == "" {
		return "", fmt.Errorf("missing session cookie")
	}

	if !strings.HasPrefix(cookieValue, "s:") {
		return cookieValue, nil
	}

	signedValue := strings.TrimPrefix(cookieValue, "s:")
	index := strings.LastIndex(signedValue, ".")
	if index <= 0 {
		return "", fmt.Errorf("invalid signed session cookie")
	}

	sessionID := signedValue[:index]
	signature := signedValue[index+1:]
	expected := cookieSignature(sessionID, secret)

	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return "", fmt.Errorf("invalid session signature")
	}

	return sessionID, nil
}

func cookieSignature(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return strings.TrimRight(base64.StdEncoding.EncodeToString(mac.Sum(nil)), "=")
}
