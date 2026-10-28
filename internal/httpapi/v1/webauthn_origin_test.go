package v1

import (
	"net/http/httptest"
	"testing"
)

func TestReqAuth022_RelyingPartyComesOnlyFromPublicOrigin(t *testing.T) {
	r := httptest.NewRequest("POST", "http://internal.test/api/v1/auth/webauthn/login/options", nil)
	r.Host = "attacker.test"
	r.Header.Set("X-Forwarded-Host", "evil.test")
	r.Header.Set("X-Forwarded-Proto", "http")
	rpID, origin, ok := webAuthnRelyingParty("https://example.org", r)
	if !ok || rpID != "example.org" || origin != "https://example.org" {
		t.Fatalf("got %q %q %v, want example.org https://example.org", rpID, origin, ok)
	}
}

func TestReqAuth022_DevelopmentFallbackIgnoresForwardedHeaders(t *testing.T) {
	r := httptest.NewRequest("POST", "http://localhost:8080/x", nil)
	r.Header.Set("X-Forwarded-Host", "evil.test")
	r.Header.Set("X-Forwarded-Proto", "https")
	rpID, origin, ok := webAuthnRelyingParty("", r)
	if !ok || rpID != "localhost" || origin != "http://localhost:8080" {
		t.Fatalf("got %q %q %v", rpID, origin, ok)
	}
}
