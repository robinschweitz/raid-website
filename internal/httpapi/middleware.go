package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"time"

	"raid-website/internal/auth"
	v1 "raid-website/internal/httpapi/v1"
)

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err == nil {
			w.Header().Set("X-Request-ID", hex.EncodeToString(b))
		}
		next.ServeHTTP(w, r)
	})
}

// authenticate resolves the session cookie (if any) into the current player and
// stores it on the request context. Missing/invalid sessions are left unset;
// route guards decide whether authentication is required.
func authenticate(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
				p, err := svc.Authenticate(r.Context(), c.Value)
				if err != nil && !errors.Is(err, auth.ErrInactive) {
					log.Printf("authenticate: %v", err)
					v1.WriteError(w, http.StatusInternalServerError, "internal", "could not authenticate session")
					return
				}
				if p != nil {
					r = r.WithContext(v1.WithActor(r.Context(), p))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
