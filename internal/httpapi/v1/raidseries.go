package v1

import (
	"errors"
	"net/http"
	"time"

	"raid-website/internal/lootcatalog"
	"raid-website/internal/push"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
)

type seriesRequest struct {
	CatalogInstanceID *string `json:"catalog_instance_id"`
	RaidSize          *int    `json:"raid_size"`
	Name              *string `json:"name"`
	NameTemplate      *string `json:"name_template"`
	NextRaidNumber    *int    `json:"next_raid_number"`
	RaidLeaderID      *int64  `json:"raid_leader_id"`
	Weekdays          *[]int  `json:"weekdays"`
	IntervalWeeks     *int64  `json:"interval_weeks"`
	StartTime         *string `json:"start_time"`
	Timezone          *string `json:"timezone"`
	PlanWeeksAhead    *int    `json:"plan_weeks_ahead"`
}

func (a *API) listRaidSeries(w http.ResponseWriter, r *http.Request) {
	series, err := a.RaidSeries.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list raid series")
		return
	}
	writeListJSON(w, series)
}
func (a *API) createRaidSeries(w http.ResponseWriter, r *http.Request) { a.saveRaidSeries(w, r, false) }
func (a *API) updateRaidSeries(w http.ResponseWriter, r *http.Request) { a.saveRaidSeries(w, r, true) }
func (a *API) saveRaidSeries(w http.ResponseWriter, r *http.Request, update bool) {
	actor, _ := ActorID(r)
	id := int64(0)
	if update {
		var ok bool
		id, ok = pathID(r, "id")
		if !ok {
			BadRequest(w, "invalid series id")
			return
		}
	}
	var req seriesRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	series, err := raidseries.NewService(a.RaidSeries).Configure(r.Context(), id, raidseries.Changes{ActorID: actor, Name: req.Name, NameTemplate: req.NameTemplate, NextRaidNumber: req.NextRaidNumber, RaidLeaderID: req.RaidLeaderID, Weekdays: req.Weekdays, IntervalWeeks: req.IntervalWeeks, StartTime: req.StartTime, Timezone: req.Timezone, PlanWeeksAhead: req.PlanWeeksAhead, CatalogInstanceID: req.CatalogInstanceID, RaidSize: req.RaidSize})
	if err != nil {
		if !writeSeriesError(w, err) {
			WriteError(w, http.StatusInternalServerError, "internal", "could not save series")
		}
		return
	}
	// Saving the series also tops up its automatic planning window (ADR 078).
	planned, err := raid.NewService(a.Raids).PlanAhead(r.Context(), series.ID, actor)
	if err != nil {
		if !writeSeriesError(w, err) {
			WriteError(w, http.StatusInternalServerError, "internal", "series saved, but future raids could not be planned")
		}
		return
	}
	if len(planned) > 0 {
		// Planning advanced the name counter; return the series as stored now.
		if series, err = a.RaidSeries.Get(r.Context(), series.ID); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", "could not load series")
			return
		}
	}
	status := http.StatusCreated
	if update {
		status = http.StatusOK
	}
	WriteJSON(w, status, map[string]any{"data": series})
	if len(planned) > 0 {
		a.notifyAll(w, r, push.KindRaid, actor)
	}
}

type planSeriesRequest struct {
	Until string `json:"until"`
}

// planRaidSeries creates the series' raids up to a chosen day (ADR 078).
func (a *API) planRaidSeries(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorID(r)
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid series id")
		return
	}
	var req planSeriesRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	until, err := time.Parse("2006-01-02", req.Until)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_until", "until must be a date (YYYY-MM-DD)")
		return
	}
	created, err := raid.NewService(a.Raids).PlanSeriesUntil(r.Context(), id, actor, until)
	if err != nil {
		if !writeSeriesError(w, err) {
			WriteError(w, http.StatusInternalServerError, "internal", "could not plan raids")
		}
		return
	}
	writeListJSON(w, created)
	if len(created) > 0 {
		a.notifyAll(w, r, push.KindRaid, actor)
	}
}

func writeSeriesError(w http.ResponseWriter, err error) bool {
	code := ""
	status := http.StatusUnprocessableEntity
	switch {
	case errors.Is(err, lootcatalog.ErrInvalid):
		code = "catalog_invalid"
		status = http.StatusBadRequest
	case errors.Is(err, raid.ErrOccurrenceConflict):
		code = "occurrence_conflict"
		status = http.StatusConflict
	case errors.Is(err, raidseries.ErrNotFound):
		code = "series_not_found"
		status = http.StatusNotFound
	case errors.Is(err, raidseries.ErrInvalid):
		code = "invalid_series"
	case errors.Is(err, raidseries.ErrLeaderNotFound):
		code = "leader_not_found"
	case errors.Is(err, raidseries.ErrSeriesRequired):
		code = "series_required"
		status = http.StatusConflict
	case errors.Is(err, raidseries.ErrScheduledStartRequired):
		code = "scheduled_start_required"
		status = http.StatusConflict
	case errors.Is(err, raidseries.ErrLocalTimeNonexistent):
		code = "local_time_nonexistent"
		status = http.StatusConflict
	case errors.Is(err, raidseries.ErrLocalTimeAmbiguous):
		code = "local_time_ambiguous"
		status = http.StatusConflict
	case errors.Is(err, raid.ErrPlanUntilPast):
		code = "plan_until_past"
		status = http.StatusBadRequest
	case errors.Is(err, raid.ErrPlanTooFar):
		code = "plan_too_far"
		status = http.StatusBadRequest
	case errors.Is(err, raidseries.ErrScheduleRange):
		code = "schedule_range"
		status = http.StatusConflict
	default:
		return false
	}
	WriteError(w, status, code, err.Error())
	return true
}
