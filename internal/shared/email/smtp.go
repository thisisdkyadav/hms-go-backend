package email

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"hms/go-backend/internal/config"
)

type Sender interface {
	SendPasswordResetEmail(ctx context.Context, email, userName, resetToken string) error
	SendPasswordResetSuccessEmail(ctx context.Context, email, userName string) error
}

type SMTP struct {
	cfg         config.SMTPConfig
	frontendURL string
	logger      *slog.Logger
}

func NewSMTP(cfg config.SMTPConfig, frontendURL string, logger *slog.Logger) *SMTP {
	return &SMTP{
		cfg:         cfg,
		frontendURL: frontendURL,
		logger:      logger,
	}
}

func (s *SMTP) SendPasswordResetEmail(_ context.Context, email, userName, resetToken string) error {
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimRight(s.frontendURL, "/"), resetToken)
	subject := "Reset Your HMS Password"
	html := fmt.Sprintf(`
<html>
  <body style="font-family: Arial, sans-serif; color: #0A1628;">
    <p>Hello %s,</p>
    <p>We received a request to reset your HMS password.</p>
    <p><a href="%s">Reset your password</a></p>
    <p>If you did not request this, you can ignore this email.</p>
  </body>
</html>`, userName, resetLink)

	return s.send(email, subject, html)
}

func (s *SMTP) SendPasswordResetSuccessEmail(_ context.Context, email, userName string) error {
	subject := "Your HMS Password Has Been Changed"
	html := fmt.Sprintf(`
<html>
  <body style="font-family: Arial, sans-serif; color: #0A1628;">
    <p>Hello %s,</p>
    <p>Your HMS password was changed successfully.</p>
    <p>If you did not perform this action, contact support immediately.</p>
  </body>
</html>`, userName)

	return s.send(email, subject, html)
}

func (s *SMTP) send(to, subject, html string) error {
	if strings.TrimSpace(s.cfg.Username) == "" || strings.TrimSpace(s.cfg.Password) == "" {
		s.logger.Info("smtp credentials not configured; skipping email send", "to", to, "subject", subject)
		return nil
	}

	address := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	message := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n%s", s.cfg.From, to, subject, html))

	if err := smtp.SendMail(address, auth, s.cfg.Username, []string{to}, message); err != nil {
		return fmt.Errorf("send smtp email: %w", err)
	}

	return nil
}
