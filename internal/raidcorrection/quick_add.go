package raidcorrection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

// ErrQuickAddForbidden identifies callers without raid staff permissions.
var ErrQuickAddForbidden = errors.New("raid staff required")

// ErrPlayerExists requires explicit selection of an existing player.
var ErrPlayerExists = errors.New("player name already exists")

// ErrQuickAddRequestConflict prevents reusing a retry key for another payload.
var ErrQuickAddRequestConflict = errors.New("quick add request has different payload")

// QuickAdd describes a new character and its first attendance, without account powers.
type QuickAdd struct {
	RequestID  string                    `json:"request_id"`
	PlayerID   int64                     `json:"player_id"`
	PlayerName string                    `json:"player_name"`
	Name       string                    `json:"name"`
	Class      string                    `json:"class"`
	MainSpec   string                    `json:"main_spec"`
	SecondSpec string                    `json:"second_spec"`
	Role       validation.AttendanceRole `json:"role"`
	Reason     string                    `json:"reason"`
}

// AddNewCharacter creates identity and attendance atomically and supports safe retries.
func (s *Service) AddNewCharacter(ctx context.Context, raidID, actor int64, input QuickAdd) (*attendance.Attendance, error) {
	var result *attendance.Attendance
	err := s.repo.InTransaction(ctx, func(r *Repo) error {
		staff, err := r.players.Get(ctx, actor)
		if err != nil {
			return referenceError(err)
		}
		if !staff.Role.AtLeast(validation.RoleRaidassistent) {
			return ErrQuickAddForbidden
		}
		input.Name = strings.TrimSpace(input.Name)
		input.PlayerName = strings.TrimSpace(input.PlayerName)
		input.Class = validation.NormalizeClassName(input.Class)
		if input.Name == "" || strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 || input.PlayerID < 0 || (input.PlayerID > 0 && input.PlayerName != "") || !slices.Contains(validation.ClassSpecs[input.Class], input.MainSpec) || (input.SecondSpec != "" && !slices.Contains(validation.ClassSpecs[input.Class], input.SecondSpec)) {
			return ErrInvalid
		}
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		previous, id, err := r.quickAddRequest(ctx, raidID, actor, input.RequestID)
		if err == nil {
			if previous != string(payload) {
				return ErrQuickAddRequestConflict
			}
			result, err = r.attendance.Get(ctx, id)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.check(ctx, r, raidID, actor, input.Reason, false); err != nil {
			return err
		}
		now := time.Now().UTC()
		ownerID := input.PlayerID
		if ownerID == 0 {
			name := input.PlayerName
			if name == "" {
				name = input.Name
			}
			if _, err := r.players.FindByName(ctx, name); err == nil {
				return ErrPlayerExists
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			owner := &player.Player{Name: name, Role: validation.RoleGuest, Active: true, CreatedAt: now, UpdatedAt: now}
			if err := r.players.Create(ctx, owner); err != nil {
				return err
			}
			ownerID = owner.ID
			if err := s.log(ctx, r, "player", ownerID, actor, input.Reason, nil, owner); err != nil {
				return err
			}
		}
		if _, err := r.players.Get(ctx, ownerID); err != nil {
			return referenceError(err)
		}
		c := &character.Character{PlayerID: ownerID, Name: input.Name, Class: input.Class, MainSpec: input.MainSpec, SecondSpec: input.SecondSpec, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := r.characters.Create(ctx, c); err != nil {
			return err
		}
		if err := s.log(ctx, r, "character", c.ID, actor, input.Reason, nil, c); err != nil {
			return err
		}
		result = &attendance.Attendance{RaidID: raidID, PlayerID: ownerID, CharacterID: c.ID, Role: input.Role}
		if err := NewService(r).SaveAttendance(ctx, result, actor, input.Reason); err != nil {
			return err
		}
		if err := s.log(ctx, r, "raid_quick_add", result.ID, actor, input.Reason, nil, result); err != nil {
			return err
		}
		return r.saveQuickAddRequest(ctx, raidID, actor, input.RequestID, string(payload), result.ID)
	})
	return result, err
}
