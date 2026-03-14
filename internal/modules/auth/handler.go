package auth

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"hms/go-backend/internal/shared/httpx"
	"hms/go-backend/internal/shared/session"
)

type Handler struct {
	logger   *slog.Logger
	service  *Service
	sessions *session.Manager
}

func NewHandler(logger *slog.Logger, service *Service, sessions *session.Manager) *Handler {
	return &Handler{
		logger:   logger,
		service:  service,
		sessions: sessions,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/google", h.loginWithGoogle)
	mux.HandleFunc("POST /api/v1/auth/verify-sso-token", h.verifyExternalSSOToken)
	mux.HandleFunc("GET /api/v1/auth/user", h.requireAuth(h.getUser))
	mux.HandleFunc("PATCH /api/v1/auth/user/pinned-tabs", h.requireAuth(h.updatePinnedTabs))
	mux.HandleFunc("GET /api/v1/auth/logout", h.requireAuth(h.logout))
	mux.HandleFunc("GET /api/v1/auth/refresh", h.requireAuth(h.refresh))
	mux.HandleFunc("GET /api/v1/auth/user/devices", h.requireAuth(h.getUserDevices))
	mux.HandleFunc("POST /api/v1/auth/user/devices/logout/{sessionId}", h.requireAuth(h.logoutDevice))
	mux.HandleFunc("POST /api/v1/auth/update-password", h.requireAuth(h.updatePassword))
	mux.HandleFunc("POST /api/v1/auth/forgot-password", h.forgotPassword)
	mux.HandleFunc("GET /api/v1/auth/reset-password/{token}", h.verifyResetToken)
	mux.HandleFunc("POST /api/v1/auth/reset-password", h.resetPassword)
	mux.HandleFunc("GET /api/v1/sso/redirect", h.requireAuth(h.redirectToSSO))
	mux.HandleFunc("POST /api/v1/sso/verify", h.verifySignedSSOToken)
	mux.HandleFunc("POST /api/sso/verify", h.verifySignedSSOToken)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

type updatePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type updatePinnedTabsRequest struct {
	PinnedTabs []string `json:"pinnedTabs"`
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, string, *session.Record)

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}

	if strings.TrimSpace(request.Email) == "" {
		httpx.WriteError(w, httpx.BadRequest("Email is required"))
		return
	}
	if strings.TrimSpace(request.Password) == "" {
		httpx.WriteError(w, httpx.BadRequest("Password is required"))
		return
	}

	result, err := h.service.Login(r.Context(), request.Email, request.Password, buildDeviceMetadata(r))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	h.sessions.WriteCookie(w, result.SessionID)
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"user": result.User}, result.Message)
}

func (h *Handler) loginWithGoogle(w http.ResponseWriter, r *http.Request) {
	var request tokenRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if strings.TrimSpace(request.Token) == "" {
		httpx.WriteError(w, httpx.BadRequest("Google token is required"))
		return
	}

	result, err := h.service.LoginWithGoogle(r.Context(), request.Token, buildDeviceMetadata(r))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	h.sessions.WriteCookie(w, result.SessionID)
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"user": result.User}, result.Message)
}

func (h *Handler) verifyExternalSSOToken(w http.ResponseWriter, r *http.Request) {
	var request tokenRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if strings.TrimSpace(request.Token) == "" {
		httpx.WriteError(w, httpx.BadRequest("Token is required"))
		return
	}

	result, err := h.service.VerifyExternalSSOToken(r.Context(), request.Token, buildDeviceMetadata(r))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	h.sessions.WriteCookie(w, result.SessionID)
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"user": result.User}, result.Message)
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request, _ string, record *session.Record) {
	user, err := h.service.GetCurrentUser(r.Context(), record.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, user, "")
}

func (h *Handler) updatePinnedTabs(w http.ResponseWriter, r *http.Request, sessionID string, record *session.Record) {
	var request updatePinnedTabsRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if request.PinnedTabs == nil {
		httpx.WriteError(w, httpx.BadRequest("pinnedTabs must be an array"))
		return
	}

	updated, err := h.service.UpdatePinnedTabs(r.Context(), record.UserID, request.PinnedTabs)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	record.UserData.PinnedTabs = updated
	if err := h.sessions.Save(r.Context(), sessionID, *record); err != nil {
		h.logger.Error("failed to persist session after pinned tab update", "error", err)
	}

	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"pinnedTabs": updated}, "Pinned tabs updated successfully")
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request, sessionID string, record *session.Record) {
	if err := h.service.Logout(r.Context(), sessionID, record.UserID); err != nil {
		httpx.WriteError(w, err)
		return
	}

	h.sessions.ClearCookie(w)
	httpx.WriteSuccess(w, http.StatusOK, nil, "Logged out successfully")
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request, sessionID string, record *session.Record) {
	user, err := h.service.RefreshSession(r.Context(), sessionID, record)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"user": user}, "User data refreshed")
}

func (h *Handler) getUserDevices(w http.ResponseWriter, r *http.Request, sessionID string, record *session.Record) {
	devices, err := h.service.GetUserDevices(r.Context(), record.UserID, sessionID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"devices": devices}, "")
}

func (h *Handler) logoutDevice(w http.ResponseWriter, r *http.Request, currentSessionID string, record *session.Record) {
	targetSessionID := strings.TrimSpace(r.PathValue("sessionId"))
	if targetSessionID == "" {
		httpx.WriteError(w, httpx.BadRequest("Session ID is required"))
		return
	}

	isCurrent, err := h.service.LogoutDevice(r.Context(), currentSessionID, targetSessionID, record.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	if isCurrent {
		h.sessions.ClearCookie(w)
		httpx.WriteSuccess(w, http.StatusOK, nil, "Logged out successfully")
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, nil, "Device logged out successfully")
}

func (h *Handler) updatePassword(w http.ResponseWriter, r *http.Request, _ string, record *session.Record) {
	var request updatePasswordRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}

	if strings.TrimSpace(request.OldPassword) == "" {
		httpx.WriteError(w, httpx.BadRequest("Old password is required"))
		return
	}
	if len(strings.TrimSpace(request.NewPassword)) < 6 {
		httpx.WriteError(w, httpx.BadRequest("New password must be at least 6 characters"))
		return
	}

	if err := h.service.UpdatePassword(r.Context(), record.UserID, request.OldPassword, request.NewPassword); err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, nil, "Password updated successfully")
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var request forgotPasswordRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}

	if strings.TrimSpace(request.Email) == "" {
		httpx.WriteError(w, httpx.BadRequest("Email is required"))
		return
	}

	if err := h.service.ForgotPassword(r.Context(), request.Email); err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, nil, passwordResetGenericMessage)
}

func (h *Handler) verifyResetToken(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		httpx.WriteError(w, httpx.BadRequest("Reset token is required"))
		return
	}

	preview, err := h.service.VerifyResetToken(r.Context(), token)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, preview, "Token is valid")
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var request resetPasswordRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}

	if strings.TrimSpace(request.Token) == "" {
		httpx.WriteError(w, httpx.BadRequest("Reset token is required"))
		return
	}
	if len(strings.TrimSpace(request.Password)) < 6 {
		httpx.WriteError(w, httpx.BadRequest("Password must be at least 6 characters"))
		return
	}

	if err := h.service.ResetPassword(r.Context(), request.Token, request.Password); err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, nil, "Password has been reset successfully")
}

func (h *Handler) redirectToSSO(w http.ResponseWriter, r *http.Request, _ string, record *session.Record) {
	redirectTarget := r.URL.Query().Get("redirect_to")
	if redirectTarget == "" {
		redirectTarget = r.URL.Query().Get("redirectTo")
	}

	redirectURL, err := h.service.BuildSSORedirectURL(redirectTarget, record)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *Handler) verifySignedSSOToken(w http.ResponseWriter, r *http.Request) {
	var request tokenRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, err)
		return
	}

	user, err := h.service.VerifySignedSSOToken(request.Token)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"user": user}, "")
}

func (h *Handler) requireAuth(next authenticatedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, err := h.sessions.ReadSessionID(r)
		if err != nil {
			httpx.WriteError(w, httpx.Unauthorized("Authentication required"))
			return
		}

		record, err := h.sessions.Get(r.Context(), sessionID)
		if err != nil {
			httpx.WriteError(w, httpx.Unauthorized("Authentication failed"))
			return
		}
		if record == nil || strings.TrimSpace(record.UserID) == "" {
			httpx.WriteError(w, httpx.Unauthorized("Authentication required"))
			return
		}

		if err := h.sessions.Touch(context.Background(), sessionID, record.UserID, time.Now().UTC()); err != nil {
			h.logger.Error("failed to touch session", "error", err, "sessionId", sessionID)
		}

		next(w, r, sessionID, record)
	}
}

func buildDeviceMetadata(r *http.Request) session.DeviceMetadata {
	userAgent := r.UserAgent()
	return session.DeviceMetadata{
		UserAgent:  userAgent,
		IP:         clientIP(r),
		DeviceName: deviceNameFromUserAgent(userAgent),
	}
}

func clientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if forwarded != "" {
		return forwarded
	}

	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}

func deviceNameFromUserAgent(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "iPhone"):
		return "iPhone"
	case strings.Contains(userAgent, "iPad"):
		return "iPad"
	case strings.Contains(userAgent, "Android"):
		return "Android device"
	case strings.Contains(userAgent, "Chrome") && !strings.Contains(userAgent, "Chromium") && !strings.Contains(userAgent, "Edge"):
		return "Chrome browser"
	case strings.Contains(userAgent, "Firefox"):
		return "Firefox browser"
	case strings.Contains(userAgent, "Safari") && !strings.Contains(userAgent, "Chrome") && !strings.Contains(userAgent, "Chromium"):
		return "Safari browser"
	case strings.Contains(userAgent, "Edge") || strings.Contains(userAgent, "Edg"):
		return "Edge browser"
	case strings.Contains(userAgent, "MSIE") || strings.Contains(userAgent, "Trident"):
		return "Internet Explorer"
	case strings.Contains(userAgent, "Windows"):
		return "Windows device"
	case strings.Contains(userAgent, "Macintosh") || strings.Contains(userAgent, "Mac OS X"):
		return "Mac device"
	case strings.Contains(userAgent, "Linux"):
		return "Linux device"
	default:
		return "Unknown device"
	}
}
