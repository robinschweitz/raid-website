// Package webui exposes the compiled single-page application so the Go server
// can serve the frontend from a single binary.
//
// The frontend is built into this directory by Vite (see web/vite.config.ts) and
// must be built before compiling the Go server:
//
//	cd web && npm ci && npm run build
package webui

import (
	"embed"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"strings"
)

func init() {
	// The system MIME table differs per host; the web manifest needs its own type
	// on every one (FR-28.5).
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

//go:embed all:dist
var distFS embed.FS

// FS returns the compiled frontend assets rooted at the dist directory.
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// ErrNotBuilt reports that the embedded frontend has no index.html.
var ErrNotBuilt = errors.New("web build missing: run cd web && npm ci && npm run build before go build")

// CheckBuilt returns ErrNotBuilt when the binary was built without the
// frontend, so production can refuse to start (NFR-1.1, DR-13).
func CheckBuilt() error {
	root, err := FS()
	if err != nil {
		return err
	}
	return checkIndex(root)
}

func checkIndex(root fs.FS) error {
	info, err := fs.Stat(root, "index.html")
	if err != nil || info.IsDir() {
		return ErrNotBuilt
	}
	return nil
}

// Handler serves the embedded SPA and falls back to index.html for unknown
// client-side routes. Requests below /api/ are never handled here so that
// unknown API routes still return 404 instead of the HTML shell.
func Handler() (http.Handler, error) {
	root, err := FS()
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if info, err := fs.Stat(root, path); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		serveIndex(w, root)
	}), nil
}

func serveIndex(w http.ResponseWriter, root fs.FS) {
	data, err := fs.ReadFile(root, "index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}
