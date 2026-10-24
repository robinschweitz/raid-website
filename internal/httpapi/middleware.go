package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"raid-website/internal/auth"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/validation"
)

const csrfCookieName = "raid_csrf"

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err == nil {
			w.Header().Set("X-Request-ID", hex.EncodeToString(b))
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(enableHSTS bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
			h.Set("Content-Security-Policy", strings.Join([]string{
				"default-src 'self'",
				"base-uri 'self'",
				"frame-ancestors 'none'",
				"object-src 'none'",
				"form-action 'self'",
				"img-src 'self' data: https://wow.zamimg.com",
				"style-src 'self' 'unsafe-inline'",
				"script-src 'self'",
				"connect-src 'self'",
			}, "; "))
			if enableHSTS {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func csrfProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// The token is required whether or not the request has a session cookie,
		// Origin, or Sec-Fetch-Site: absent browser metadata must never skip it (SC-019).
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" || r.Header.Get("X-CSRF-Token") == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
			v1.WriteError(w, http.StatusForbidden, "csrf_required", "CSRF token required")
			return
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
					ctx := v1.WithActor(r.Context(), p)
					if p.Role == validation.RoleAdmin {
						has, err := svc.HasSecondFactor(r.Context(), p.ID)
						if err != nil {
							log.Printf("authenticate: second factor: %v", err)
							v1.WriteError(w, http.StatusInternalServerError, "internal", "could not authenticate session")
							return
						}
						ctx = v1.WithAdminSecondFactor(ctx, has)
					}
					r = r.WithContext(ctx)
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

// Unwrap lets http.ResponseController reach the connection, so a handler can
// flush its answer before it sends push notifications (FR-32.4).
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}
