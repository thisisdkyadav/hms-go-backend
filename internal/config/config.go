package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App            AppConfig
	Mongo          MongoConfig
	Redis          RedisConfig
	Session        SessionConfig
	SMTP           SMTPConfig
	AllowedOrigins []string
	FrontendURL    string
	Auth           AuthConfig
}

type AppConfig struct {
	Env             string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type MongoConfig struct {
	URI      string
	Database string
}

type RedisConfig struct {
	URL string
}

type SessionConfig struct {
	CookieName string
	CookieDomain string
	Prefix     string
	TTL        time.Duration
	Secret     string
	Secure     bool
	SameSite   http.SameSite
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type AuthConfig struct {
	JWTSecret            string
	SSOVerifyURL         string
	GoogleTokenVerifyURL string
	PasswordResetTTL     time.Duration
	BcryptCost           int
}

func Load() (Config, error) {
	mongoURI := envString("MONGO_URI", "")
	sessionSecret := envString("SESSION_SECRET", "")

	if mongoURI == "" {
		return Config{}, fmt.Errorf("MONGO_URI is required")
	}

	if sessionSecret == "" {
		return Config{}, fmt.Errorf("SESSION_SECRET is required")
	}

	cfg := Config{
		App: AppConfig{
			Env:             envString("APP_ENV", envString("NODE_ENV", "development")),
			Port:            envInt("PORT", 5001),
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    15 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Mongo: MongoConfig{
			URI:      mongoURI,
			Database: resolveMongoDatabaseName(mongoURI, envString("MONGO_DB_NAME", "")),
		},
		Redis: RedisConfig{
			URL: envString("REDIS_URL", "redis://localhost:6379"),
		},
		Session: SessionConfig{
			CookieName: envString("SESSION_COOKIE_NAME", "connect.sid"),
			CookieDomain: envString("SESSION_COOKIE_DOMAIN", ""),
			Prefix:     envString("REDIS_SESSION_PREFIX", "go:sess:"),
			TTL:        time.Duration(envInt("SESSION_TTL_SECONDS", 7*24*60*60)) * time.Second,
			Secret:     sessionSecret,
			Secure:     envBool("SESSION_SECURE", false),
			SameSite:   resolveSameSite(envString("SESSION_SAME_SITE", "")),
		},
		SMTP: SMTPConfig{
			Host:     envString("SMTP_HOST", "smtp.gmail.com"),
			Port:     envInt("SMTP_PORT", 587),
			Username: envString("SMTP_USERNAME", envString("SMTP_USER", "")),
			Password: envString("SMTP_PASSWORD", envString("SMTP_PASS", "")),
			From:     envString("SMTP_FROM", "HMS <saappsupport@iiti.ac.in>"),
		},
		AllowedOrigins: splitCSV(envString("ALLOWED_ORIGINS", "")),
		FrontendURL:    envString("FRONTEND_URL", "http://localhost:3000"),
		Auth: AuthConfig{
			JWTSecret:            envString("JWT_SECRET", ""),
			SSOVerifyURL:         envString("AUTH_SSO_VERIFY_URL", "https://hms-sso.andiindia.in/api/auth/verify-sso-token"),
			GoogleTokenVerifyURL: envString("GOOGLE_TOKEN_VERIFY_URL", "https://www.googleapis.com/oauth2/v3/tokeninfo"),
			PasswordResetTTL:     time.Duration(envInt("PASSWORD_RESET_TTL_MINUTES", 60)) * time.Minute,
			BcryptCost:           envInt("BCRYPT_COST", 10),
		},
	}

	if cfg.Mongo.Database == "" {
		return Config{}, fmt.Errorf("MONGO_DB_NAME could not be resolved")
	}

	if cfg.Session.SameSite == 0 {
		if cfg.App.Env == "production" {
			cfg.Session.SameSite = http.SameSiteNoneMode
		} else {
			cfg.Session.SameSite = http.SameSiteStrictMode
		}
	}

	return cfg, nil
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

func resolveMongoDatabaseName(uri, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return ""
	}

	return strings.TrimPrefix(parsed.Path, "/")
}

func resolveSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return 0
	}
}
