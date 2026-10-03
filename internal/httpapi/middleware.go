package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strconv"
	"time"

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

// actor parses the temporary X-Actor-ID header and stores it on the context.
// This is a placeholder for real authentication and will be replaced.
func actor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw := r.Header.Get("X-Actor-ID"); raw != "" {
			if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
				r = r.WithContext(v1.WithActor(r.Context(), id))
			}
		}
		next.ServeHTTP(w, r)
	})
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
