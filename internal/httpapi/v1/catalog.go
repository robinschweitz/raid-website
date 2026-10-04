package v1

import (
	"errors"
	"net/http"
	"strconv"

	"raid-website/internal/lootcatalog"
)

// CatalogHandlers exposes authenticated reads of the compiled catalog.
type CatalogHandlers struct{ Catalog *lootcatalog.Service }

// Register mounts reads only; catalog mutations are not exposed.
func (h CatalogHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/loot-catalog", RequireAuth(h.list))
	mux.HandleFunc("GET /api/v1/loot-catalog/instances", RequireAuth(h.instances))
	mux.HandleFunc("GET /api/v1/loot-catalog/bosses", RequireAuth(h.bosses))
	mux.HandleFunc("GET /api/v1/loot-catalog/items", RequireAuth(h.items))
}

func (h CatalogHandlers) list(w http.ResponseWriter, r *http.Request) {
	c, err := h.Catalog.List(r.Context())
	if err != nil {
		catalogError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (h CatalogHandlers) instances(w http.ResponseWriter, r *http.Request) {
	items, err := h.Catalog.Instances(r.Context())
	if err != nil {
		catalogError(w, err)
		return
	}
	writeListJSON(w, items)
}

func (h CatalogHandlers) bosses(w http.ResponseWriter, r *http.Request) {
	items, err := h.Catalog.Bosses(r.Context(), r.URL.Query().Get("instance_id"))
	if err != nil {
		catalogError(w, err)
		return
	}
	writeListJSON(w, items)
}

// items returns selectable drops for a boss, raid size, and gameplay mode. Raw
// source paths are never a selection: they stay attached to each merged item.
func (h CatalogHandlers) items(w http.ResponseWriter, r *http.Request) {
	filter := lootcatalog.Filter{
		BossID: r.URL.Query().Get("boss_id"),
		Mode:   lootcatalog.Mode(r.URL.Query().Get("mode")),
	}
	if raw := r.URL.Query().Get("raid_size"); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil || size != 10 && size != 25 {
			catalogError(w, lootcatalog.ErrInvalid)
			return
		}
		filter.RaidSize = size
	}
	items, err := h.Catalog.Items(r.Context(), filter)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeListJSON(w, items)
}

func catalogError(w http.ResponseWriter, err error) {
	if errors.Is(err, lootcatalog.ErrInvalid) {
		WriteError(w, http.StatusUnprocessableEntity, "catalog_invalid", "Katalog oder Filter ist ungueltig")
		return
	}
	WriteError(w, http.StatusInternalServerError, "internal", "Katalog konnte nicht verarbeitet werden")
}
