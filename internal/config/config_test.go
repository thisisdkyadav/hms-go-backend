package config

import (
	"net/http"
	"testing"
)

func TestDefaultSameSiteUsesLaxForNonSecureCookies(t *testing.T) {
	if got := defaultSameSite(false); got != http.SameSiteLaxMode {
		t.Fatalf("expected Lax for non-secure cookies, got %v", got)
	}
}

func TestDefaultSameSiteUsesNoneForSecureCookies(t *testing.T) {
	if got := defaultSameSite(true); got != http.SameSiteNoneMode {
		t.Fatalf("expected None for secure cookies, got %v", got)
	}
}
