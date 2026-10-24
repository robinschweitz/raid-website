package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/bosskill"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/validation"
)

// BossKillHandlers expose the killed bosses of a raid (FR-29).
type BossKillHandlers struct {
	Kills       *bosskill.Service
	Corrections *raidcorrection.Service
}

// Register mounts the read for signed-in users and the writes for raid staff.
func (h BossKillHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/raids/{id}/kills", RequireAuth(h.list))
	mux.HandleFunc("POST /api/v1/raids/{id}/kills", RequireRole(validation.RoleRaidassistent, h.create))
	mux.HandleFunc("PATCH /api/v1/raids/{id}/kills/{killID}", RequireRole(validation.RoleRaidassistent, h.update))
	mux.HandleFunc("POST /api/v1/raids/{id}/kills/{killID}/remove", RequireRole(validation.RoleRaidassistent, h.remove))
}

type killRequest struct {
	CatalogBossID string `json:"catalog_boss_id"`
	Difficulty    string `json:"difficulty"`
	Reason        string `json:"reason"`
}

type killChangeRequest struct {
	Difficulty string `json:"difficulty"`
	Reason     string `json:"reason"`
}

type killRemoveRequest struct {
	Reason string `json:"reason"`
}

func (h BossKillHandlers) list(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	kills, err := h.Kills.List(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list boss kills")
		return
	}
	writeListJSON(w, kills)
}

func (h BossKillHandlers) create(w http.ResponseWriter, r *http.Request) {
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
	var req killRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	kill, err := h.Corrections.RecordKill(r.Context(), id, actor, req.CatalogBossID, bosskill.Difficulty(req.Difficulty), req.Reason)
	if err != nil {
		writeKillError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": kill})
}

func (h BossKillHandlers) update(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	id, ok := pathID(r, "id")
	killID, killOK := pathID(r, "killID")
	if !ok || !killOK {
		BadRequest(w, "invalid id")
		return
	}
	var req killChangeRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	kill, err := h.Corrections.ChangeKillDifficulty(r.Context(), id, killID, actor, bosskill.Difficulty(req.Difficulty), req.Reason)
	if err != nil {
		writeKillError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": kill})
}

func (h BossKillHandlers) remove(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	id, ok := pathID(r, "id")
	killID, killOK := pathID(r, "killID")
	if !ok || !killOK {
		BadRequest(w, "invalid id")
		return
	}
	var req killRemoveRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := h.Corrections.RemoveKill(r.Context(), id, killID, actor, req.Reason); err != nil {
		writeKillError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeKillError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bosskill.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Bosskill nicht gefunden.")
	case errors.Is(err, bosskill.ErrExists):
		WriteError(w, http.StatusConflict, "kill_exists", "Dieser Boss ist in diesem Raid schon als getötet eingetragen.")
	case errors.Is(err, bosskill.ErrInvalidBoss):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_boss", "Der Boss gehört nicht zur Instanz dieses Raids.")
	case errors.Is(err, bosskill.ErrInvalidDifficulty):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_difficulty", "Diese Schwierigkeit gibt es für diesen Boss und diese Raidgröße nicht.")
	case errors.Is(err, bosskill.ErrHasLoot):
		WriteError(w, http.StatusConflict, "kill_has_loot", "Für diesen Boss gibt es noch Beute; zuerst die Beute zurückziehen.")
	default:
		writeCorrectionError(w, err)
	}
}
