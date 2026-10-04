package v1

import (
	"encoding/json"
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
	SeriesID       *int64  `json:"series_id"`
}

type configureRaidScheduleRequest struct {
	SeriesID       *int64          `json:"series_id"`
	ScheduledStart json.RawMessage `json:"scheduled_start"`
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
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
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
		SeriesID:       req.SeriesID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := raid.NewService(a.Raids).CreateRaid(r.Context(), rd); err != nil {
		if writeSeriesError(w, err) {
			return
		}
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
		if errors.Is(err, raid.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "raid not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load raid")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rd})
}

func (a *API) cancelRaid(w http.ResponseWriter, r *http.Request) {
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
	rd, err := raid.NewService(a.Raids).CancelRaid(r.Context(), id, actor)
	if err != nil {
		switch {
		case errors.Is(err, raid.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "raid not found")
		case errors.Is(err, raid.ErrActorNotFound):
			WriteError(w, http.StatusUnprocessableEntity, "actor_not_found", "raid actor not found")
		case errors.Is(err, raid.ErrNotCancellable):
			WriteError(w, http.StatusConflict, "raid_not_cancellable", "raid is not in a cancellable state")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not cancel raid")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rd})
}

func (a *API) archiveRaid(w http.ResponseWriter, r *http.Request) {
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
	rd, next, err := a.Completion.ArchiveRaid(r.Context(), id, actor)
	if err != nil {
		if writeSeriesError(w, err) {
			return
		}
		switch {
		case errors.Is(err, raid.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "raid not found")
		case errors.Is(err, raid.ErrActorNotFound):
			WriteError(w, http.StatusUnprocessableEntity, "actor_not_found", "raid actor not found")
		case errors.Is(err, raidcompletion.ErrRaidCancelled):
			WriteError(w, http.StatusConflict, "raid_cancelled", "cancelled raids cannot be archived")
		case errors.Is(err, raidcompletion.ErrRaidNotArchivable):
			WriteError(w, http.StatusConflict, "raid_not_archivable", "raid cannot be archived")
		case errors.Is(err, raidcompletion.ErrRosterIncomplete):
			WriteError(w, http.StatusConflict, "roster_incomplete", err.Error())
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not archive raid")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rd, "next_raid": next})
}

func (a *API) configureRaidSchedule(w http.ResponseWriter, r *http.Request) {
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
	var req configureRaidScheduleRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	changes := raid.ScheduleChanges{SeriesID: req.SeriesID}
	if req.ScheduledStart != nil {
		var raw string
		if err := json.Unmarshal(req.ScheduledStart, &raw); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_scheduled_start", "scheduled_start must be a nonnull RFC3339 timestamp")
			return
		}
		start, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_scheduled_start", "scheduled_start must be a nonnull RFC3339 timestamp")
			return
		}
		changes.ScheduledStart = &start
	}
	rd, err := raid.NewService(a.Raids).ConfigureSchedule(r.Context(), id, actor, changes)
	if err != nil {
		if writeSeriesError(w, err) {
			return
		}
		switch {
		case errors.Is(err, raid.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "raid not found")
		case errors.Is(err, raid.ErrActorNotFound):
			WriteError(w, http.StatusUnprocessableEntity, "actor_not_found", "raid actor not found")
		case errors.Is(err, raid.ErrScheduleNotEditable):
			WriteError(w, http.StatusConflict, "schedule_not_editable", "finalized, cancelled or archived raid schedule cannot be changed")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not configure raid schedule")
		}
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
