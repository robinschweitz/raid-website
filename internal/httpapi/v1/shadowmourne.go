package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/shadowmourne"
)

func writeShadowmourneError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shadowmourne.ErrNotFound), errors.Is(err, shadowmourne.ErrCharacterMissing):
		WriteError(w, http.StatusNotFound, "not_found", "Der Charakter oder Listenplatz wurde nicht gefunden.")
	case errors.Is(err, shadowmourne.ErrInvalidProgress):
		WriteError(w, http.StatusBadRequest, "invalid_progress", "Der Stand muss zwischen 0 und dem Ziel liegen; das Ziel muss größer als 0 sein.")
	case errors.Is(err, shadowmourne.ErrNotCurrent):
		WriteError(w, http.StatusConflict, "not_current_collector", "Nur der aktuell sammelnde Charakter kann fortgeschrieben oder abgeschlossen werden.")
	case errors.Is(err, shadowmourne.ErrProgressIncomplete):
		WriteError(w, http.StatusConflict, "shards_incomplete", "Schattengram kann erst bei erreichtem Splitterziel abgeschlossen werden.")
	case errors.Is(err, shadowmourne.ErrNotQueued), errors.Is(err, shadowmourne.ErrQueueBoundary):
		WriteError(w, http.StatusConflict, "queue_move_not_allowed", "Dieser Listenplatz kann nicht weiter verschoben werden.")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "Die Schattengram-Liste konnte nicht verarbeitet werden.")
	}
}

func (a *API) listShadowmourneQueue(w http.ResponseWriter, r *http.Request) {
	entries, err := a.Shadowmourne.List(r.Context())
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	writeListJSON(w, entries)
}

func (a *API) createShadowmourneQueueEntry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CharacterID     int64 `json:"character_id"`
		ShardsCollected int   `json:"shards_collected"`
		ShardTarget     int   `json:"shard_target"`
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
		ShardsCollected int `json:"shards_collected"`
		ShardTarget     int `json:"shard_target"`
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

func (a *API) moveShadowmourneEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid entry id")
		return
	}
	var req struct {
		Direction shadowmourne.Direction `json:"direction"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	entries, err := a.Shadowmourne.Move(r.Context(), actor, id, req.Direction)
	if err != nil {
		writeShadowmourneError(w, err)
		return
	}
	writeListJSON(w, entries)
}
