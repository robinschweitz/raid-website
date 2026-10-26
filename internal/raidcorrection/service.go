package raidcorrection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

var (
	ErrNotFound  = errors.New("referenced record not found")
	ErrOwnership = errors.New("character does not belong to player")
	ErrRaidState = errors.New("raid does not allow this write")
	ErrReason    = errors.New("correction requires a nonblank reason and actor")
	ErrInvalid   = errors.New("invalid correction")
	ErrConflict  = errors.New("attendance already exists")
	ErrHasLoot   = errors.New("character still holds loot in this raid")
	// ErrEPShortfall means a correction would take more EP than a player has.
	// The write is rolled back until staff confirm (FR-16.8).
	ErrEPShortfall = errors.New("correction would take more EP than the player has")
)

// Shortfall names a player whose EP deduction the zero floor limits (FR-16.6).
type Shortfall struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	Balance    int    `json:"balance"`
	Taken      int    `json:"points_taken"`
	NotTaken   int    `json:"points_not_taken"`
}

// ShortfallError carries the limited deductions of a refused correction.
type ShortfallError struct{ Shortfalls []Shortfall }

func (e *ShortfallError) Error() string        { return ErrEPShortfall.Error() }
func (e *ShortfallError) Is(target error) bool { return target == ErrEPShortfall }

type confirmKey struct{}

// WithEPShortfallConfirmed marks a request whose staff saw the FR-16.8 warning
// and chose to save the limited correction anyway.
func WithEPShortfallConfirmed(ctx context.Context) context.Context {
	return context.WithValue(ctx, confirmKey{}, true)
}

// Service applies history changes with audit and append-only compensation.
type Service struct {
	repo     *Repo
	external registration.ExternalClient
}

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

func (s *Service) WithExternalClient(external registration.ExternalClient) *Service {
	s.external = external
	return s
}

func referenceError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Service) check(ctx context.Context, r *Repo, raidID, actor int64, reason string) (bool, error) {
	if actor <= 0 {
		return false, ErrReason
	}
	rd, err := r.raids.Get(ctx, raidID)
	if err != nil {
		return false, referenceError(err)
	}
	if _, err := r.players.Get(ctx, actor); err != nil {
		return false, referenceError(err)
	}
	if rd.Status == validation.RaidStatusCancelled {
		return false, ErrRaidState
	}
	if rd.ArchivedAt != nil {
		if rd.Status != validation.RaidStatusCompleted {
			return false, ErrRaidState
		}
		if strings.TrimSpace(reason) == "" {
			return false, ErrReason
		}
	}
	completed := rd.Status == validation.RaidStatusCompleted
	if !completed && rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen {
		return false, ErrRaidState
	}
	return completed, nil
}

func (s *Service) ownership(ctx context.Context, r *Repo, playerID int64, characterID *int64) error {
	if _, err := r.players.Get(ctx, playerID); err != nil {
		return referenceError(err)
	}
	if characterID != nil {
		c, err := r.characters.Get(ctx, *characterID)
		if err != nil {
			return referenceError(err)
		}
		if c.PlayerID != playerID {
			return ErrOwnership
		}
	}
	return nil
}

func (s *Service) log(ctx context.Context, r *Repo, entity string, id, actor int64, reason string, before, after any) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	return r.audit.Insert(ctx, &audit.Log{ActorID: &actor, EntityType: entity, EntityID: strconv.FormatInt(id, 10), Action: "correction", BeforeJSON: string(b), AfterJSON: string(a), Reason: strings.TrimSpace(reason), CreatedAt: time.Now().UTC()})
}

// SnapshotPatch distinguishes omitted defaults from explicitly blank snapshot values.
type SnapshotPatch struct {
	CharacterName *string
	Class         *string
	Spec          *string
}

// SaveAttendance adds attendance or applies an audited correction.
func (s *Service) SaveAttendance(ctx context.Context, row *attendance.Attendance, actor int64, reason string, snapshot ...SnapshotPatch) error {
	return s.saveAttendance(ctx, row, actor, reason, nil, snapshot...)
}

// CorrectAttendance loads and patches the record within the correction transaction.
func (s *Service) CorrectAttendance(ctx context.Context, raidID, id, actor int64, reason string, patch func(*attendance.Attendance), snapshot ...SnapshotPatch) (*attendance.Attendance, error) {
	row := &attendance.Attendance{ID: id, RaidID: raidID}
	if patch == nil {
		patch = func(*attendance.Attendance) {}
	}
	err := s.saveAttendance(ctx, row, actor, reason, patch, snapshot...)
	return row, err
}

func (s *Service) saveAttendance(ctx context.Context, row *attendance.Attendance, actor int64, reason string, patch func(*attendance.Attendance), snapshot ...SnapshotPatch) error {
	return s.repo.InTransaction(ctx, func(r *Repo) error {
		var before any
		if row.ID != 0 {
			old, err := r.attendance.Get(ctx, row.ID)
			if err != nil {
				return referenceError(err)
			}
			if old.RaidID != row.RaidID {
				return ErrNotFound
			}
			before = old
			row.CreatedAt = old.CreatedAt
			if patch != nil {
				*row = *old
				patch(row)
				row.ID = old.ID
				row.RaidID = old.RaidID
			}
		}
		completed, err := s.check(ctx, r, row.RaidID, actor, reason)
		if err != nil {
			return err
		}
		if row.Role != validation.RoleTank && row.Role != validation.RoleHeal && row.Role != validation.RoleDD {
			return ErrInvalid
		}
		if err := s.ownership(ctx, r, row.PlayerID, &row.CharacterID); err != nil {
			return err
		}
		old, _ := before.(*attendance.Attendance)
		referenceChanged := old != nil && (old.CharacterID != row.CharacterID || old.PlayerID != row.PlayerID)
		if row.ID == 0 || referenceChanged {
			c, err := r.characters.Get(ctx, row.CharacterID)
			if err != nil {
				return referenceError(err)
			}
			if referenceChanged || row.CharacterName == "" {
				row.CharacterName = c.Name
			}
			if referenceChanged || row.Class == "" {
				row.Class = c.Class
			}
			if referenceChanged || row.Spec == "" {
				row.Spec = c.MainSpec
			}
		}
		if len(snapshot) > 1 {
			return ErrInvalid
		}
		var input SnapshotPatch
		if len(snapshot) == 1 {
			input = snapshot[0]
		}
		if input.CharacterName != nil {
			row.CharacterName = *input.CharacterName
		}
		if input.Class != nil {
			row.Class = *input.Class
		}
		if input.Spec != nil {
			row.Spec = *input.Spec
		}
		// Unknown migrated history stays unknown unless explicitly corrected.
		if row.ID == 0 || referenceChanged || input.CharacterName != nil || (old != nil && old.CharacterName != row.CharacterName) {
			row.CharacterName = strings.TrimSpace(row.CharacterName)
			if row.CharacterName == "" {
				return ErrInvalid
			}
		}
		if row.ID == 0 || referenceChanged || input.Class != nil || (old != nil && old.Class != row.Class) {
			row.Class = attendance.NormalizeClass(row.Class)
			if _, ok := validation.ClassSpecs[row.Class]; !ok {
				return ErrInvalid
			}
		}
		if row.ID == 0 || referenceChanged || input.Spec != nil || (old != nil && old.Spec != row.Spec) {
			row.Spec = strings.TrimSpace(row.Spec)
			if row.Spec == "" {
				return ErrInvalid
			}
		}
		rows, err := r.attendance.ListByRaid(ctx, row.RaidID)
		if err != nil {
			return err
		}
		for _, existing := range rows {
			if existing.ID != row.ID && existing.PlayerID == row.PlayerID && existing.CharacterID == row.CharacterID {
				return ErrConflict
			}
		}
		var eligibilityBefore map[int64]int
		if completed {
			eligibilityBefore, err = s.pointEligibility(ctx, r, row.RaidID)
			if err != nil {
				return err
			}
		}
		row.UpdatedAt = time.Now().UTC()
		if row.ID == 0 {
			row.CreatedAt = row.UpdatedAt
			err = r.attendance.Create(ctx, row)
		} else {
			err = r.attendance.Update(ctx, row)
		}
		if err != nil {
			return err
		}
		if completed {
			if err := s.reconcilePoints(ctx, r, row.RaidID, actor, reason, eligibilityBefore); err != nil {
				return err
			}
		}
		// Additions to active raids are logged too, so the audit log shows who
		// put a character into the roster (ADR 077).
		return s.log(ctx, r, "attendance", row.ID, actor, reason, before, row)
	})
}

// RemoveAttendance hard-deletes a roster row from a planned or open raid
// (ADR 076). Completed, archived and cancelled raids keep their roster. A row
// whose character still holds unwithdrawn loot in this raid cannot be removed;
// the loot must be withdrawn first. The audit log keeps the removed row.
func (s *Service) RemoveAttendance(ctx context.Context, raidID, id, actor int64) error {
	return s.repo.InTransaction(ctx, func(r *Repo) error {
		if actor <= 0 {
			return ErrReason
		}
		if _, err := r.players.Get(ctx, actor); err != nil {
			return referenceError(err)
		}
		rd, err := r.raids.Get(ctx, raidID)
		if err != nil {
			return referenceError(err)
		}
		if rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen) {
			return ErrRaidState
		}
		row, err := r.attendance.Get(ctx, id)
		if err != nil {
			return referenceError(err)
		}
		if row.RaidID != raidID {
			return ErrNotFound
		}
		awards, err := r.loot.ListByRaid(ctx, raidID)
		if err != nil {
			return err
		}
		for _, award := range awards {
			if award.WithdrawnAt != nil {
				continue
			}
			if award.AwardedToCharacterID != nil && *award.AwardedToCharacterID == row.CharacterID {
				return ErrHasLoot
			}
			if award.AwardedToCharacterID == nil && award.AwardedToPlayerID == row.PlayerID {
				return ErrHasLoot
			}
		}
		if err := r.deleteQuickAddRequests(ctx, row.ID); err != nil {
			return err
		}
		if err := r.attendance.Delete(ctx, row.ID); err != nil {
			return err
		}
		before, err := json.Marshal(row)
		if err != nil {
			return err
		}
		return r.audit.Insert(ctx, &audit.Log{ActorID: &actor, EntityType: "attendance", EntityID: strconv.FormatInt(row.ID, 10), Action: "remove", BeforeJSON: string(before), AfterJSON: "null", CreatedAt: time.Now().UTC()})
	})
}

// SaveLoot applies ordinary active-raid additions or explicit audited corrections.
func (s *Service) SaveLoot(ctx context.Context, row *loot.Award, actor int64, reason string) error {
	return s.saveLoot(ctx, row, actor, reason, nil, false)
}

// SaveImportedLoot retains the existing addon import behavior until its
// transactional per-boss difficulty flow is introduced in #96.
func (s *Service) SaveImportedLoot(ctx context.Context, row *loot.Award, actor int64, reason string) error {
	return s.saveLoot(ctx, row, actor, reason, nil, true)
}

// CorrectLoot loads and patches the award within the correction transaction.
func (s *Service) CorrectLoot(ctx context.Context, raidID, id, actor int64, reason string, patch func(*loot.Award)) (*loot.Award, error) {
	row := &loot.Award{ID: id, RaidID: raidID}
	err := s.saveLoot(ctx, row, actor, reason, patch, false)
	return row, err
}

func (s *Service) saveLoot(ctx context.Context, row *loot.Award, actor int64, reason string, patch func(*loot.Award), imported bool) error {
	return s.repo.InTransaction(ctx, func(r *Repo) error {
		if row.ID == 0 {
			row.AwardedByPlayerID = actor
			retried, err := loot.ResolveRequest(ctx, r.loot, row, reason)
			if err != nil || retried {
				return err
			}
		}
		var before any
		if row.ID != 0 {
			old, err := r.loot.Get(ctx, row.ID)
			if err != nil {
				return referenceError(err)
			}
			if old.RaidID != row.RaidID {
				return ErrNotFound
			}
			before = old
			if patch != nil {
				*row = *old
				patch(row)
				row.ID = old.ID
				row.RaidID = old.RaidID
			}
			row.Difficulty = old.Difficulty
			row.CreatedAt = old.CreatedAt
			row.AwardedAt = old.AwardedAt
			row.AwardedByPlayerID = old.AwardedByPlayerID
			row.Source = old.Source
			row.RequestID = old.RequestID
			row.RequestHash = old.RequestHash
			// Corrections preserve historical lock consumption, never reapply it.
			if row.ItemLockID != nil && (old.ItemLockID == nil || *row.ItemLockID != *old.ItemLockID) {
				return ErrInvalid
			}
			row.ItemLockID = old.ItemLockID
		}
		completed, err := s.check(ctx, r, row.RaidID, actor, reason)
		if err != nil {
			return err
		}
		if row.ID == 0 && !imported {
			if err := loot.EnsureBossKill(ctx, r.loot, row, reason); err != nil {
				return err
			}
		}
		if !completed && row.ID == 0 {
			row.UpdatedAt = time.Now().UTC()
			row.CreatedAt = row.UpdatedAt
			row.AwardedAt = row.UpdatedAt
			row.AwardedByPlayerID = actor
			row.Source = "manual"
			if err := loot.AwardPreparedInTransaction(ctx, r.loot, row, reason); err != nil {
				return err
			}
			return nil
		}
		oldAward, _ := before.(*loot.Award)
		if err := loot.ValidateCatalog(ctx, r.loot, row, oldAward); err != nil {
			return err
		}
		if strings.TrimSpace(row.ItemName) == "" {
			return ErrInvalid
		}
		if err := row.DeriveMainSpec(); err != nil {
			return err
		}
		if err := s.ownership(ctx, r, row.AwardedToPlayerID, row.AwardedToCharacterID); err != nil {
			return err
		}
		var eligibilityBefore map[int64]int
		if completed {
			eligibilityBefore, err = s.pointEligibility(ctx, r, row.RaidID)
			if err != nil {
				return err
			}
		}
		row.UpdatedAt = time.Now().UTC()
		old, _ := before.(*loot.Award)
		recipientChanged := old != nil && (old.AwardedToPlayerID != row.AwardedToPlayerID || !sameCharacter(old.AwardedToCharacterID, row.AwardedToCharacterID))
		if recipientChanged {
			row.AwardedToCharacterName = ""
		} else if old != nil {
			row.AwardedToCharacterName = old.AwardedToCharacterName
		}
		enteringEnhanced := old != nil && old.AwardMethod != loot.Enhanced && row.AwardMethod == loot.Enhanced
		if old == nil || recipientChanged || enteringEnhanced {
			if err := loot.ValidateRecipient(ctx, r.loot, row); err != nil {
				return err
			}
		}
		if old == nil {
			if err := loot.ValidateEnhancedRequest(row); err != nil {
				return err
			}
		}
		row.EnhancedCost = 0
		if row.ID == 0 && row.ItemLockID != nil {
			return ErrInvalid
		}
		if row.ID == 0 {
			row.CreatedAt = row.UpdatedAt
			row.AwardedByPlayerID = actor
			row.AwardedAt = row.UpdatedAt
			row.Source = "manual"
			err = r.loot.CreateAward(ctx, row)
		} else {
			err = nil
		}
		if err != nil {
			return err
		}
		if err := loot.ReconcileEnhanced(ctx, r.loot, row, old, actor, reason); err != nil {
			return err
		}
		if err := r.loot.Update(ctx, row); err != nil {
			return err
		}
		if completed {
			if err := s.reconcilePoints(ctx, r, row.RaidID, actor, reason, eligibilityBefore); err != nil {
				return err
			}
		}
		if err := s.log(ctx, r, "loot_award", row.ID, actor, reason, before, row); err != nil {
			return err
		}
		return nil
	})
}

// WithdrawLoot retains the award and its audit history while removing its loot
// consequences. Only manual awards can be withdrawn.
func (s *Service) WithdrawLoot(ctx context.Context, raidID, id, actor int64, reason string) (*loot.Award, error) {
	row := &loot.Award{}
	err := s.repo.InTransaction(ctx, func(r *Repo) error {
		old, err := r.loot.Get(ctx, id)
		if err != nil {
			return referenceError(err)
		}
		if old.RaidID != raidID || old.Source != "manual" || old.WithdrawnAt != nil {
			return ErrInvalid
		}
		// check requires a nonblank reason only for archived raids (FR-26.5).
		completed, err := s.check(ctx, r, raidID, actor, reason)
		if err != nil {
			return err
		}
		var eligibilityBefore map[int64]int
		if completed {
			eligibilityBefore, err = s.pointEligibility(ctx, r, raidID)
			if err != nil {
				return err
			}
		}
		*row = *old
		now := time.Now().UTC()
		row.WithdrawnAt, row.WithdrawnByPlayerID = &now, &actor
		if withdrawnReason := strings.TrimSpace(reason); withdrawnReason != "" {
			row.WithdrawnReason = &withdrawnReason
		}
		row.UpdatedAt = now
		if err := r.loot.Update(ctx, row); err != nil {
			return err
		}
		if completed {
			if err := s.reconcilePoints(ctx, r, raidID, actor, reason, eligibilityBefore); err != nil {
				return err
			}
		}
		if err := s.log(ctx, r, "loot_award", row.ID, actor, reason, old, row); err != nil {
			return err
		}
		return nil
	})
	return row, err
}

func sameCharacter(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Service) pointEligibility(ctx context.Context, r *Repo, raidID int64) (map[int64]int, error) {
	rows, err := r.attendance.ListByRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	desired := make(map[int64]int)
	for _, row := range rows {
		if _, exists := desired[row.PlayerID]; exists {
			continue
		}
		desired[row.PlayerID] = 0
		hasLoot, err := r.loot.PlayerHasMainSpecAward(ctx, raidID, row.PlayerID)
		if err != nil {
			return nil, err
		}
		if !hasLoot {
			desired[row.PlayerID] = 10
		}
	}
	return desired, nil
}

func (s *Service) reconcilePoints(ctx context.Context, r *Repo, raidID, actor int64, reason string, before map[int64]int) error {
	after, err := s.pointEligibility(ctx, r, raidID)
	if err != nil {
		return err
	}
	for playerID := range before {
		if _, exists := after[playerID]; !exists {
			after[playerID] = 0
		}
	}
	playerIDs := make([]int64, 0, len(after))
	for playerID := range after {
		playerIDs = append(playerIDs, playerID)
	}
	sort.Slice(playerIDs, func(i, j int) bool { return playerIDs[i] < playerIDs[j] })
	confirmed, _ := ctx.Value(confirmKey{}).(bool)
	var shortfalls []Shortfall
	// Apply only the eligibility change, never backfill historical raid awards.
	for _, playerID := range playerIDs {
		amount := after[playerID] - before[playerID]
		if amount == 0 {
			continue
		}
		var notTaken int
		if amount < 0 {
			// A deduction never takes a balance below 0 and the rest is not taken later (FR-16.6).
			balance, err := r.points.Balance(ctx, playerID)
			if err != nil {
				return err
			}
			if available := max(balance, 0); -amount > available {
				notTaken = -amount - available
				amount = -available
				p, err := r.players.Get(ctx, playerID)
				if err != nil {
					return referenceError(err)
				}
				shortfalls = append(shortfalls, Shortfall{PlayerID: playerID, PlayerName: p.Name, Balance: balance, Taken: available, NotTaken: notTaken})
			}
		}
		if amount != 0 {
			if err := r.points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: playerID, RaidID: &raidID, Amount: amount, Type: "correction_no_mainspec_loot", Reason: strings.TrimSpace(reason), RefType: "raid", RefID: strconv.FormatInt(raidID, 10), ActorID: actor, CreatedAt: time.Now().UTC()}); err != nil {
				return err
			}
		}
		if notTaken > 0 {
			// The audit entry keeps the number of points not taken (FR-16.8).
			if err := s.log(ctx, r, "ep_correction", playerID, actor, reason, nil, map[string]int64{"raid_id": raidID, "points_not_taken": int64(notTaken)}); err != nil {
				return err
			}
		}
	}
	if len(shortfalls) > 0 && !confirmed {
		return &ShortfallError{Shortfalls: shortfalls}
	}
	return nil
}
