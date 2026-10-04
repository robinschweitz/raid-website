package v1

import (
	"errors"
	"net/http"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/validation"
)

type createRaidRequest struct {
	Name           string  `json:"name"`
	RaidLeaderID   int64   `json:"raid_leader_id"`
	Status         string  `json:"status"`
	ScheduledStart *string `json:"scheduled_start"`
	ScheduledEnd   *string `json:"scheduled_end"`
	Notes          string  `json:"notes"`
}

func (a *API) listRaids(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	rs, err := a.Raids.List(r.Context(), includeArchived)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list raids")
		return
	}
	writeListJSON(w, rs)
}

func (a *API) createRaid(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	var req createRaidRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	if req.RaidLeaderID <= 0 {
		BadRequest(w, "raid_leader_id is required")
		return
	}
	status := validation.RaidStatusPlanned
	if req.Status != "" {
		status = validation.RaidStatus(req.Status)
	}
	switch status {
	case validation.RaidStatusPlanned, validation.RaidStatusOpen:
	default:
		BadRequest(w, "status must be PLANNED or OPEN")
		return
	}
	start, err := parseOptionalTime(req.ScheduledStart)
	if err != nil {
		BadRequest(w, "scheduled_start must be RFC3339")
		return
	}
	end, err := parseOptionalTime(req.ScheduledEnd)
	if err != nil {
		BadRequest(w, "scheduled_end must be RFC3339")
		return
	}
	now := time.Now().UTC()
	rd := &raid.Raid{
		Name:           req.Name,
		RaidLeaderID:   req.RaidLeaderID,
		Status:         status,
		ScheduledStart: start,
		ScheduledEnd:   end,
		Notes:          req.Notes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := a.Raids.Create(r.Context(), rd); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create raid")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": rd})
}

func (a *API) getRaid(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	rd, err := a.Raids.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "raid not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load raid")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rd})
}

func (a *API) completeRaid(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	if err := a.Completion.CompleteRaid(r.Context(), id, actor); err != nil {
		switch {
		case errors.Is(err, raidcompletion.ErrRaidCancelled):
			WriteError(w, http.StatusConflict, "raid_cancelled", "cancelled raids cannot be completed")
		case errors.Is(err, raidcompletion.ErrRaidNotCompletable):
			WriteError(w, http.StatusConflict, "raid_not_completable", "raid is not in a completable state")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not complete raid")
		}
		return
	}
	rd, err := a.Raids.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load raid")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rd})
}

func (a *API) cancelRaid(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	rd, err := a.Raids.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "raid not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load raid")
		return
	}
	if rd.Status == validation.RaidStatusCompleted || rd.Status == validation.RaidStatusCancelled {
		WriteError(w, http.StatusConflict, "raid_not_cancellable", "completed or cancelled raids cannot be cancelled")
		return
	}
	now := time.Now().UTC()
	rd.Status = validation.RaidStatusCancelled
	rd.CancelledAt = &now
	rd.UpdatedAt = now
	if err := a.Raids.Update(r.Context(), rd); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not cancel raid")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rd})
}

func (a *API) archiveRaid(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	rd, err := a.Raids.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "raid not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load raid")
		return
	}
	now := time.Now().UTC()
	rd.ArchivedAt = &now
	rd.UpdatedAt = now
	if err := a.Raids.Update(r.Context(), rd); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not archive raid")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rd})
}

func parseOptionalTime(raw *string) (*time.Time, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, err
	}
	u := t.UTC()
	return &u, nil
}
