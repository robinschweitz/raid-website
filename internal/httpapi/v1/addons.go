package v1

import (
	"errors"
	"net/http"
	"strconv"

	"raid-website/internal/addons"
)

// AddonHandlers offers the bundled WoW addons for download (ADR 085). Both
// routes are public: the addons are public on GitHub, and the addons page,
// like the guide, is readable before an account exists.
type AddonHandlers struct{ Addons *addons.Catalog }

// Register mounts the addon list and the zip downloads.
func (h AddonHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/addons", h.list)
	mux.HandleFunc("GET /api/v1/addons/{name}/download", h.download)
}

func (h AddonHandlers) list(w http.ResponseWriter, _ *http.Request) {
	writeListJSON(w, h.Addons.List())
}

func (h AddonHandlers) download(w http.ResponseWriter, r *http.Request) {
	addon, err := h.Addons.Get(r.PathValue("name"))
	if errors.Is(err, addons.ErrUnknownAddon) {
		WriteError(w, http.StatusNotFound, "addon_not_found", "Dieses Addon gibt es hier nicht.")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "Download fehlgeschlagen.")
		return
	}
	data := addon.Zip()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+addon.FileName+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
