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

func (r *Repo) ArchivePlayer(ctx context.Context, id int64, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().Table("players").Set("active = ?", false).Set("updated_at = ?", updatedAt).Where("id = ?", id).Exec(ctx)
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
