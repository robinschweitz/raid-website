package accountmerge

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Reserve the SQLite writer before checking cross-account lock conflicts.
		if _, err := tx.ExecContext(ctx, "UPDATE players SET id = id WHERE id = -1"); err != nil {
			return err
		}
		return fn(NewRepo(tx))
	})
}

func (r *Repo) GetPlayer(ctx context.Context, id int64) (*player.Player, error) {
	p := new(player.Player)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	return p, err
}

func (r *Repo) ListCharacters(ctx context.Context, playerID int64) ([]character.Character, error) {
	var rows []character.Character
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) UpdateCharacterOwner(ctx context.Context, id, playerID int64, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().Table("characters").Set("player_id = ?", playerID).Set("updated_at = ?", updatedAt).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) ListLocks(ctx context.Context, playerID int64) ([]enhancepoints.ItemLock, error) {
	var rows []enhancepoints.ItemLock
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) ListActiveLocks(ctx context.Context, playerID int64) ([]enhancepoints.ItemLock, error) {
	var rows []enhancepoints.ItemLock
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).Where("status = ?", validation.LockStatusActive).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) UpdateLockOwner(ctx context.Context, id, playerID int64, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().Table("item_locks").Set("player_id = ?", playerID).Set("updated_at = ?", updatedAt).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) Balance(ctx context.Context, playerID int64) (int, error) {
	var balance int
	err := r.db.NewSelect().TableExpr("enhance_point_transactions").ColumnExpr("COALESCE(SUM(amount), 0)").Where("player_id = ?", playerID).Scan(ctx, &balance)
	return balance, err
}

func (r *Repo) InsertPointTransaction(ctx context.Context, row *enhancepoints.Transaction) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *Repo) RevokeSourceAuth(ctx context.Context, playerID int64) (map[string]int64, error) {
	tables := []string{"sessions", "webauthn_sessions", "mfa_login_challenges", "mfa_recovery_codes", "webauthn_credentials", "player_mfa_settings"}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		result, err := r.db.NewDelete().Table(table).Where("player_id = ?", playerID).Exec(ctx)
		if err != nil {
			return nil, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		counts[table] = count
	}
	return counts, nil
}

func (r *Repo) MoveHistoricalReferences(ctx context.Context, sourceID, targetID int64) error {
	// Keep both awards when an actor-scoped request ID collides after the merge.
	if _, err := r.db.NewUpdate().Table("loot_awards").Set("request_id = 'account-merge-' || ? || '-' || id", sourceID).Where("awarded_by_player_id = ?", sourceID).Where("request_id IS NOT NULL").Where("EXISTS (SELECT 1 FROM loot_awards AS target WHERE target.awarded_by_player_id = ? AND target.request_id = loot_awards.request_id)", targetID).Exec(ctx); err != nil {
		return err
	}
	// Poll votes and quick-add requests have per-player unique keys. The target's existing
	// key wins; the source key cannot survive after its player is deleted.
	if _, err := r.db.NewDelete().Table("poll_votes").Where("player_id = ?", sourceID).Where("EXISTS (SELECT 1 FROM poll_votes AS target WHERE target.poll_id = poll_votes.poll_id AND target.player_id = ?)", targetID).Exec(ctx); err != nil {
		return err
	}
	if _, err := r.db.NewDelete().Table("raid_quick_add_requests").Where("actor_id = ?", sourceID).Where("EXISTS (SELECT 1 FROM raid_quick_add_requests AS target WHERE target.raid_id = raid_quick_add_requests.raid_id AND target.actor_id = ? AND target.request_id = raid_quick_add_requests.request_id)", targetID).Exec(ctx); err != nil {
		return err
	}
	statements := []struct{ table, column string }{
		{"character_name_history", "actor_id"}, {"raid_series", "raid_leader_id"}, {"raids", "raid_leader_id"},
		{"raid_attendance", "player_id"}, {"raid_signups", "player_id"}, {"character_wishlist", "player_id"},
		{"loot_awards", "awarded_to_player_id"}, {"loot_awards", "awarded_by_player_id"}, {"enhance_point_transactions", "player_id"},
		{"enhance_point_transactions", "actor_id"}, {"item_locks", "actor_id"}, {"polls", "created_by"},
		{"poll_votes", "player_id"}, {"legacy_imports", "actor_id"}, {"audit_log", "actor_id"},
		{"raid_quick_add_requests", "actor_id"}, {"registration_requests", "requester_player_id"},
		{"registration_codes", "created_by_player_id"}, {"registration_codes", "used_by_player_id"},
	}
	for _, statement := range statements {
		if _, err := r.db.NewUpdate().Table(statement.table).Set(statement.column+" = ?", targetID).Where(statement.column+" = ?", sourceID).Exec(ctx); err != nil {
			return fmt.Errorf("move %s.%s: %w", statement.table, statement.column, err)
		}
	}
	return nil
}

func (r *Repo) DeletePlayer(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Table("players").Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) Audit(ctx context.Context, actorID int64, entityType string, entityID any, action string, before, after any, reason string, now time.Time) error {
	beforeJSON, err := auditJSON(before)
	if err != nil {
		return fmt.Errorf("marshal audit before: %w", err)
	}
	afterJSON, err := auditJSON(after)
	if err != nil {
		return fmt.Errorf("marshal audit after: %w", err)
	}
	entity := fmt.Sprint(entityID)
	if id, ok := entityID.(int64); ok {
		entity = strconv.FormatInt(id, 10)
	}
	_, err = r.db.NewInsert().Model(&audit.Log{ActorID: &actorID, EntityType: entityType, EntityID: entity, Action: action, BeforeJSON: beforeJSON, AfterJSON: afterJSON, Reason: reason, CreatedAt: now}).Exec(ctx)
	return err
}
