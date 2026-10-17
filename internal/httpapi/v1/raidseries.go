package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/lootcatalog"
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
	series, err := raidseries.NewService(a.RaidSeries).Configure(r.Context(), id, raidseries.Changes{ActorID: actor, Name: req.Name, NameTemplate: req.NameTemplate, NextRaidNumber: req.NextRaidNumber, RaidLeaderID: req.RaidLeaderID, Weekdays: req.Weekdays, IntervalWeeks: req.IntervalWeeks, StartTime: req.StartTime, Timezone: req.Timezone, CatalogInstanceID: req.CatalogInstanceID, RaidSize: req.RaidSize})
	if err != nil {
		if !writeSeriesError(w, err) {
			WriteError(w, http.StatusInternalServerError, "internal", "could not save series")
		}
		return
	}
	status := http.StatusCreated
	if update {
		status = http.StatusOK
	}
	WriteJSON(w, status, map[string]any{"data": series})
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
	case errors.Is(err, raidseries.ErrScheduleRange):
		code = "schedule_range"
		status = http.StatusConflict
	default:
		return false
	}
	WriteError(w, status, code, err.Error())
	return true
}
