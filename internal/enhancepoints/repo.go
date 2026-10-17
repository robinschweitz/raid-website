package enhancepoints

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/validation"
)

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

// AuditLock records an item-lock change in the same transaction (ADR 077).
func (r *Repo) AuditLock(ctx context.Context, actorID int64, action string, before, after *ItemLock) error {
	var b any
	if before != nil {
		b = before
	}
	return audit.NewRepo(r.db).Record(ctx, actorID, "item_lock", after.ID, action, b, after, "")
}

// InTransaction passes a transaction-bound repository to fn. All queries in fn
// must use that repository, not the connection pool, which has one connection.
func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	if err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Reserve the SQLite writer before lock eligibility reads, including across handles.
		if _, err := tx.ExecContext(ctx, "UPDATE raids SET id = id WHERE id = -1"); err != nil {
			return err
		}
		return fn(r.WithTx(tx))
	}); err != nil {
		return fmt.Errorf("enhance points transaction: %w", err)
	}
	return nil
}

func (r *Repo) InsertTransaction(ctx context.Context, t *Transaction) error {
	_, err := r.db.NewInsert().Model(t).Exec(ctx)
	return err
}

func (r *Repo) ListByPlayer(ctx context.Context, playerID int64) ([]Transaction, error) {
	var ts []Transaction
	err := r.db.NewSelect().Model(&ts).
		Where("player_id = ?", playerID).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return ts, nil
}

func (r *Repo) Balance(ctx context.Context, playerID int64) (int, error) {
	var sum int
	err := r.db.NewSelect().
		ColumnExpr("COALESCE(SUM(amount), 0)").
		TableExpr("enhance_point_transactions").
		Where("player_id = ?", playerID).
		Scan(ctx, &sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

func (r *Repo) HasTransactionType(ctx context.Context, playerID int64, txnType string) (bool, error) {
	count, err := r.db.NewSelect().Model((*Transaction)(nil)).
		Where("player_id = ?", playerID).
		Where("type = ?", txnType).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repo) HasNoMainSpecAward(ctx context.Context, raidID, playerID int64) (bool, error) {
	count, err := r.db.NewSelect().Model((*Transaction)(nil)).
		Where("raid_id = ?", raidID).
		Where("player_id = ?", playerID).
		Where("type = ?", "award_no_mainspec_loot").
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repo) ListActiveLocksByRaid(ctx context.Context, raidID int64) ([]ItemLock, error) {
	var locks []ItemLock
	err := r.db.NewSelect().Model(&locks).
		Where("raid_id = ?", raidID).
		Where("status = ?", validation.LockStatusActive).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return locks, nil
}

// FindActiveLockByItem resolves item identity consistently for creation and loot.
// Use WithTx when resolving a lock as part of a write. Multiple matches return
// ErrLockConflict, including legacy name-only locks; no match returns nil, nil.
func (r *Repo) FindActiveLockByItem(ctx context.Context, raidID int64, itemID, itemName string) (*ItemLock, error) {
	locks, err := r.ListActiveLocksByRaid(ctx, raidID)
	if err != nil {
		return nil, fmt.Errorf("list active locks for raid %d: %w", raidID, err)
	}
	return ResolveActiveLock(locks, itemID, itemName)
}

// GetRaidLockState reads raid lifecycle state using this repository's transaction.
func (r *Repo) GetRaidLockState(ctx context.Context, raidID int64) (validation.RaidStatus, bool, error) {
	var status validation.RaidStatus
	var archivedAt *time.Time
	if err := r.db.NewSelect().TableExpr("raids").Column("status", "archived_at").
		Where("id = ?", raidID).Scan(ctx, &status, &archivedAt); err != nil {
		return "", false, fmt.Errorf("load raid %d lock state: %w", raidID, err)
	}
	return status, archivedAt != nil, nil
}

func (r *Repo) UpdateLock(ctx context.Context, l *ItemLock) error {
	_, err := r.db.NewUpdate().Model(l).WherePK().Exec(ctx)
	return err
}

func (r *Repo) CreateLock(ctx context.Context, l *ItemLock) error {
	_, err := r.db.NewInsert().Model(l).Exec(ctx)
	return err
}

func (r *Repo) GetLock(ctx context.Context, id int64) (*ItemLock, error) {
	l := new(ItemLock)
	err := r.db.NewSelect().Model(l).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (r *Repo) ListLocksByPlayer(ctx context.Context, playerID int64) ([]ItemLock, error) {
	var locks []ItemLock
	err := r.db.NewSelect().Model(&locks).
		Where("player_id = ?", playerID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return locks, nil
}
