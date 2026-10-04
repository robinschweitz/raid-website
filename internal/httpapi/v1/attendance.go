package v1

import (
	"net/http"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/validation"
)

type createAttendanceRequest struct {
	PlayerID     int64  `json:"player_id"`
	CharacterID  int64  `json:"character_id"`
	Role         string `json:"role"`
	IsSubstitute bool   `json:"is_substitute"`
	IsBench      bool   `json:"is_bench"`
	LateJoin     bool   `json:"late_join"`
	EarlyLeave   bool   `json:"early_leave"`
	Notes        string `json:"notes"`
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
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
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
	if err := a.Attendance.Create(r.Context(), row); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create attendance")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": row})
}
