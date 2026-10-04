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
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the compiled frontend assets rooted at the dist directory.
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
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
