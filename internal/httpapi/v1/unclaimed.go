package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/raid"
	"raid-website/internal/unclaimedloot"
	"raid-website/internal/validation"
)

// UnclaimedHandlers expose loot that went to nobody (ADR 089).
type UnclaimedHandlers struct{ Unclaimed *unclaimedloot.Service }

// Register mounts the reads for signed-in users and the writes for raid staff.
func (h UnclaimedHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/raids/{id}/unclaimed-loot", RequireAuth(h.list))
	mux.HandleFunc("POST /api/v1/raids/{id}/unclaimed-loot", RequireRole(validation.RoleRaidassistent, h.create))
	mux.HandleFunc("POST /api/v1/unclaimed-loot/{id}/remove", RequireRole(validation.RoleRaidassistent, h.remove))
}

type unclaimedRequest struct {
	CatalogBossID string `json:"catalog_boss_id"`
	ItemID        string `json:"item_id"`
	ItemName      string `json:"item_name"`
	Outcome       string `json:"outcome"`
	HolderName    string `json:"holder_name"`
	Notes         string `json:"notes"`
	Reason        string `json:"reason"`
}

func (h UnclaimedHandlers) list(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	entries, err := h.Unclaimed.List(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list unclaimed loot")
		return
	}
	writeListJSON(w, entries)
}

func (h UnclaimedHandlers) create(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req unclaimedRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	entry, err := h.Unclaimed.Create(r.Context(), actor, id, unclaimedloot.Input{CatalogBossID: req.CatalogBossID, ItemID: req.ItemID,
		ItemName: req.ItemName, Outcome: unclaimedloot.Outcome(req.Outcome), HolderName: req.HolderName, Notes: req.Notes, Reason: req.Reason})
	if err != nil {
		unclaimedError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": entry})
}

type removeUnclaimedRequest struct {
	Reason string `json:"reason"`
}

func (h UnclaimedHandlers) remove(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid id")
		return
	}
	var req removeUnclaimedRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := h.Unclaimed.Remove(r.Context(), actor, id, req.Reason); err != nil {
		unclaimedError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func unclaimedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, unclaimedloot.ErrNotFound), errors.Is(err, raid.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Nicht gefunden")
	case errors.Is(err, unclaimedloot.ErrInvalid):
		WriteError(w, http.StatusUnprocessableEntity, "unclaimed_invalid", "Bitte Gegenstand und Verbleib angeben.")
	case errors.Is(err, unclaimedloot.ErrBoss):
		WriteError(w, http.StatusUnprocessableEntity, "unclaimed_boss_invalid", "Der Boss gehört nicht zur Instanz dieses Raids.")
	case errors.Is(err, unclaimedloot.ErrRaidState):
		WriteError(w, http.StatusConflict, "raid_state", "Ein abgesagter Raid nimmt keine Beute auf.")
	case errors.Is(err, unclaimedloot.ErrReasonRequired):
		WriteError(w, http.StatusUnprocessableEntity, "reason_required", "Für einen archivierten Raid ist ein Grund nötig.")
	case errors.Is(err, unclaimedloot.ErrAlreadyRemoved):
		WriteError(w, http.StatusConflict, "already_removed", "Dieser Eintrag ist schon entfernt.")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "Beute ohne Empfänger konnte nicht gespeichert werden.")
	}
}
