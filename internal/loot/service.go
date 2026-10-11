package loot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/validation"
)

// RaidScopedRequestPrefix marks request IDs that identify one award per raid,
// whoever records it: lines imported from the RaidSync addon (ADR 045). Other
// request IDs only make retries of the same staff member idempotent.
const RaidScopedRequestPrefix = "raidsync-"

var (
	ErrRaidState       = errors.New("raid does not allow ordinary loot awards")
	ErrNotFound        = errors.New("referenced record not found")
	ErrOwnership       = errors.New("character does not belong to recipient")
	ErrLockConflict    = errors.New("multiple active locks match item")
	ErrInvalidLock     = errors.New("item lock must match an active lock for this raid and item")
	ErrInvalidMethod   = errors.New("award_method must be MAIN, SECOND, ENHANCED or BOE")
	ErrItemName        = errors.New("item_name is required")
	ErrRoster          = errors.New("recipient character must be in raid roster")
	ErrRequestRequired = errors.New("new enhanced awards require a nonblank request_id")
	ErrRequestConflict = errors.New("request id already used with different payload")
	// ErrAlreadyRecorded means a raid-scoped request ID already has an award in
	// this raid, recorded by any staff member.
	ErrAlreadyRecorded        = errors.New("raid-scoped request already recorded")
	ErrCatalogContextRequired = errors.New("raid catalog instance and size are required")
)

// Service coordinates loot awards with derived token transactions.
type Service struct {
	awards *Repo
}

// NewService constructs a loot service using the repository transaction boundary.
func NewService(repo *Repo) *Service {
	return &Service{awards: repo}
}

// Award persists a loot award and, when the item is an armor token,
// records the derived token transaction in the same database transaction.
func (s *Service) Award(ctx context.Context, a *Award) error {
	return s.awards.InTransaction(ctx, func(awards *Repo) error { return AwardInTransaction(ctx, awards, a, "") })
}

// AwardInTransaction applies ordinary loot consequences using the caller's repository transaction.
func AwardInTransaction(ctx context.Context, awards *Repo, a *Award, reason string) error {
	if err := awards.ReserveWriter(ctx); err != nil {
		return err
	}
	retried, err := ResolveRequest(ctx, awards, a, reason)
	if err != nil || retried {
		return err
	}
	if err := a.DeriveMainSpec(); err != nil {
		return err
	}
	repositories := awards.awardRepositories()
	rd, err := repositories.raids.Get(ctx, a.RaidID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen) {
		return ErrRaidState
	}
	if err := ValidateCatalog(ctx, awards, a, nil); err != nil {
		return err
	}
	if strings.TrimSpace(a.ItemName) == "" {
		return ErrItemName
	}
	for _, id := range []int64{a.AwardedToPlayerID, a.AwardedByPlayerID} {
		if _, err := repositories.players.Get(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	if a.AwardedToCharacterID != nil {
		c, err := repositories.characters.Get(ctx, *a.AwardedToCharacterID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if c.PlayerID != a.AwardedToPlayerID {
			return ErrOwnership
		}
	}
	points := repositories.points
	locks, err := points.ListActiveLocksByRaid(ctx, a.RaidID)
	if err != nil {
		return err
	}
	matching, err := enhancepoints.ResolveActiveLock(locks, a.ItemID, a.ItemName)
	if errors.Is(err, enhancepoints.ErrLockConflict) {
		return ErrLockConflict
	}
	if err != nil {
		return err
	}
	if a.ItemLockID != nil {
		if _, err := points.GetLock(ctx, *a.ItemLockID); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if matching == nil || matching.ID != *a.ItemLockID {
			return ErrInvalidLock
		}
	}
	if matching != nil {
		if a.AwardedToPlayerID != matching.PlayerID {
			a.AwardedToCharacterID = nil
		}
		a.AwardedToPlayerID = matching.PlayerID
		a.AwardMethod = Main
		a.IsMainSpec = true
		if a.AwardedToCharacterID == nil {
			rows, err := repositories.attendance.ListByRaid(ctx, a.RaidID)
			if err != nil {
				return err
			}
			for _, row := range rows {
				if row.PlayerID != matching.PlayerID {
					continue
				}
				if a.AwardedToCharacterID != nil {
					return ErrLockConflict
				}
				id := row.CharacterID
				a.AwardedToCharacterID = &id
			}
		}
		a.ItemLockID = &matching.ID
		matching.Status = validation.LockStatusUsed
		now := time.Now().UTC()
		matching.UsedAt = &now
		matching.UpdatedAt = now
		if err := points.UpdateLock(ctx, matching); err != nil {
			return err
		}
	}
	if err := ValidateRecipient(ctx, awards, a); err != nil {
		return err
	}
	if err := ValidateEnhancedRequest(a); err != nil {
		return err
	}
	a.EnhancedCost = 0

	if err := awards.CreateAward(ctx, a); err != nil {
		return fmt.Errorf("create award: %w", err)
	}
	if err := ReconcileEnhanced(ctx, awards, a, nil, a.AwardedByPlayerID, "enhanced winning loot"); err != nil {
		return err
	}
	if err := awards.Update(ctx, a); err != nil {
		return err
	}

	if a.AwardMethod == Enhanced {
		after, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if strings.TrimSpace(reason) == "" {
			reason = "enhanced winning loot"
		}
		return repositories.audit.Insert(ctx, &audit.Log{ActorID: &a.AwardedByPlayerID, EntityType: "loot_award", EntityID: fmt.Sprint(a.ID), Action: "award", BeforeJSON: "null", AfterJSON: string(after), Reason: strings.TrimSpace(reason), CreatedAt: time.Now().UTC()})
	}
	return nil
}

// ValidateEnhancedRequest requires a retry key for a new enhanced award, after lock resolution.
func ValidateEnhancedRequest(a *Award) error {
	if a.AwardMethod == Enhanced && (a.RequestID == nil || strings.TrimSpace(*a.RequestID) == "") {
		return ErrRequestRequired
	}
	return nil
}

// ValidateRecipient requires ownership and the exact existing raid roster association.
func ValidateRecipient(ctx context.Context, repo *Repo, a *Award) error {
	if a.AwardedToCharacterID == nil || *a.AwardedToCharacterID <= 0 {
		return ErrRoster
	}
	repositories := repo.awardRepositories()
	c, err := repositories.characters.Get(ctx, *a.AwardedToCharacterID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if c.PlayerID != a.AwardedToPlayerID {
		return ErrOwnership
	}
	rows, err := repositories.attendance.ListByRaid(ctx, a.RaidID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.PlayerID == a.AwardedToPlayerID && row.CharacterID == c.ID {
			return nil
		}
	}
	return ErrRoster
}

// ResolveRequest retains the original payload fingerprint, even after corrections.
func ResolveRequest(ctx context.Context, repo *Repo, a *Award, reason string) (bool, error) {
	if a.RequestID == nil {
		return false, nil
	}
	if strings.TrimSpace(*a.RequestID) == "" {
		if a.AwardMethod == Enhanced {
			return false, ErrRequestRequired
		}
		return false, ErrRequestConflict
	}
	payload := struct {
		RaidID                 int64
		Boss, ItemID, ItemName string
		PlayerID               int64
		CharacterID            *int64
		CatalogBossID          *string
		Method                 Method
		LockID                 *int64
		Notes, Reason          string
	}{a.RaidID, a.Boss, a.ItemID, a.ItemName, a.AwardedToPlayerID, a.AwardedToCharacterID, a.CatalogBossID, a.AwardMethod, a.ItemLockID, a.Notes, reason}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	a.RequestHash = fmt.Sprintf("%x", sha256.Sum256(encoded))
	if strings.HasPrefix(*a.RequestID, RaidScopedRequestPrefix) {
		// Callers hold the write reservation, so this read cannot miss an
		// award another staff member is recording at the same time.
		_, err := repo.GetByRaidRequest(ctx, a.RaidID, *a.RequestID)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return false, ErrAlreadyRecorded
	}
	old, err := repo.GetByRequest(ctx, a.AwardedByPlayerID, *a.RequestID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if old.RequestHash != a.RequestHash {
		return false, ErrRequestConflict
	}
	*a = *old
	return true, nil
}

// ValidateCatalog preserves historical snapshots on unrelated edits and validates new identities.
func ValidateCatalog(ctx context.Context, repo *Repo, a, old *Award) error {
	sameBoss := old != nil && ((old.CatalogBossID == nil && a.CatalogBossID == nil) || (old.CatalogBossID != nil && a.CatalogBossID != nil && *old.CatalogBossID == *a.CatalogBossID))
	enteringEnhanced := old != nil && old.AwardMethod != Enhanced && a.AwardMethod == Enhanced
	if old != nil && !enteringEnhanced && sameBoss && old.Boss == a.Boss && old.ItemName == a.ItemName && old.ItemID == a.ItemID && old.AwardedToPlayerID == a.AwardedToPlayerID && ((old.AwardedToCharacterID == nil && a.AwardedToCharacterID == nil) || (old.AwardedToCharacterID != nil && a.AwardedToCharacterID != nil && *old.AwardedToCharacterID == *a.AwardedToCharacterID)) {
		if old.CatalogBossID != nil {
			a.Boss, a.ItemName = old.Boss, old.ItemName
		}
		return nil
	}
	rd, err := repo.awardRepositories().raids.Get(ctx, a.RaidID)
	if err != nil {
		return err
	}
	if rd.CatalogInstanceID == nil || rd.RaidSize == nil {
		return ErrCatalogContextRequired
	}
	boss, item, err := repo.Catalog().ValidateDrop(ctx, *rd.CatalogInstanceID, rd.RaidSize, a.CatalogBossID, a.ItemID)
	if err != nil {
		return err
	}
	a.Boss, a.ItemName = boss.Name, item.Name
	return nil
}

// ReconcileEnhanced appends refunds and new winner debits, never changes ledger history.
// The award must already exist; callers persist EnhancedCost in the same transaction.
func ReconcileEnhanced(ctx context.Context, repo *Repo, a, old *Award, actor int64, reason string) error {
	points := repo.awardRepositories().points
	if old != nil && old.AwardMethod == Enhanced && a.AwardMethod == Enhanced && old.AwardedToPlayerID == a.AwardedToPlayerID {
		a.EnhancedCost = old.EnhancedCost
		return nil
	}
	appendEntry := func(playerID int64, amount int) error {
		return points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: playerID, RaidID: &a.RaidID, LootAwardID: &a.ID, Amount: amount, Type: "enhanced_loot", RefType: "loot_award", RefID: fmt.Sprint(a.ID), Reason: reason, ActorID: actor, CreatedAt: time.Now().UTC()})
	}
	if old != nil && old.AwardMethod == Enhanced && old.EnhancedCost > 0 {
		if err := appendEntry(old.AwardedToPlayerID, old.EnhancedCost); err != nil {
			return err
		}
	}
	a.EnhancedCost = 0
	if a.AwardMethod != Enhanced {
		return nil
	}
	balance, err := points.Balance(ctx, a.AwardedToPlayerID)
	if err != nil {
		return err
	}
	cost, err := enhancepoints.RollCost(balance)
	if err != nil {
		return err
	}
	a.EnhancedCost = cost
	return appendEntry(a.AwardedToPlayerID, -cost)
}
