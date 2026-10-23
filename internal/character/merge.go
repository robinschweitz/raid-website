package character

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/audit"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

var (
	ErrMergeSame           = errors.New("source and target characters must differ")
	ErrMergeDifferentOwner = errors.New("merge the player accounts first")
	ErrMergeInactive       = errors.New("source character is inactive")
	ErrMergeConflict       = errors.New("both characters have overlapping raid attendance, signup or wishlist records")
)

// MergeDuplicate corrects a mistakenly duplicated character. This is not an in-game rename.
// The source is deleted after all of its references have moved to the target.
// Both characters must belong to the same player (use the account merge first).
func (s *Service) MergeDuplicate(ctx context.Context, actorID, targetID, sourceID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if targetID <= 0 || sourceID <= 0 || targetID == sourceID {
		return ErrMergeSame
	}
	if reason == "" {
		return ErrReasonRequired
	}
	return s.repo.runInTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		chars := NewRepo(tx)
		if err := chars.AcquireWriteLock(ctx); err != nil {
			return err
		}
		players := player.NewRepo(tx)
		actor, err := players.Get(ctx, actorID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActor
		}
		if err != nil {
			return err
		}
		if !actor.Role.AtLeast(validation.RoleAdmin) {
			return ErrForbidden
		}
		target, err := chars.Get(ctx, targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		source, err := chars.Get(ctx, sourceID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !source.Active || !target.Active {
			return ErrMergeInactive
		}
		if source.PlayerID != target.PlayerID {
			return ErrMergeDifferentOwner
		}

		// Reject ambiguous duplicates rather than dropping an attendance/signup/wishlist row.
		conflicts, err := chars.CountMergeConflicts(ctx, sourceID, targetID)
		if err != nil {
			return err
		}
		if conflicts != 0 {
			return ErrMergeConflict
		}
		now := time.Now().UTC()
		// Character references and their owner snapshots are corrected together.
		if err := chars.MoveReferences(ctx, sourceID, target, now); err != nil {
			return err
		}
		// Persist an alias to prevent subsequent legacy imports from recreating this typo.
		if err := chars.InsertMergeAlias(ctx, source.Name, targetID, sourceID, now); err != nil {
			return err
		}
		audits := audit.NewRepo(tx)
		if err := audits.Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "character", EntityID: fmt.Sprint(sourceID), Action: "merge_duplicate_source", BeforeJSON: fmt.Sprintf("{\"id\":%d,\"name\":%q,\"active\":true}", sourceID, source.Name), AfterJSON: fmt.Sprintf("{\"id\":%d,\"deleted\":true,\"merged_into\":%d}", sourceID, targetID), Reason: reason, CreatedAt: now}); err != nil {
			return err
		}
		if err := audits.Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "character", EntityID: fmt.Sprint(targetID), Action: "merge_duplicate_target", BeforeJSON: fmt.Sprintf("{\"id\":%d,\"name\":%q}", targetID, target.Name), AfterJSON: fmt.Sprintf("{\"id\":%d,\"merged_source\":%d}", targetID, sourceID), Reason: reason, CreatedAt: now}); err != nil {
			return err
		}
		return chars.Delete(ctx, sourceID)
	})
}
