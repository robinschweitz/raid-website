package v1

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"raid-website/internal/auth"
)

// securityLog writes one JSON line per security event to stderr, which systemd
// sends to the journal (SC-041). Tests replace it.
var securityLog = slog.New(slog.NewJSONHandler(os.Stderr, nil))

// SetSecurityLogger replaces the security event logger and returns the previous one.
func SetSecurityLogger(l *slog.Logger) *slog.Logger {
	prev := securityLog
	securityLog = l
	return prev
}

// LogSecurityEvent writes a security event with the client address (SC-006) and
// the request ID. Callers pass only identifiers such as a player ID or a
// normalised username, never passwords, codes, or tokens (SC-030). A logging
// problem never reaches the request.
func LogSecurityEvent(w http.ResponseWriter, r *http.Request, trustProxyHeaders bool, event string, attrs ...slog.Attr) {
	base := []slog.Attr{
		slog.String("event", event),
		slog.String("client_address", clientAddress(r, trustProxyHeaders)),
		slog.String("request_id", w.Header().Get("X-Request-ID")),
	}
	securityLog.LogAttrs(r.Context(), slog.LevelWarn, "security_event", append(base, attrs...)...)
}

func (a *API) securityEvent(w http.ResponseWriter, r *http.Request, event string, attrs ...slog.Attr) {
	LogSecurityEvent(w, r, a.TrustProxyHeaders, event, attrs...)
}

// playerAttrs names the player of a failed MFA step when the challenge knew one.
func playerAttrs(err error) []slog.Attr {
	if id, ok := auth.PlayerIDFromError(err); ok {
		return []slog.Attr{slog.Int64("player_id", id)}
	}
	return nil
}

func normalisedUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
