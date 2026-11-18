package v1

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var gearAsset = regexp.MustCompile(`^data/(meta/[a-zA-Z0-9_/-]+\.json|mo3/[0-9]+\.mo3|bone/[0-9]+\.bone|textures/[0-9]+\.(webp|png|blp)|icons/[a-zA-Z0-9_-]+\.jpg)$`)

func RegisterGearManager(mux *http.ServeMux, dir string) {
	mux.HandleFunc("GET /api/v1/gear-manager/{file...}", RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		name := r.PathValue("file")
		var file string
		switch name {
		case "":
			file = "private-ui/index.html"
		case "viewer.js", "planner.js", "jquery.js":
			file = "private-ui/" + name
		default:
			if !fs.ValidPath(name) || !gearAsset.MatchString(name) {
				http.NotFound(w, r)
				return
			}
			file = strings.TrimPrefix(name, "data/")
		}
		if dir == "" {
			http.Error(w, "Der Gear-Manager ist noch nicht eingerichtet.", http.StatusServiceUnavailable)
			return
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			http.Error(w, "Der Gear-Manager ist noch nicht eingerichtet.", http.StatusServiceUnavailable)
			return
		}
		defer root.Close()
		f, err := root.Open(file)
		if os.IsNotExist(err) && file == "meta/charactercustomization2/2_0.json" {
			f, err = root.Open("meta/charactercustomization2/2.json")
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		if name == "" {
			page, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, "Gear-Manager nicht verfügbar.", http.StatusInternalServerError)
				return
			}
			p, _ := CurrentPlayer(r)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; worker-src 'self' blob:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
			_, _ = w.Write(bytes.Replace(page, []byte("PLAYER_ID"), []byte(strconv.FormatInt(p.ID, 10)), 1))
			return
		}
		if path.Ext(file) == ".js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		http.ServeContent(w, r, path.Base(file), info.ModTime(), f)
	}))
}
