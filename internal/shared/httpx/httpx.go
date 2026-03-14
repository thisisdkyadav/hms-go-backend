package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type FieldError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type AppError struct {
	Status  int
	Message string
	Errors  []FieldError
}

func (e *AppError) Error() string {
	return e.Message
}

func BadRequest(message string, fieldErrors ...FieldError) *AppError {
	return &AppError{Status: http.StatusBadRequest, Message: message, Errors: fieldErrors}
}

func Unauthorized(message string) *AppError {
	return &AppError{Status: http.StatusUnauthorized, Message: message}
}

func NotFound(message string) *AppError {
	return &AppError{Status: http.StatusNotFound, Message: message}
}

func Internal(message string) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: message}
}

func WriteSuccess(w http.ResponseWriter, status int, data any, message string) {
	if status == 0 {
		status = http.StatusOK
	}

	WriteJSON(w, status, map[string]any{
		"success": true,
		"message": nullableString(message),
		"data":    data,
		"errors":  nil,
	})
}

func WriteError(w http.ResponseWriter, err error) {
	appErr := &AppError{}
	if !errors.As(err, &appErr) {
		appErr = Internal("Request failed")
	}

	WriteJSON(w, appErr.Status, map[string]any{
		"success": false,
		"message": nullableString(appErr.Message),
		"data":    nil,
		"errors":  nullableErrors(appErr.Errors),
	})
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, fmt.Sprintf(`{"success":false,"message":"%s","data":null,"errors":null}`, "Failed to encode response"), http.StatusInternalServerError)
	}
}

func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return BadRequest("Request body is required")
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return BadRequest("Invalid request payload")
	}

	if decoder.More() {
		return BadRequest("Request body must contain a single JSON object")
	}

	return nil
}

type Middleware func(http.Handler) http.Handler

func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	wrapped := handler
	for index := len(middlewares) - 1; index >= 0; index-- {
		wrapped = middlewares[index](wrapped)
	}
	return wrapped
}

func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered", "path", r.URL.Path, "method", r.Method, "panic", recovered)
					WriteError(w, Internal("Internal server error"))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func RequestLogger(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("request completed", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
		})
	}
}

func CORS(allowedOrigins []string) Middleware {
	allowedSet := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowedSet[origin] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && isOriginAllowed(origin, allowedSet) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return value
}

func nullableErrors(value []FieldError) any {
	if len(value) == 0 {
		return nil
	}

	return value
}

func isOriginAllowed(origin string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}

	if _, exists := allowed["*"]; exists {
		return true
	}

	_, exists := allowed[origin]
	return exists
}
