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
	return s.repo.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE players SET id = id WHERE id = -1"); err != nil {
			return err
		}
		chars := NewRepo(tx)
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
		checks := []string{
			"SELECT COUNT(*) FROM raid_attendance s JOIN raid_attendance t ON s.raid_id=t.raid_id WHERE s.character_id=? AND t.character_id=?",
			"SELECT COUNT(*) FROM raid_signups s JOIN raid_signups t ON s.raid_id=t.raid_id WHERE s.character_id=? AND t.character_id=?",
			"SELECT COUNT(*) FROM character_wishlist s JOIN character_wishlist t ON s.item_id=t.item_id WHERE s.character_id=? AND t.character_id=?",
		}
		for _, query := range checks {
			var n int
			if err := tx.QueryRowContext(ctx, query, sourceID, targetID).Scan(&n); err != nil {
				return fmt.Errorf("check merge conflicts: %w", err)
			}
			if n != 0 {
				return ErrMergeConflict
			}
		}
		now := time.Now().UTC()
		// Character references and their owner snapshots are corrected together.
		statements := []struct {
			query string
			args  []any
		}{
			{"UPDATE character_name_history SET character_id=? WHERE character_id=?", []any{targetID, sourceID}},
			{"UPDATE raid_attendance SET player_id=?, character_id=?, character_name=?, updated_at=? WHERE character_id=?", []any{target.PlayerID, targetID, target.Name, now, sourceID}},
			{"UPDATE loot_awards SET awarded_to_player_id=?, awarded_to_character_id=?, awarded_to_character_name=?, updated_at=? WHERE awarded_to_character_id=?", []any{target.PlayerID, targetID, target.Name, now, sourceID}},
			{"UPDATE raid_signups SET player_id=?, character_id=?, updated_at=? WHERE character_id=?", []any{target.PlayerID, targetID, now, sourceID}},
			{"UPDATE character_wishlist SET character_id=?, player_id=?, updated_at=? WHERE character_id=?", []any{targetID, target.PlayerID, now, sourceID}},
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return fmt.Errorf("move character references: %w", err)
			}
		}
		// Persist an alias to prevent subsequent legacy imports from recreating this typo.
		if _, err := tx.ExecContext(ctx, "INSERT INTO character_merge_aliases (source_name, target_character_id, source_character_id, created_at) VALUES (?, ?, ?, ?)", source.Name, targetID, sourceID, now); err != nil {
			return fmt.Errorf("store character alias: %w", err)
		}
		audits := audit.NewRepo(tx)
		if err := audits.Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "character", EntityID: fmt.Sprint(sourceID), Action: "merge_duplicate_source", BeforeJSON: fmt.Sprintf("{\"id\":%d,\"name\":%q,\"active\":true}", sourceID, source.Name), AfterJSON: fmt.Sprintf("{\"id\":%d,\"deleted\":true,\"merged_into\":%d}", sourceID, targetID), Reason: reason, CreatedAt: now}); err != nil {
			return err
		}
		if err := audits.Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "character", EntityID: fmt.Sprint(targetID), Action: "merge_duplicate_target", BeforeJSON: fmt.Sprintf("{\"id\":%d,\"name\":%q}", targetID, target.Name), AfterJSON: fmt.Sprintf("{\"id\":%d,\"merged_source\":%d}", targetID, sourceID), Reason: reason, CreatedAt: now}); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM characters WHERE id=?", sourceID); err != nil {
			return fmt.Errorf("delete source character: %w", err)
		}
		return nil
	})
}
