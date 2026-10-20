package raidsignup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/validation"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrForbidden = errors.New("signup permission denied")
	ErrClosed    = errors.New("signups are closed")
	ErrInvalid   = errors.New("invalid character or spec")
	ErrConflict  = errors.New("roster changed or character already accepted")
	ErrNotFound  = errors.New("signup reference not found")
	ErrRaidState = errors.New("raid is no longer active")
	ErrTooLate   = errors.New("raid has already started")
)

// Choice is a player's requested character and spec.
type Choice struct {
	CharacterID int64  `json:"character_id"`
	Spec        string `json:"spec"`
}

// Entry presents a signup together with its current roster association.
type Entry struct {
	ID            int64  `json:"id"`
	PlayerID      int64  `json:"player_id"`
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name"`
	Class         string `json:"class"`
	Spec          string `json:"spec"`
	IsWithdrawn   bool   `json:"is_withdrawn"`
	IsAccepted    bool   `json:"is_accepted"`
	AttendanceID  int64  `json:"attendance_id"`
}

// Absence marks a player who announced not to attend (DECLINED) or who is not
// sure yet (TENTATIVE, ADR 082). It never changes the roster.
type Absence struct {
	PlayerID  int64     `json:"player_id"`
	Reason    string    `json:"reason"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Absence statuses. Only DECLINED affects EP at archive (ADR 082).
const (
	AbsenceDeclined  = "DECLINED"
	AbsenceTentative = "TENTATIVE"
)

// MaxAbsenceReason limits the optional free-text reason in characters.
const MaxAbsenceReason = 200

// Overview includes effective availability, evaluated against the server clock.
type Overview struct {
	IsEnabled  bool                `json:"is_enabled"`
	IsOpen     bool                `json:"is_open"`
	Entries    []Entry             `json:"entries"`
	ClassSpecs map[string][]string `json:"class_specs"`
	Absences   []Absence           `json:"absences"`
}

// Service coordinates signup changes and audited roster acceptance.
type Service struct {
	repo *Repo
	now  func() time.Time
}

// NewService constructs a signup service using the UTC server clock.
func NewService(repo *Repo) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

func reference(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func active(rd *raid.Raid) bool {
	return rd.ArchivedAt == nil && (rd.Status == validation.RaidStatusPlanned || rd.Status == validation.RaidStatusOpen)
}

func (s *Service) authorize(ctx context.Context, r *Repo, actor int64, role validation.Role) error {
	p, err := r.players.Get(ctx, actor)
	if err != nil {
		return reference(err)
	}
	if !p.Active || !p.Role.AtLeast(role) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) log(ctx context.Context, r *Repo, raidID, actor int64, action string, before, after any) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	return r.audit.Insert(ctx, &audit.Log{ActorID: &actor, EntityType: "raid_signup", EntityID: strconv.FormatInt(raidID, 10), Action: action, BeforeJSON: string(b), AfterJSON: string(a), CreatedAt: s.now()})
}

// Overview returns signups and the authoritative class/spec options.
func (s *Service) Overview(ctx context.Context, raidID int64) (*Overview, error) {
	var result *Overview
	err := s.repo.transaction(ctx, func(r *Repo) error {
		rd, err := r.raids.Get(ctx, raidID)
		if err != nil {
			return reference(err)
		}
		enabled, err := r.enabled(ctx, raidID)
		if err != nil {
			return err
		}
		rows, err := r.list(ctx, raidID)
		if err != nil {
			return err
		}
		roster, err := r.attendance.ListByRaid(ctx, raidID)
		if err != nil {
			return err
		}
		result = &Overview{IsEnabled: enabled, IsOpen: enabled && active(rd) && rd.ScheduledStart != nil && s.now().Before(*rd.ScheduledStart), Entries: []Entry{}, ClassSpecs: validation.ClassSpecs, Absences: []Absence{}}
		for _, row := range rows {
			c, err := r.characters.Get(ctx, row.CharacterID)
			if err != nil {
				return reference(err)
			}
			entry := Entry{ID: row.ID, PlayerID: row.PlayerID, CharacterID: row.CharacterID, CharacterName: c.Name, Class: attendance.NormalizeClass(c.Class), Spec: row.Spec, IsWithdrawn: row.IsWithdrawn}
			for _, a := range roster {
				if a.PlayerID == row.PlayerID {
					entry.AttendanceID = a.ID
					if a.CharacterID == row.CharacterID {
						entry.IsAccepted = true
						entry.Spec = a.Spec
					}
				}
			}
			result.Entries = append(result.Entries, entry)
		}
		abs, err := r.absences(ctx, raidID)
		if err != nil {
			return err
		}
		for _, a := range abs {
			result.Absences = append(result.Absences, Absence{PlayerID: a.PlayerID, Reason: a.Reason, Status: a.Status, CreatedAt: a.CreatedAt})
		}
		return nil
	})
	return result, err
}

// SetEnabled opens future signups or closes them early without changing raid lifecycle.
func (s *Service) SetEnabled(ctx context.Context, raidID, actor int64, enabled bool) error {
	return s.repo.transaction(ctx, func(r *Repo) error {
		if err := s.authorize(ctx, r, actor, validation.RoleRaidassistent); err != nil {
			return err
		}
		rd, err := r.raids.Get(ctx, raidID)
		if err != nil {
			return reference(err)
		}
		if !active(rd) {
			return ErrRaidState
		}
		if enabled && (rd.ScheduledStart == nil || !s.now().Before(*rd.ScheduledStart)) {
			return ErrClosed
		}
		before, err := r.enabled(ctx, raidID)
		if err != nil {
			return err
		}
		if err := r.setEnabled(ctx, raidID, enabled); err != nil {
			return err
		}
		return s.log(ctx, r, raidID, actor, "availability", before, enabled)
	})
}

func (s *Service) checkOpen(ctx context.Context, r *Repo, raidID, actor int64) error {
	if err := s.authorize(ctx, r, actor, validation.RoleMember); err != nil {
		return err
	}
	rd, err := r.raids.Get(ctx, raidID)
	if err != nil {
		return reference(err)
	}
	enabled, err := r.enabled(ctx, raidID)
	if err != nil {
		return err
	}
	if !enabled || !active(rd) || rd.ScheduledStart == nil || !s.now().Before(*rd.ScheduledStart) {
		return ErrClosed
	}
	return nil
}

// Save submits multiple alternatives atomically; other existing choices are preserved.
func (s *Service) Save(ctx context.Context, raidID, actor int64, choices []Choice) error {
	if len(choices) == 0 {
		return ErrInvalid
	}
	return s.repo.transaction(ctx, func(r *Repo) error {
		if err := s.checkOpen(ctx, r, raidID, actor); err != nil {
			return err
		}
		rows, err := r.list(ctx, raidID)
		if err != nil {
			return err
		}
		roster, err := r.attendance.ListByRaid(ctx, raidID)
		if err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, choice := range choices {
			if seen[choice.CharacterID] {
				return ErrInvalid
			}
			seen[choice.CharacterID] = true
			c, err := r.characters.Get(ctx, choice.CharacterID)
			if err != nil {
				return reference(err)
			}
			if c.PlayerID != actor {
				return ErrForbidden
			}
			spec := strings.TrimSpace(choice.Spec)
			if !slices.Contains(validation.ClassSpecs[attendance.NormalizeClass(c.Class)], spec) {
				return ErrInvalid
			}
			for _, a := range roster {
				if a.PlayerID == actor && a.CharacterID == c.ID {
					return ErrConflict
				}
			}
			now := s.now()
			row := signupRow{RaidID: raidID, PlayerID: actor, CharacterID: c.ID, Spec: spec, CreatedAt: now, UpdatedAt: now}
			var before any
			for _, existing := range rows {
				if existing.CharacterID == c.ID {
					if existing.PlayerID != actor {
						return ErrConflict
					}
					before = existing
					row.ID = existing.ID
					row.CreatedAt = existing.CreatedAt
				}
			}
			if err := r.save(ctx, &row); err != nil {
				return err
			}
			if err := s.log(ctx, r, raidID, actor, "submit", before, row); err != nil {
				return err
			}
		}
		return nil
	})
}

// Withdraw retains the signup record and rejects changes to confirmed characters.
func (s *Service) Withdraw(ctx context.Context, raidID, id, actor int64) error {
	return s.repo.transaction(ctx, func(r *Repo) error {
		if err := s.checkOpen(ctx, r, raidID, actor); err != nil {
			return err
		}
		row, err := r.get(ctx, raidID, id)
		if err != nil {
			return reference(err)
		}
		if row.PlayerID != actor {
			return ErrForbidden
		}
		roster, err := r.attendance.ListByRaid(ctx, raidID)
		if err != nil {
			return err
		}
		for _, a := range roster {
			if a.PlayerID == actor && a.CharacterID == row.CharacterID {
				return ErrConflict
			}
		}
		before := *row
		row.IsWithdrawn = true
		row.UpdatedAt = s.now()
		if err := r.save(ctx, row); err != nil {
			return err
		}
		return s.log(ctx, r, raidID, actor, "withdraw", before, row)
	})
}

// SpecRole is the roster role a spec plays: tank specs tank, healing specs
// heal, everything else is DD.
func SpecRole(spec string) validation.AttendanceRole {
	if strings.Contains(spec, "Tank") {
		return validation.RoleTank
	}
	switch spec {
	case "Heilig", "Disziplin", "Wiederherstellung", "Wiederherstellung (Baum)":
		return validation.RoleHeal
	}
	return validation.RoleDD
}

// ErrAlreadySignedUp means the character already has a pending signup or a
// roster slot in this raid; nothing was changed.
var ErrAlreadySignedUp = errors.New("character already signed up")

// SubmitForPlayer lets raid staff sign a player's character up, e.g. because
// the player accepted the raid's invite in the game calendar (ADR 092). It
// needs RAIDASSISTENT and an active raid, but not an open signup window: staff
// may still review and accept choices after closing (ADR 033). An existing
// pending choice is left as it is; a withdrawn one is submitted again with spec.
func (s *Service) SubmitForPlayer(ctx context.Context, raidID, actor, characterID int64, spec string) error {
	spec = strings.TrimSpace(spec)
	return s.repo.transaction(ctx, func(r *Repo) error {
		if err := s.authorize(ctx, r, actor, validation.RoleRaidassistent); err != nil {
			return err
		}
		rd, err := r.raids.Get(ctx, raidID)
		if err != nil {
			return reference(err)
		}
		if !active(rd) {
			return ErrRaidState
		}
		c, err := r.characters.Get(ctx, characterID)
		if err != nil {
			return reference(err)
		}
		if !slices.Contains(validation.ClassSpecs[attendance.NormalizeClass(c.Class)], spec) {
			return ErrInvalid
		}
		roster, err := r.attendance.ListByRaid(ctx, raidID)
		if err != nil {
			return err
		}
		for _, a := range roster {
			if a.PlayerID == c.PlayerID && a.CharacterID == c.ID {
				return ErrAlreadySignedUp
			}
		}
		rows, err := r.list(ctx, raidID)
		if err != nil {
			return err
		}
		now := s.now()
		row := signupRow{RaidID: raidID, PlayerID: c.PlayerID, CharacterID: c.ID, Spec: spec, CreatedAt: now, UpdatedAt: now}
		var before any
		for _, existing := range rows {
			if existing.CharacterID != c.ID {
				continue
			}
			if existing.PlayerID != c.PlayerID {
				return ErrConflict
			}
			if !existing.IsWithdrawn {
				return ErrAlreadySignedUp
			}
			before = existing
			row.ID, row.CreatedAt = existing.ID, existing.CreatedAt
		}
		if err := r.save(ctx, &row); err != nil {
			return err
		}
		return s.log(ctx, r, raidID, actor, "submit_for_player", before, row)
	})
}

// Accept creates attendance or explicitly switches an existing roster slot.
// expectedAttendanceID prevents silently replacing a concurrently created slot.
func (s *Service) Accept(ctx context.Context, raidID, id, actor, expectedAttendanceID, expectedCharacterID int64) error {
	return s.repo.transaction(ctx, func(r *Repo) error {
		if err := s.authorize(ctx, r, actor, validation.RoleRaidassistent); err != nil {
			return err
		}
		rd, err := r.raids.Get(ctx, raidID)
		if err != nil {
			return reference(err)
		}
		if !active(rd) {
			return ErrRaidState
		}
		row, err := r.get(ctx, raidID, id)
		if err != nil {
			return reference(err)
		}
		if row.IsWithdrawn {
			return ErrConflict
		}
		c, err := r.characters.Get(ctx, row.CharacterID)
		if err != nil {
			return reference(err)
		}
		if c.PlayerID != row.PlayerID || !slices.Contains(validation.ClassSpecs[attendance.NormalizeClass(c.Class)], row.Spec) {
			return ErrInvalid
		}
		roster, err := r.attendance.ListByRaid(ctx, raidID)
		if err != nil {
			return err
		}
		var current *attendance.Attendance
		for i := range roster {
			if roster[i].PlayerID == row.PlayerID {
				if current != nil {
					return ErrConflict
				}
				current = &roster[i]
			}
		}
		if current != nil && current.CharacterID == row.CharacterID {
			return nil
		}
		role := SpecRole(row.Spec)
		class := attendance.NormalizeClass(c.Class)
		snapshot := raidcorrection.SnapshotPatch{CharacterName: &c.Name, Class: &class, Spec: &row.Spec}
		var after *attendance.Attendance
		if current == nil {
			if expectedAttendanceID != 0 {
				return ErrConflict
			}
			after = &attendance.Attendance{RaidID: raidID, PlayerID: row.PlayerID, CharacterID: row.CharacterID, Role: role}
			err = r.corrections.SaveAttendance(ctx, after, actor, "", snapshot)
		} else {
			if expectedAttendanceID != current.ID || expectedCharacterID != current.CharacterID {
				return ErrConflict
			}
			after, err = r.corrections.CorrectAttendance(ctx, raidID, current.ID, actor, "", func(a *attendance.Attendance) { a.CharacterID = row.CharacterID; a.Role = role }, snapshot)
		}
		if err != nil {
			return err
		}
		return s.log(ctx, r, raidID, actor, "accept", current, after)
	})
}

// absenceTarget authorizes declaring or revoking an absence. Players may only
// change their own absence until the scheduled start; staff may change anyone's
// while the raid is active.
func (s *Service) absenceTarget(ctx context.Context, r *Repo, raidID, actor, target int64) error {
	if target == 0 {
		target = actor
	}
	if err := s.authorize(ctx, r, actor, validation.RoleMember); err != nil {
		return err
	}
	rd, err := r.raids.Get(ctx, raidID)
	if err != nil {
		return reference(err)
	}
	if !active(rd) {
		return ErrRaidState
	}
	p, err := r.players.Get(ctx, actor)
	if err != nil {
		return reference(err)
	}
	if target != actor {
		if !p.Role.AtLeast(validation.RoleRaidassistent) {
			return ErrForbidden
		}
		if _, err := r.players.Get(ctx, target); err != nil {
			return reference(err)
		}
		return nil
	}
	if rd.ScheduledStart != nil && !s.now().Before(*rd.ScheduledStart) {
		return ErrTooLate
	}
	return nil
}

// Decline records or updates an absence with an optional reason. status is
// AbsenceDeclined ("kann nicht") or AbsenceTentative ("unsicher"); empty means declined.
func (s *Service) Decline(ctx context.Context, raidID, actor, target int64, reason, status string) error {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > MaxAbsenceReason {
		return ErrInvalid
	}
	if status == "" {
		status = AbsenceDeclined
	}
	if status != AbsenceDeclined && status != AbsenceTentative {
		return ErrInvalid
	}
	action := "absence_declare"
	if status == AbsenceTentative {
		action = "absence_tentative"
	}
	if target == 0 {
		target = actor
	}
	return s.repo.transaction(ctx, func(r *Repo) error {
		if err := s.absenceTarget(ctx, r, raidID, actor, target); err != nil {
			return err
		}
		row := &absenceRow{RaidID: raidID, PlayerID: target, Reason: reason, Status: status, CreatedAt: s.now()}
		var before any
		if existing, err := r.absence(ctx, raidID, target); err == nil {
			before = existing
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := r.saveAbsence(ctx, row); err != nil {
			return err
		}
		return s.log(ctx, r, raidID, actor, action, before, row)
	})
}

// Revoke removes an absence; it is idempotent.
func (s *Service) Revoke(ctx context.Context, raidID, actor, target int64) error {
	if target == 0 {
		target = actor
	}
	return s.repo.transaction(ctx, func(r *Repo) error {
		if err := s.absenceTarget(ctx, r, raidID, actor, target); err != nil {
			return err
		}
		existing, err := r.absence(ctx, raidID, target)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := r.deleteAbsence(ctx, existing.ID); err != nil {
			return err
		}
		return s.log(ctx, r, raidID, actor, "absence_revoke", existing, nil)
	})
}
