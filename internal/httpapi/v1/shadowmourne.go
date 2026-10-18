package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/shadowmourne"
)

func writeShadowmourneError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shadowmourne.ErrNotFound), errors.Is(err, shadowmourne.ErrCharacterMissing):
		WriteError(w, http.StatusNotFound, "not_found", "Der Charakter oder Listeneintrag wurde nicht gefunden.")
	case errors.Is(err, shadowmourne.ErrRaidMissing):
		WriteError(w, http.StatusNotFound, "not_found", "Der Raid wurde nicht gefunden.")
	case errors.Is(err, shadowmourne.ErrInvalidProgress):
		WriteError(w, http.StatusBadRequest, "invalid_progress", "Ungültiger Stand: Die Splitter müssen zwischen 0 und dem Ziel liegen, das Ziel muss größer als 0 sein.")
	case errors.Is(err, shadowmourne.ErrInvalidPosition):
		WriteError(w, http.StatusBadRequest, "invalid_position", "Diesen Listenplatz gibt es nicht.")
	case errors.Is(err, shadowmourne.ErrInvalidTask):
		WriteError(w, http.StatusBadRequest, "invalid_task", "Diesen Schattengram-Schritt gibt es nicht.")
	case errors.Is(err, shadowmourne.ErrClassNotEligible):
		WriteError(w, http.StatusBadRequest, "class_not_eligible", "Schattengram können nur Krieger, Paladine und Todesritter tragen.")
	case errors.Is(err, shadowmourne.ErrAlreadyListed):
		WriteError(w, http.StatusConflict, "already_listed", "Dieser Charakter steht schon auf der Liste.")
	case errors.Is(err, shadowmourne.ErrNotActive):
		WriteError(w, http.StatusConflict, "entry_not_active", "Dieser Eintrag ist schon fertig oder entfernt.")
	case errors.Is(err, shadowmourne.ErrProgressIncomplete):
		WriteError(w, http.StatusConflict, "shards_incomplete", "Fertig geht erst, wenn alle Splitter gesammelt sind.")
	case errors.Is(err, shadowmourne.ErrRaidLocked):
		WriteError(w, http.StatusConflict, "raid_locked", "Bei abgesagten oder archivierten Raids kann nichts mehr eingeteilt werden.")
	case errors.Is(err, shadowmourne.ErrNotInRoster):
		WriteError(w, http.StatusConflict, "not_in_roster", "Dieser Charakter ist nicht in der Aufstellung des Raids.")
	case errors.Is(err, shadowmourne.ErrNotTaskOption):
		WriteError(w, http.StatusConflict, "not_task_option", "Dieser Charakter passt nicht zu diesem Schritt: Er steht auf der Liste bei einem anderen Schritt oder kann Schattengram nicht tragen.")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "Die Schattengram-Liste konnte nicht verarbeitet werden.")
	}
}

func (a *API) listShadowmourne(w http.ResponseWriter, r *http.Request) {
	entries, err := a.Shadowmourne.List(r.Context())
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	writeListJSON(w, entries)
}

func (a *API) createShadowmourneEntry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CharacterID     int64              `json:"character_id"`
		Position        int                `json:"position"`
		Stage           shadowmourne.Stage `json:"stage"`
		ShardsCollected int                `json:"shards_collected"`
		ShardTarget     int                `json:"shard_target"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	entry, err := a.Shadowmourne.Create(r.Context(), actor, shadowmourne.CreateInput(req))
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": entry})
}

func (a *API) updateShadowmourneProgress(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid entry id")
		return
	}
	var req struct {
		Stage           shadowmourne.Stage `json:"stage"`
		ShardsCollected int                `json:"shards_collected"`
		ShardTarget     int                `json:"shard_target"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	entry, err := a.Shadowmourne.UpdateProgress(r.Context(), actor, id, shadowmourne.ProgressInput(req))
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": entry})
}

func (a *API) completeShadowmourneEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid entry id")
		return
	}
	actor, _ := ActorID(r)
	entry, err := a.Shadowmourne.Complete(r.Context(), actor, id)
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": entry})
}

func (a *API) removeShadowmourneEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid entry id")
		return
	}
	actor, _ := ActorID(r)
	if err := a.Shadowmourne.Remove(r.Context(), actor, id); err != nil {
		writeShadowmourneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) moveShadowmourneEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid entry id")
		return
	}
	var req struct {
		Position int `json:"position"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	entries, err := a.Shadowmourne.Move(r.Context(), actor, id, req.Position)
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	writeListJSON(w, entries)
}

func (a *API) getRaidShadowmourne(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	plan, err := a.Shadowmourne.RaidPlan(r.Context(), id)
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": plan})
}

func (a *API) assignRaidShadowmourneTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req struct {
		CharacterID *int64 `json:"character_id"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	plan, err := a.Shadowmourne.AssignRaidTask(r.Context(), actor, id, shadowmourne.Stage(r.PathValue("task")), req.CharacterID)
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": plan})
}
