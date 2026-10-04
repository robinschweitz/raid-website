package v1

import (
	"errors"
	"net/http"
	"strconv"

	"raid-website/internal/lootcatalog"
	"raid-website/internal/validation"
)

// CatalogHandlers is standalone so the coordinator can wire it without changing API dependencies.
type CatalogHandlers struct{ Catalog *lootcatalog.Service }

// Register mounts authenticated reads and Admin-only reviewed imports on the existing mux.
// The existing session middleware must surround this mux.
func (h CatalogHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/loot-catalog", RequireAuth(h.list))
	mux.HandleFunc("GET /api/v1/loot-catalog/instances", RequireAuth(h.instances))
	mux.HandleFunc("GET /api/v1/loot-catalog/bosses", RequireAuth(h.bosses))
	mux.HandleFunc("GET /api/v1/loot-catalog/items", RequireAuth(h.items))
	mux.HandleFunc("POST /api/v1/loot-catalog/preview", RequireRole(validation.RoleAdmin, h.preview))
	mux.HandleFunc("POST /api/v1/loot-catalog/merge", RequireRole(validation.RoleAdmin, h.merge))
}

type catalogRequest struct {
	Catalog lootcatalog.Dataset `json:"catalog"`
	RawHash string              `json:"raw_hash"`
}

func (h CatalogHandlers) list(w http.ResponseWriter, r *http.Request) {
	c, err := h.Catalog.List(r.Context())
	if err != nil {
		catalogError(w, err, nil)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (h CatalogHandlers) instances(w http.ResponseWriter, r *http.Request) {
	items, err := h.Catalog.Instances(r.Context())
	if err != nil {
		catalogError(w, err, nil)
		return
	}
	writeListJSON(w, items)
}

func (h CatalogHandlers) bosses(w http.ResponseWriter, r *http.Request) {
	items, err := h.Catalog.Bosses(r.Context(), r.URL.Query().Get("instance_id"))
	if err != nil {
		catalogError(w, err, nil)
		return
	}
	writeListJSON(w, items)
}

func (h CatalogHandlers) items(w http.ResponseWriter, r *http.Request) {
	filter := lootcatalog.Filter{BossID: r.URL.Query().Get("boss_id"), Difficulty: r.URL.Query().Get("difficulty")}
	if raw := r.URL.Query().Get("raid_size"); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil || size != 10 && size != 25 {
			catalogError(w, lootcatalog.ErrInvalid, nil)
			return
		}
		filter.RaidSize = size
	}
	items, err := h.Catalog.Items(r.Context(), filter)
	if err != nil {
		catalogError(w, err, nil)
		return
	}
	writeListJSON(w, items)
}

func (h CatalogHandlers) preview(w http.ResponseWriter, r *http.Request) {
	h.importCatalog(w, r, false)
}
func (h CatalogHandlers) merge(w http.ResponseWriter, r *http.Request) { h.importCatalog(w, r, true) }

func (h CatalogHandlers) importCatalog(w http.ResponseWriter, r *http.Request, merge bool) {
	var req catalogRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "catalog_invalid_json", "Ungueltiges Katalog-JSON")
		return
	}
	var report lootcatalog.Report
	var err error
	if merge {
		report, err = h.Catalog.Merge(r.Context(), req.Catalog, req.RawHash)
	} else {
		report, err = h.Catalog.Preview(r.Context(), req.Catalog, req.RawHash)
	}
	if err != nil {
		catalogError(w, err, &report)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": report})
}

func catalogError(w http.ResponseWriter, err error, report *lootcatalog.Report) {
	status, code, message := http.StatusInternalServerError, "internal", "Katalog konnte nicht verarbeitet werden"
	switch {
	case errors.Is(err, lootcatalog.ErrInvalid):
		status, code, message = http.StatusUnprocessableEntity, "catalog_invalid", "Katalog oder Filter ist ungueltig"
	case errors.Is(err, lootcatalog.ErrConflict):
		status, code, message = http.StatusConflict, "catalog_conflict", "Katalog kollidiert mit vorhandenen Daten"
	case errors.Is(err, lootcatalog.ErrPreviewRequired):
		status, code, message = http.StatusConflict, "catalog_preview_required", "Bitte zuerst eine fehlerfreie Vorschau pruefen"
	case errors.Is(err, lootcatalog.ErrHashMismatch):
		status, code, message = http.StatusConflict, "catalog_hash_mismatch", "Katalog stimmt nicht mit dem geprueften Hash ueberein"
	}
	if status == http.StatusInternalServerError || report == nil {
		WriteError(w, status, code, message)
		return
	}
	WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}, "data": report})
}
