package session

import (
	"encoding/json"
	"net/http/httptest"
	"net/http"
	"testing"
	"time"

	"hms/go-backend/internal/config"
)

func TestSignAndUnsignSessionID(t *testing.T) {
	secret := "test-session-secret"
	sessionID := "abc123session"

	signed := signSessionID(sessionID, secret)
	if signed == sessionID {
		t.Fatalf("expected signed value to differ from raw session ID")
	}

	decoded, err := unsignSessionID(signed, secret)
	if err != nil {
		t.Fatalf("expected signed cookie to validate, got error: %v", err)
	}

	if decoded != sessionID {
		t.Fatalf("expected decoded session ID %q, got %q", sessionID, decoded)
	}
}

func TestUnsignSessionIDRejectsTamperedValue(t *testing.T) {
	secret := "test-session-secret"
	signed := signSessionID("abc123session", secret) + "tampered"

	if _, err := unsignSessionID(signed, secret); err == nil {
		t.Fatalf("expected tampered cookie to be rejected")
	}
}

func TestSessionRecordJSONShapeIncludesExpressCookie(t *testing.T) {
	manager := &Manager{
		cfg: config.SessionConfig{
			TTL:      7 * 24 * time.Hour,
			Secure:   false,
			SameSite: http.SameSiteStrictMode,
		},
	}

	record := Record{
		Cookie: manager.newSessionCookie(time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)),
		UserID: "507f1f77bcf86cd799439011",
		UserData: UserSnapshot{
			ID:      "507f1f77bcf86cd799439011",
			Email:   "admin@example.com",
			Role:    "Admin",
			Authz:   map[string]any{"override": map[string]any{"allowRoutes": []string{"route.admin.dashboard"}}},
			Hostel:  nil,
			PinnedTabs: []string{"/admin"},
		},
		Role:  "Admin",
		Email: "admin@example.com",
	}

	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("expected record to marshal, got error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("expected marshaled record to unmarshal, got error: %v", err)
	}

	cookieValue, ok := decoded["cookie"].(map[string]any)
	if !ok {
		t.Fatalf("expected cookie object in session payload")
	}

	if _, exists := cookieValue["originalMaxAge"]; !exists {
		t.Fatalf("expected originalMaxAge in cookie payload")
	}

	userDataValue, ok := decoded["userData"].(map[string]any)
	if !ok {
		t.Fatalf("expected userData object in session payload")
	}

	authzValue, ok := userDataValue["authz"].(map[string]any)
	if !ok {
		t.Fatalf("expected authz object in userData payload")
	}

	if _, hasEffective := authzValue["effective"]; hasEffective {
		t.Fatalf("expected shared session payload to omit authz.effective")
	}
}

func TestWriteCookieAndReadSessionIDRoundTrip(t *testing.T) {
	manager := &Manager{
		cfg: config.SessionConfig{
			CookieName: "connect.sid",
			Secret:     "test-session-secret",
			TTL:        24 * time.Hour,
			Secure:     false,
			SameSite:   http.SameSiteLaxMode,
		},
	}

	recorder := httptest.NewRecorder()
	manager.WriteCookie(recorder, "abc123session")

	response := recorder.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie to be written, got %d", len(cookies))
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookies[0])

	sessionID, err := manager.ReadSessionID(request)
	if err != nil {
		t.Fatalf("expected cookie roundtrip to succeed, got error: %v", err)
	}

	if sessionID != "abc123session" {
		t.Fatalf("expected session ID %q, got %q", "abc123session", sessionID)
	}

	if cookies[0].Expires.IsZero() {
		t.Fatalf("expected written cookie to include expires")
	}
}
