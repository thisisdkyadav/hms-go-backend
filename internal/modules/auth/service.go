package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hms/go-backend/internal/config"
	"hms/go-backend/internal/shared/email"
	"hms/go-backend/internal/shared/httpx"
	"hms/go-backend/internal/shared/session"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

const passwordResetGenericMessage = "If an account with that email exists, a password reset link has been sent."

type Service struct {
	cfg        config.Config
	logger     *slog.Logger
	repo       *Repository
	sessions   *session.Manager
	email      email.Sender
	httpClient *http.Client
}

func NewService(cfg config.Config, logger *slog.Logger, repo *Repository, sessions *session.Manager, emailSender email.Sender) *Service {
	return &Service{
		cfg:      cfg,
		logger:   logger,
		repo:     repo,
		sessions: sessions,
		email:    emailSender,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *Service) Login(ctx context.Context, emailAddress, password string, meta session.DeviceMetadata) (*AuthSessionResult, error) {
	user, err := s.repo.FindUserByEmail(ctx, emailAddress, true)
	if err != nil {
		return nil, httpx.Internal("Failed to load user")
	}
	if user == nil {
		return nil, httpx.Unauthorized("Invalid email or password")
	}
	if strings.TrimSpace(pointerValue(user.Password)) == "" {
		return nil, httpx.Unauthorized("Password not set for this account")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pointerValue(user.Password)), []byte(password)); err != nil {
		return nil, httpx.Unauthorized("Invalid email or password")
	}

	return s.createSessionResult(ctx, user, meta, "Login successful")
}

func (s *Service) LoginWithGoogle(ctx context.Context, token string, meta session.DeviceMetadata) (*AuthSessionResult, error) {
	emailAddress, err := s.verifyGoogleToken(ctx, token)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindUserByEmail(ctx, emailAddress, false)
	if err != nil {
		return nil, httpx.Internal("Failed to load user")
	}
	if user == nil {
		return nil, httpx.Unauthorized("User not found")
	}

	return s.createSessionResult(ctx, user, meta, "Login successful")
}

func (s *Service) VerifyExternalSSOToken(ctx context.Context, token string, meta session.DeviceMetadata) (*AuthSessionResult, error) {
	emailAddress, err := s.verifyRemoteSSOToken(ctx, token)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindUserByEmail(ctx, emailAddress, false)
	if err != nil {
		return nil, httpx.Internal("Failed to load user")
	}
	if user == nil {
		return nil, httpx.NotFound("User not found in system")
	}

	return s.createSessionResult(ctx, user, meta, "SSO authentication successful")
}

func (s *Service) GetCurrentUser(ctx context.Context, userID string) (*UserResponse, error) {
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, httpx.Unauthorized("Authentication failed")
	}

	user, err := s.repo.FindUserByID(ctx, objectID, false)
	if err != nil {
		return nil, httpx.Internal("Failed to load user")
	}
	if user == nil {
		return nil, httpx.NotFound("User not found")
	}

	response, _, err := s.buildUserBundle(ctx, user)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (s *Service) RefreshSession(ctx context.Context, sessionID string, record *session.Record) (*UserResponse, error) {
	response, snapshot, err := s.loadSessionUser(ctx, record.UserID)
	if err != nil {
		return nil, err
	}

	record.UserID = snapshot.ID
	record.UserData = snapshot
	record.Role = snapshot.Role
	record.Email = snapshot.Email

	if err := s.sessions.Save(ctx, sessionID, *record); err != nil {
		return nil, httpx.Internal("Failed to refresh session")
	}

	return &response, nil
}

func (s *Service) Logout(ctx context.Context, sessionID, userID string) error {
	if err := s.sessions.Delete(ctx, sessionID, userID); err != nil {
		return httpx.Internal("Logout failed")
	}
	return nil
}

func (s *Service) GetUserDevices(ctx context.Context, userID, currentSessionID string) ([]session.DeviceMetadata, error) {
	devices, err := s.sessions.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, httpx.Internal("Failed to load devices")
	}

	validDevices := make([]session.DeviceMetadata, 0, len(devices))
	for _, device := range devices {
		isCurrent := device.SessionID == currentSessionID
		exists := isCurrent
		if !isCurrent {
			exists, err = s.sessions.Exists(ctx, device.SessionID)
			if err != nil {
				return nil, httpx.Internal("Failed to inspect active sessions")
			}
		}

		if !exists {
			_ = s.sessions.Delete(ctx, device.SessionID, userID)
			continue
		}

		if isCurrent {
			now := time.Now().UTC()
			device.LastActive = now
			_ = s.sessions.Touch(ctx, currentSessionID, userID, now)
		}

		device.IsCurrent = isCurrent
		validDevices = append(validDevices, device)
	}

	return validDevices, nil
}

func (s *Service) LogoutDevice(ctx context.Context, currentSessionID, targetSessionID, userID string) (bool, error) {
	devices, err := s.sessions.ListUserSessions(ctx, userID)
	if err != nil {
		return false, httpx.Internal("Failed to load device sessions")
	}

	authorized := false
	for _, device := range devices {
		if device.SessionID == targetSessionID {
			authorized = true
			break
		}
	}

	if !authorized {
		return false, httpx.NotFound("Session not found or unauthorized")
	}

	if err := s.sessions.Delete(ctx, targetSessionID, userID); err != nil {
		return false, httpx.Internal("Failed to logout device")
	}

	return targetSessionID == currentSessionID, nil
}

func (s *Service) UpdatePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return httpx.Unauthorized("Authentication failed")
	}

	user, err := s.repo.FindUserByID(ctx, objectID, true)
	if err != nil {
		return httpx.Internal("Failed to load user")
	}
	if user == nil {
		return httpx.NotFound("User not found")
	}
	if strings.TrimSpace(pointerValue(user.Password)) == "" {
		return httpx.Unauthorized("No password is currently set for this account")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pointerValue(user.Password)), []byte(oldPassword)); err != nil {
		return httpx.Unauthorized("Old password is incorrect")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.cfg.Auth.BcryptCost)
	if err != nil {
		return httpx.Internal("Failed to hash password")
	}

	if err := s.repo.UpdateUserPassword(ctx, objectID, string(passwordHash)); err != nil {
		return httpx.Internal("Failed to update password")
	}

	return nil
}

func (s *Service) ForgotPassword(ctx context.Context, emailAddress string) error {
	user, err := s.repo.FindUserByEmail(ctx, emailAddress, false)
	if err != nil {
		return httpx.Internal("Failed to process password reset")
	}
	if user == nil {
		return nil
	}

	if err := s.repo.InvalidatePasswordResetTokens(ctx, user.ID); err != nil {
		return httpx.Internal("Failed to invalidate password reset tokens")
	}

	resetToken, err := randomHex(32)
	if err != nil {
		return httpx.Internal("Failed to generate password reset token")
	}

	hashedToken := hashToken(resetToken)
	tokenDocument := PasswordResetToken{
		UserID:    user.ID,
		Token:     hashedToken,
		ExpiresAt: time.Now().UTC().Add(s.cfg.Auth.PasswordResetTTL),
		Used:      false,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.repo.CreatePasswordResetToken(ctx, tokenDocument); err != nil {
		return httpx.Internal("Failed to store password reset token")
	}

	if err := s.email.SendPasswordResetEmail(ctx, user.Email, user.Name, resetToken); err != nil {
		s.logger.Error("failed to send password reset email", "error", err, "email", user.Email)
	}

	return nil
}

func (s *Service) VerifyResetToken(ctx context.Context, token string) (*PasswordResetPreview, error) {
	tokenDocument, err := s.repo.FindValidPasswordResetToken(ctx, hashToken(token))
	if err != nil {
		return nil, httpx.Internal("Failed to verify reset token")
	}
	if tokenDocument == nil {
		return nil, httpx.BadRequest("Invalid or expired reset token")
	}

	user, err := s.repo.FindUserByID(ctx, tokenDocument.UserID, false)
	if err != nil {
		return nil, httpx.Internal("Failed to load user")
	}
	if user == nil {
		return nil, httpx.NotFound("User not found")
	}

	return &PasswordResetPreview{
		User: ResetTokenUser{
			ID:    user.ID.Hex(),
			Name:  user.Name,
			Email: user.Email,
		},
	}, nil
}

func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	tokenDocument, err := s.repo.FindValidPasswordResetToken(ctx, hashToken(token))
	if err != nil {
		return httpx.Internal("Failed to reset password")
	}
	if tokenDocument == nil {
		return httpx.BadRequest("Invalid or expired reset token")
	}

	user, err := s.repo.FindUserByID(ctx, tokenDocument.UserID, false)
	if err != nil {
		return httpx.Internal("Failed to load user")
	}
	if user == nil {
		return httpx.NotFound("User not found")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), s.cfg.Auth.BcryptCost)
	if err != nil {
		return httpx.Internal("Failed to hash password")
	}

	if err := s.repo.UpdateUserPassword(ctx, tokenDocument.UserID, string(passwordHash)); err != nil {
		return httpx.Internal("Failed to update password")
	}
	if err := s.repo.MarkPasswordResetTokenUsed(ctx, tokenDocument.ID); err != nil {
		return httpx.Internal("Failed to mark token as used")
	}
	if err := s.repo.InvalidatePasswordResetTokens(ctx, tokenDocument.UserID); err != nil {
		return httpx.Internal("Failed to invalidate reset tokens")
	}

	if err := s.email.SendPasswordResetSuccessEmail(ctx, user.Email, user.Name); err != nil {
		s.logger.Error("failed to send password reset success email", "error", err, "email", user.Email)
	}

	return nil
}

func (s *Service) UpdatePinnedTabs(ctx context.Context, userID string, pinnedTabs []string) ([]string, error) {
	normalized := normalizePinnedTabs(pinnedTabs)
	if len(normalized) > 30 {
		return nil, httpx.BadRequest("Too many pinned tabs")
	}
	for _, path := range normalized {
		if !strings.HasPrefix(path, "/admin") {
			return nil, httpx.BadRequest("Invalid pinned tab path")
		}
	}

	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, httpx.Unauthorized("Authentication failed")
	}

	updated, err := s.repo.UpdatePinnedTabs(ctx, objectID, normalized)
	if err != nil {
		return nil, httpx.Internal("Failed to update pinned tabs")
	}
	if updated == nil {
		return nil, httpx.NotFound("User not found")
	}

	return updated, nil
}

func (s *Service) BuildSSORedirectURL(redirectTarget string, record *session.Record) (string, error) {
	if strings.TrimSpace(redirectTarget) == "" {
		return "", httpx.BadRequest("Missing redirect_to parameter")
	}

	parsedURL, err := url.Parse(redirectTarget)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", httpx.BadRequest("Invalid redirect_to parameter")
	}

	if strings.TrimSpace(record.UserData.Email) == "" {
		return "", httpx.Unauthorized("No user data in session")
	}

	token, err := signToken(s.jwtSecret(), record.UserData)
	if err != nil {
		return "", httpx.Internal("Failed to sign SSO token")
	}

	query := parsedURL.Query()
	query.Set("token", token)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func (s *Service) VerifySignedSSOToken(token string) (*VerifiedSSOUser, error) {
	if strings.TrimSpace(token) == "" {
		return nil, httpx.BadRequest("Token is required")
	}

	var payload session.UserSnapshot
	if err := verifyToken(s.jwtSecret(), token, &payload); err != nil {
		return nil, httpx.Unauthorized("Invalid or expired token")
	}

	return &VerifiedSSOUser{
		ID:         payload.ID,
		Email:      payload.Email,
		Role:       payload.Role,
		SubRole:    payload.SubRole,
		Hostel:     hostelDTOFromSession(payload.Hostel),
		PinnedTabs: copyStringSlice(payload.PinnedTabs),
	}, nil
}

func (s *Service) createSessionResult(ctx context.Context, user *User, meta session.DeviceMetadata, message string) (*AuthSessionResult, error) {
	response, snapshot, err := s.buildUserBundle(ctx, user)
	if err != nil {
		return nil, err
	}

	record := session.Record{
		UserID:   snapshot.ID,
		UserData: snapshot,
		Role:     snapshot.Role,
		Email:    snapshot.Email,
	}

	sessionID, err := s.sessions.Create(ctx, record, meta)
	if err != nil {
		return nil, httpx.Internal("Failed to create session. Please try again.")
	}

	return &AuthSessionResult{
		User:      response,
		SessionID: sessionID,
		Message:   message,
	}, nil
}

func (s *Service) loadSessionUser(ctx context.Context, userID string) (UserResponse, session.UserSnapshot, error) {
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return UserResponse{}, session.UserSnapshot{}, httpx.Unauthorized("Authentication failed")
	}

	user, err := s.repo.FindUserByID(ctx, objectID, false)
	if err != nil {
		return UserResponse{}, session.UserSnapshot{}, httpx.Internal("Failed to load user")
	}
	if user == nil {
		return UserResponse{}, session.UserSnapshot{}, httpx.NotFound("User not found")
	}

	return s.buildUserBundle(ctx, user)
}

func (s *Service) buildUserBundle(ctx context.Context, user *User) (UserResponse, session.UserSnapshot, error) {
	if strings.TrimSpace(user.AESKey) == "" {
		aesKey, err := randomHex(32)
		if err != nil {
			return UserResponse{}, session.UserSnapshot{}, httpx.Internal("Failed to generate AES key")
		}

		if err := s.repo.SetUserAESKey(ctx, user.ID, aesKey); err != nil {
			return UserResponse{}, session.UserSnapshot{}, httpx.Internal("Failed to persist AES key")
		}

		user.AESKey = aesKey
	}

	hostel, err := s.repo.FindHostelSummary(ctx, user.Role, user.ID)
	if err != nil {
		return UserResponse{}, session.UserSnapshot{}, httpx.Internal("Failed to load hostel assignment")
	}

	authzResponse := UserAuthzResponse{
		Override:  normalizeAuthzOverride(user.Authz.Override),
		Effective: buildEffectiveAuthz(user.Role),
		Meta:      user.Authz.Meta,
	}

	response := UserResponse{
		ID:                    user.ID.Hex(),
		Name:                  user.Name,
		Email:                 user.Email,
		Phone:                 user.Phone,
		ProfileImage:          user.ProfileImage,
		Role:                  user.Role,
		SubRole:               user.SubRole,
		Authz:                 authzResponse,
		PinnedTabs:            normalizePinnedTabs(user.PinnedTabs),
		AESKey:                user.AESKey,
		AcceptingAppointments: user.AcceptingAppointments,
		CreatedAt:             user.CreatedAt,
		UpdatedAt:             user.UpdatedAt,
		Hostel:                hostelDTO(hostel),
	}

	snapshot := session.UserSnapshot{
		ID:      response.ID,
		Email:   response.Email,
		Role:    response.Role,
		SubRole: response.SubRole,
		Authz: map[string]any{
			"override": authzResponse.Override,
		},
		Hostel:     sessionHostel(hostel),
		PinnedTabs: copyStringSlice(response.PinnedTabs),
	}

	return response, snapshot, nil
}

func (s *Service) verifyGoogleToken(ctx context.Context, token string) (string, error) {
	requestURL := fmt.Sprintf("%s?id_token=%s", s.cfg.Auth.GoogleTokenVerifyURL, url.QueryEscape(token))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", httpx.Internal("Failed to create Google verification request")
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", httpx.Unauthorized("Invalid Google Token")
	}
	defer response.Body.Close()

	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", httpx.Unauthorized("Invalid Google Token")
	}

	if strings.TrimSpace(payload.Email) == "" {
		return "", httpx.Unauthorized("Invalid Google Token")
	}

	return payload.Email, nil
}

func (s *Service) verifyRemoteSSOToken(ctx context.Context, token string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"token": token})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Auth.SSOVerifyURL, bytes.NewReader(payload))
	if err != nil {
		return "", httpx.Internal("Failed to create SSO verification request")
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", httpx.Internal("Failed to verify SSO token")
	}
	defer response.Body.Close()

	var body struct {
		Success bool `json:"success"`
		User    struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return "", httpx.Internal("Failed to verify SSO token")
	}

	if !body.Success || strings.TrimSpace(body.User.Email) == "" {
		return "", httpx.Unauthorized("Invalid or expired SSO token")
	}

	return body.User.Email, nil
}

func normalizePinnedTabs(pinnedTabs []string) []string {
	if len(pinnedTabs) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(pinnedTabs))
	result := make([]string, 0, len(pinnedTabs))
	for _, tab := range pinnedTabs {
		trimmed := strings.TrimSpace(tab)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	return result
}

func normalizeAuthzOverride(input UserAuthzOverride) UserAuthzOverride {
	return UserAuthzOverride{
		AllowRoutes:       copyStringSlice(input.AllowRoutes),
		DenyRoutes:        copyStringSlice(input.DenyRoutes),
		AllowCapabilities: copyStringSlice(input.AllowCapabilities),
		DenyCapabilities:  copyStringSlice(input.DenyCapabilities),
		Constraints:       copyConstraints(input.Constraints),
	}
}

func buildEffectiveAuthz(role string) EffectiveAuthz {
	return EffectiveAuthz{
		CatalogVersion: 1,
		Role:           role,
		RouteAccess:    map[string]bool{},
		Capabilities:   map[string]bool{"*": true},
		Constraints:    map[string]any{},
	}
}

func hostelDTO(hostel *HostelSummary) *HostelDTO {
	if hostel == nil {
		return nil
	}

	return &HostelDTO{
		ID:   hostel.ID.Hex(),
		Name: hostel.Name,
		Type: hostel.Type,
	}
}

func sessionHostel(hostel *HostelSummary) *session.HostelSummary {
	if hostel == nil {
		return nil
	}

	return &session.HostelSummary{
		ID:   hostel.ID.Hex(),
		Name: hostel.Name,
		Type: hostel.Type,
	}
}

func hostelDTOFromSession(hostel *session.HostelSummary) *HostelDTO {
	if hostel == nil {
		return nil
	}

	return &HostelDTO{
		ID:   hostel.ID,
		Name: hostel.Name,
		Type: hostel.Type,
	}
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func copyStringSlice(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}

	result := make([]string, len(input))
	copy(result, input)
	return result
}

func copyConstraints(input []UserAuthzConstraint) []UserAuthzConstraint {
	if len(input) == 0 {
		return []UserAuthzConstraint{}
	}

	result := make([]UserAuthzConstraint, len(input))
	copy(result, input)
	return result
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomHex(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func (s *Service) jwtSecret() string {
	if strings.TrimSpace(s.cfg.Auth.JWTSecret) != "" {
		return s.cfg.Auth.JWTSecret
	}
	return s.cfg.Session.Secret
}

func signToken(secret string, payload any) (string, error) {
	header, err := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	headerPart := base64.RawURLEncoding.EncodeToString(header)
	bodyPart := base64.RawURLEncoding.EncodeToString(body)
	signingInput := headerPart + "." + bodyPart

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

func verifyToken(secret, token string, destination any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid token")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expectedSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedSignature), []byte(parts[2])) {
		return fmt.Errorf("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}

	return json.Unmarshal(payload, destination)
}
