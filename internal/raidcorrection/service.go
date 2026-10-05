package raidcorrection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/tokens"
	"raid-website/internal/validation"
)

var (
	ErrNotFound  = errors.New("referenced record not found")
	ErrOwnership = errors.New("character does not belong to player")
	ErrRaidState = errors.New("raid does not allow this write")
	ErrReason    = errors.New("correction requires a nonblank reason and actor")
	ErrInvalid   = errors.New("invalid correction")
	ErrConflict  = errors.New("attendance already exists")
)

// Service applies history changes with audit and append-only compensation.
type Service struct{ repo *Repo }

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

func referenceError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Service) check(ctx context.Context, r *Repo, raidID, actor int64, reason string, imported bool) (bool, error) {
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
		if imported {
			mapped, err := r.isImportedRaid(ctx, raidID)
			if err != nil {
				return false, err
			}
			if !mapped {
				return false, ErrRaidState
			}
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
	return s.saveAttendance(ctx, row, actor, reason, nil, false, snapshot...)
}

// SaveImportedAttendance audits a new historical attendance entry in an existing raid.
func (s *Service) SaveImportedAttendance(ctx context.Context, row *attendance.Attendance, actor int64, reason string) error {
	if row.ID != 0 {
		return ErrInvalid
	}
	return s.saveAttendance(ctx, row, actor, reason, nil, true)
}

// CorrectAttendance loads and patches the record within the correction transaction.
func (s *Service) CorrectAttendance(ctx context.Context, raidID, id, actor int64, reason string, patch func(*attendance.Attendance), snapshot ...SnapshotPatch) (*attendance.Attendance, error) {
	row := &attendance.Attendance{ID: id, RaidID: raidID}
	if patch == nil {
		patch = func(*attendance.Attendance) {}
	}
	err := s.saveAttendance(ctx, row, actor, reason, patch, false, snapshot...)
	return row, err
}

func (s *Service) saveAttendance(ctx context.Context, row *attendance.Attendance, actor int64, reason string, patch func(*attendance.Attendance), imported bool, snapshot ...SnapshotPatch) error {
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
		completed, err := s.check(ctx, r, row.RaidID, actor, reason, imported)
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
			if referenceChanged || (!imported && row.CharacterName == "") {
				row.CharacterName = c.Name
			}
			if referenceChanged || (!imported && row.Class == "") {
				row.Class = c.Class
			}
			if referenceChanged || (!imported && row.Spec == "") {
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
			// Imported historical additions preserve unavailable source metadata,
			// matching the initial import; they never use live character defaults.
			if _, ok := validation.ClassSpecs[row.Class]; !ok && !imported {
				return ErrInvalid
			}
		}
		if row.ID == 0 || referenceChanged || input.Spec != nil || (old != nil && old.Spec != row.Spec) {
			row.Spec = strings.TrimSpace(row.Spec)
			if row.Spec == "" && !imported {
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
		if completed || before != nil || imported {
			return s.log(ctx, r, "attendance", row.ID, actor, reason, before, row)
		}
		return nil
	})
}

// SaveLoot applies ordinary active-raid additions or explicit audited corrections.
func (s *Service) SaveLoot(ctx context.Context, row *loot.Award, actor int64, reason string) error {
	return s.saveLoot(ctx, row, actor, reason, nil, false)
}

// SaveImportedLoot audits a new historical award, preserving its source timestamp.
func (s *Service) SaveImportedLoot(ctx context.Context, row *loot.Award, actor int64, reason string) error {
	if row.ID != 0 || row.Source != "legacy_import" || row.AwardedAt.IsZero() {
		return ErrInvalid
	}
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
		if row.ID == 0 && !imported {
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
		completed, err := s.check(ctx, r, row.RaidID, actor, reason, imported)
		if err != nil {
			return err
		}
		if !completed && row.ID == 0 && !imported {
			row.UpdatedAt = time.Now().UTC()
			row.CreatedAt = row.UpdatedAt
			row.AwardedAt = row.UpdatedAt
			row.AwardedByPlayerID = actor
			row.Source = "manual"
			return loot.AwardInTransaction(ctx, r.loot, row, reason)
		}
		oldAward, _ := before.(*loot.Award)
		if !imported {
			if err := loot.ValidateCatalog(ctx, r.loot, row, oldAward); err != nil {
				return err
			}
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
		enteringEnhanced := old != nil && old.AwardMethod != loot.Enhanced && row.AwardMethod == loot.Enhanced
		if !imported && (old == nil || recipientChanged || enteringEnhanced) {
			if err := loot.ValidateRecipient(ctx, r.loot, row); err != nil {
				return err
			}
		}
		if !imported && old == nil {
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
			if !imported {
				row.AwardedAt = row.UpdatedAt
				row.Source = "manual"
			}
			err = r.loot.CreateAward(ctx, row)
		} else {
			err = nil
		}
		if err != nil {
			return err
		}
		if !imported {
			if err := loot.ReconcileEnhanced(ctx, r.loot, row, old, actor, reason); err != nil {
				return err
			}
		}
		if err := r.loot.Update(ctx, row); err != nil {
			return err
		}
		if err := s.reconcileTokens(ctx, r, row, actor, reason); err != nil {
			return err
		}
		if completed {
			if err := s.reconcilePoints(ctx, r, row.RaidID, actor, reason, eligibilityBefore); err != nil {
				return err
			}
		}
		return s.log(ctx, r, "loot_award", row.ID, actor, reason, before, row)
	})
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
	// Apply only the eligibility change, never backfill historical raid awards.
	for playerID, eligibility := range after {
		amount := eligibility - before[playerID]
		if amount == 0 {
			continue
		}
		if err := r.points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: playerID, RaidID: &raidID, Amount: amount, Type: "correction_no_mainspec_loot", Reason: strings.TrimSpace(reason), RefType: "raid", RefID: strconv.FormatInt(raidID, 10), ActorID: actor, CreatedAt: time.Now().UTC()}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) reconcileTokens(ctx context.Context, r *Repo, row *loot.Award, actor int64, reason string) error {
	nets, err := r.tokenNets(ctx, row.ID)
	if err != nil {
		return err
	}
	desired := make(map[int64]int)
	if tokens.IsTokenAward(row.ItemName) {
		desired[row.AwardedToPlayerID] = tokens.DeriveTokenAmount(1)
	}
	for playerID := range desired {
		if _, ok := nets[playerID]; !ok {
			nets[playerID] = 0
		}
	}
	for playerID, net := range nets {
		amount := desired[playerID] - net
		if amount == 0 {
			continue
		}
		if err := r.tokens.InsertTransaction(ctx, &tokens.Transaction{PlayerID: playerID, RaidID: &row.RaidID, LootAwardID: &row.ID, Amount: amount, Type: "correction", Reason: strings.TrimSpace(reason), RefType: "loot_award", RefID: strconv.FormatInt(row.ID, 10), ActorID: actor, CreatedAt: time.Now().UTC()}); err != nil {
			return err
		}
	}
	return nil
}
