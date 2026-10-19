package v1

import (
	"errors"
	"net/http"
	"strconv"

	"raid-website/internal/lootcatalog"
	"raid-website/internal/validation"
)

// CatalogHandlers exposes authenticated reads of the compiled catalog.
type CatalogHandlers struct{ Catalog *lootcatalog.Service }

// Register mounts reads; the catalog itself is never changed here. The boss
// order is guild configuration beside it (ADR 088).
func (h CatalogHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/v1/loot-catalog/boss-order", RequireRole(validation.RoleAdmin, h.setBossOrder))
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
	// Bosses in the Admin-set order (ADR 088); Bosses returns a copy.
	if c.Bosses, err = h.Catalog.Bosses(r.Context(), ""); err != nil {
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

type bossOrderRequest struct {
	InstanceID string   `json:"instance_id"`
	BossIDs    []string `json:"boss_ids"`
}

func (h CatalogHandlers) setBossOrder(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	var req bossOrderRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := h.Catalog.SetBossOrder(r.Context(), actor, req.InstanceID, req.BossIDs); err != nil {
		if errors.Is(err, lootcatalog.ErrBossOrderInvalid) {
			WriteError(w, http.StatusUnprocessableEntity, "boss_order_invalid", "Die Reihenfolge muss jeden Boss der Instanz genau einmal enthalten.")
			return
		}
		catalogError(w, err)
		return
	}
	items, err := h.Catalog.Bosses(r.Context(), req.InstanceID)
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
