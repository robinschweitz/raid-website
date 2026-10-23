package v1

import (
	"net/http/httptest"
	"testing"
)

func TestClientAddress(t *testing.T) {
	tests := []struct {
		name              string
		trustProxyHeaders bool
		realIP            string
		forwardedFor      string
		want              string
	}{
		{
			name:         "untrusted proxy headers are ignored",
			realIP:       "203.0.113.9",
			forwardedFor: "198.51.100.1",
			want:         "192.0.2.10",
		},
		{
			name:              "nginx X-Real-IP wins over a forged X-Forwarded-For",
			trustProxyHeaders: true,
			realIP:            "203.0.113.9",
			forwardedFor:      "198.51.100.1, 203.0.113.9",
			want:              "203.0.113.9",
		},
		{
			name:              "without X-Real-IP the proxy-appended last entry is used",
			trustProxyHeaders: true,
			forwardedFor:      "198.51.100.1, 198.51.100.2, 203.0.113.9",
			want:              "203.0.113.9",
		},
		{
			name:              "trusted mode without proxy headers uses the connection",
			trustProxyHeaders: true,
			want:              "192.0.2.10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
			r.RemoteAddr = "192.0.2.10:54321"
			if tt.realIP != "" {
				r.Header.Set("X-Real-IP", tt.realIP)
			}
			if tt.forwardedFor != "" {
				r.Header.Set("X-Forwarded-For", tt.forwardedFor)
			}
			if got := clientAddress(r, tt.trustProxyHeaders); got != tt.want {
				t.Fatalf("clientAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestForgedForwardedForCannotRotateClientAddress shows the attack the fix
// closes: a client that sends a different X-Forwarded-For on every login
// attempt must still be counted under one address.
func TestForgedForwardedForCannotRotateClientAddress(t *testing.T) {
	seen := map[string]bool{}
	for _, forged := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5", "10.0.0.6"} {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.RemoteAddr = "127.0.0.1:40000"
		// nginx: X-Real-IP $remote_addr; X-Forwarded-For $proxy_add_x_forwarded_for
		r.Header.Set("X-Real-IP", "203.0.113.9")
		r.Header.Set("X-Forwarded-For", forged+", 203.0.113.9")
		seen[clientAddress(r, true)] = true
	}
	if len(seen) != 1 || !seen["203.0.113.9"] {
		t.Fatalf("forged X-Forwarded-For values produced addresses %v, want only 203.0.113.9", seen)
	}
}
