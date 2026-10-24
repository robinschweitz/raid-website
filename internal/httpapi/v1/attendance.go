package v1

import (
	"errors"
	"net/http"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/validation"
)

type createAttendanceRequest struct {
	CharacterName *string `json:"character_name"`
	Class         *string `json:"class"`
	Spec          *string `json:"spec"`
	PlayerID      int64   `json:"player_id"`
	CharacterID   int64   `json:"character_id"`
	Role          string  `json:"role"`
	IsSubstitute  bool    `json:"is_substitute"`
	IsBench       bool    `json:"is_bench"`
	LateJoin      bool    `json:"late_join"`
	EarlyLeave    bool    `json:"early_leave"`
	Notes         string  `json:"notes"`
	Reason        string  `json:"reason"`
}

type correctAttendanceRequest struct {
	CharacterName *string `json:"character_name"`
	Class         *string `json:"class"`
	Spec          *string `json:"spec"`
	PlayerID      *int64  `json:"player_id"`
	CharacterID   *int64  `json:"character_id"`
	Role          *string `json:"role"`
	IsSubstitute  *bool   `json:"is_substitute"`
	IsBench       *bool   `json:"is_bench"`
	LateJoin      *bool   `json:"late_join"`
	EarlyLeave    *bool   `json:"early_leave"`
	Notes         *string `json:"notes"`
	Reason        string  `json:"reason"`
}

func (a *API) correctAttendance(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorID(r)
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	id, ok := pathID(r, "recordID")
	if !ok {
		BadRequest(w, "invalid attendance id")
		return
	}
	var req correctAttendanceRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	row, err := a.Corrections.CorrectAttendance(r.Context(), raidID, id, actor, req.Reason, func(row *attendance.Attendance) {
		if req.PlayerID != nil {
			row.PlayerID = *req.PlayerID
		}
		if req.CharacterID != nil {
			row.CharacterID = *req.CharacterID
		}
		if req.Role != nil {
			row.Role = validation.AttendanceRole(*req.Role)
		}
		if req.IsSubstitute != nil {
			row.IsSubstitute = *req.IsSubstitute
		}
		if req.IsBench != nil {
			row.IsBench = *req.IsBench
		}
		if req.LateJoin != nil {
			row.LateJoin = *req.LateJoin
		}
		if req.EarlyLeave != nil {
			row.EarlyLeave = *req.EarlyLeave
		}
		if req.Notes != nil {
			row.Notes = *req.Notes
		}
	}, raidcorrection.SnapshotPatch{CharacterName: req.CharacterName, Class: req.Class, Spec: req.Spec})
	if err != nil {
		writeCorrectionError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": row})
}

// removeAttendance hard-deletes a roster row from a planned or open raid (ADR 076).
func (a *API) removeAttendance(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorID(r)
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	id, ok := pathID(r, "recordID")
	if !ok {
		BadRequest(w, "invalid attendance id")
		return
	}
	if err := a.Corrections.RemoveAttendance(r.Context(), raidID, id, actor); err != nil {
		writeCorrectionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listAttendance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	rows, err := a.Attendance.ListByRaid(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list attendance")
		return
	}
	writeListJSON(w, rows)
}

func (a *API) createAttendance(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req createAttendanceRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.PlayerID <= 0 {
		BadRequest(w, "player_id is required")
		return
	}
	if req.CharacterID <= 0 {
		BadRequest(w, "character_id is required")
		return
	}
	role := validation.AttendanceRole(req.Role)
	switch role {
	case validation.RoleTank, validation.RoleHeal, validation.RoleDD:
	default:
		BadRequest(w, "role must be TANK, HEAL or DD")
		return
	}
	now := time.Now().UTC()
	row := &attendance.Attendance{
		RaidID:       raidID,
		PlayerID:     req.PlayerID,
		CharacterID:  req.CharacterID,
		Role:         role,
		IsSubstitute: req.IsSubstitute,
		IsBench:      req.IsBench,
		LateJoin:     req.LateJoin,
		EarlyLeave:   req.EarlyLeave,
		Notes:        req.Notes,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	before := a.rosterSnapshot(r.Context(), raidID)
	if err := a.Corrections.SaveAttendance(r.Context(), row, actor, req.Reason, raidcorrection.SnapshotPatch{CharacterName: req.CharacterName, Class: req.Class, Spec: req.Spec}); err != nil {
		writeCorrectionError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": row})
	a.notifyNewRosterPlayers(w, r, raidID, actor, before)
}

func writeCorrectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, loot.ErrCatalogContextRequired):
		WriteError(w, http.StatusConflict, "catalog_context_required", "Raidinstanz und Raidgroesse muessen vor der Lootauswahl konfiguriert werden")
	case errors.Is(err, lootcatalog.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "catalog_item_invalid", "Bitte einen Katalogboss und einen passenden Gegenstand fuer die Raidinstanz und Raidgroesse auswaehlen")
	case errors.Is(err, enhancepoints.ErrInsufficientBalance):
		WriteError(w, http.StatusUnprocessableEntity, "insufficient_balance", "Spieler hat keine EP fuer Enhanced-Loot")
	case errors.Is(err, loot.ErrInvalidMethod):
		WriteError(w, http.StatusBadRequest, "invalid_award_method", "award_method muss MAIN, SECOND, ENHANCED oder BOE sein")
	case errors.Is(err, loot.ErrItemName):
		WriteError(w, http.StatusBadRequest, "item_name_required", "item_name ist erforderlich")
	case errors.Is(err, loot.ErrRoster):
		WriteError(w, http.StatusBadRequest, "recipient_not_in_roster", "Empfaengercharakter muss im Raidkader stehen")
	case errors.Is(err, loot.ErrRequestConflict), errors.Is(err, loot.ErrAlreadyRecorded):
		WriteError(w, http.StatusConflict, "request_conflict", "request_id wurde mit anderem Inhalt verwendet")
	case errors.Is(err, loot.ErrRequestRequired):
		WriteError(w, http.StatusBadRequest, "request_id_required", "Neues Enhanced-Loot erfordert eine nichtleere request_id")
	case errors.Is(err, raidcorrection.ErrNotFound), errors.Is(err, loot.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Referenzierter Datensatz nicht gefunden")
	case errors.Is(err, raidcorrection.ErrOwnership), errors.Is(err, loot.ErrOwnership):
		WriteError(w, http.StatusBadRequest, "character_ownership", "Charakter gehoert nicht zum Spieler")
	case errors.Is(err, raidcorrection.ErrReason):
		WriteError(w, http.StatusBadRequest, "reason_required", "Ein Akteur ist erforderlich; Aenderungen im Archiv brauchen eine nichtleere Begruendung")
	case errors.Is(err, raidcorrection.ErrInvalid), errors.Is(err, loot.ErrInvalidLock):
		WriteError(w, http.StatusBadRequest, "invalid_correction", "Ungueltige Korrektur oder unpassender Item-Lock")
	case errors.Is(err, raidcorrection.ErrRaidState), errors.Is(err, loot.ErrRaidState):
		WriteError(w, http.StatusConflict, "raid_state", "Raid erlaubt diese Aenderung nicht")
	case errors.Is(err, loot.ErrLockConflict):
		WriteError(w, http.StatusConflict, "item_lock_conflict", "Mehrere aktive Item-Locks passen auf dieses Item")
	case errors.Is(err, raidcorrection.ErrHasLoot):
		WriteError(w, http.StatusConflict, "attendance_has_loot", "Charakter hat in diesem Raid noch Loot; zuerst den Loot zurueckziehen")
	case errors.Is(err, raidcorrection.ErrConflict):
		WriteError(w, http.StatusConflict, "attendance_conflict", "Teilnahme ist bereits vorhanden")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "Datensatz konnte nicht gespeichert werden")
	}
}
