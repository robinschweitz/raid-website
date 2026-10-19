package v1

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/validation"
)

type createRaidRequest struct {
	CatalogInstanceID *string `json:"catalog_instance_id"`
	RaidSize          *int    `json:"raid_size"`
	Name              string  `json:"name"`
	RaidLeaderID      int64   `json:"raid_leader_id"`
	Status            string  `json:"status"`
	ScheduledStart    *string `json:"scheduled_start"`
	ScheduledEnd      *string `json:"scheduled_end"`
	Notes             string  `json:"notes"`
	SeriesID          *int64  `json:"series_id"`
}

type configureRaidScheduleRequest struct {
	Name              *string         `json:"name"`
	CatalogInstanceID *string         `json:"catalog_instance_id"`
	RaidSize          *int            `json:"raid_size"`
	SeriesID          *int64          `json:"series_id"`
	ScheduledStart    json.RawMessage `json:"scheduled_start"`
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

// currentRaid answers which raid the "Aktueller Raid" link opens (ADR 088).
func (a *API) currentRaid(w http.ResponseWriter, r *http.Request) {
	rs, err := a.Raids.List(r.Context(), true)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list raids")
		return
	}
	current := raid.Current(rs, time.Now().UTC())
	if current == nil {
		WriteError(w, http.StatusNotFound, "no_current_raid", "no raid has begun or is planned")
		return
	}
	WriteJSON(w, http.StatusOK, current)
}

func (a *API) createRaid(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	var req createRaidRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
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
		CatalogInstanceID: req.CatalogInstanceID,
		RaidSize:          req.RaidSize,
		Name:              req.Name,
		RaidLeaderID:      req.RaidLeaderID,
		Status:            status,
		ScheduledStart:    start,
		ScheduledEnd:      end,
		Notes:             req.Notes,
		SeriesID:          req.SeriesID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := raid.NewService(a.Raids).CreateRaid(r.Context(), rd, actor); err != nil {
		if errors.Is(err, raid.ErrNameRequired) {
			WriteError(w, http.StatusBadRequest, "name_required", "name is required")
			return
		}
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
	var req struct {
		WithoutPoints []int64 `json:"without_points"`
	}
	if err := DecodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		BadRequest(w, "malformed json body")
		return
	}
	rd, next, err := a.Completion.ArchiveRaidWithout(r.Context(), id, actor, req.WithoutPoints)
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
		case errors.Is(err, raidcompletion.ErrInvalidExclusion):
			WriteError(w, http.StatusBadRequest, "invalid_exclusion", "Nur Spieler aus der Aufstellung, die abgesagt haben, können von den EP ausgenommen werden.")
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
	changes := raid.ScheduleChanges{Name: req.Name, SeriesID: req.SeriesID, CatalogInstanceID: req.CatalogInstanceID, RaidSize: req.RaidSize}
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
		case errors.Is(err, raid.ErrNameRequired):
			WriteError(w, http.StatusBadRequest, "name_required", "name is required")
		case errors.Is(err, raid.ErrNameNotEditable):
			WriteError(w, http.StatusConflict, "name_not_editable", "raid name can only be changed for raids without series")
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

func (a *API) archivePreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	rows, err := a.Completion.PreviewArchive(r.Context(), id)
	switch {
	case errors.Is(err, raid.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "raid not found")
	case errors.Is(err, raidcompletion.ErrRaidNotArchivable):
		WriteError(w, http.StatusConflict, "raid_not_archivable", "raid cannot be archived")
	case err != nil:
		WriteError(w, http.StatusInternalServerError, "internal", "could not preview archive")
	default:
		WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
	}
}
