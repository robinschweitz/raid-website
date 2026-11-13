package enhancepoints

import (
	"context"
	"database/sql"
	"errors"
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

// InsertTransaction appends a ledger row. A row without a pool, such as an
// imported opening balance, counts in the first pool (ADR 113).
func (r *Repo) InsertTransaction(ctx context.Context, t *Transaction) error {
	if t.PoolID == 0 {
		poolID, err := r.DefaultPoolID(ctx)
		if err != nil {
			return fmt.Errorf("default EP pool: %w", err)
		}
		t.PoolID = poolID
	}
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

// PublicBalance is the only EP data visitors see (FR-16.9).
type PublicBalance struct {
	PlayerID   int64  `bun:"player_id" json:"player_id"`
	PlayerName string `bun:"player_name" json:"player_name"`
	Balance    int    `bun:"balance" json:"balance"`
}

// ActiveBalances lists every active player's balance in one pool, highest
// first, then by name (FR-16.9).
func (r *Repo) ActiveBalances(ctx context.Context, poolID int64) ([]PublicBalance, error) {
	rows := []PublicBalance{}
	err := r.db.NewSelect().
		TableExpr("players AS p").
		ColumnExpr("p.id AS player_id, p.name AS player_name").
		ColumnExpr("COALESCE((SELECT SUM(t.amount) FROM enhance_point_transactions AS t WHERE t.player_id = p.id AND t.pool_id = ?), 0) AS balance", poolID).
		Where("p.active = TRUE").
		OrderExpr("balance DESC, p.name ASC, p.id ASC").
		Scan(ctx, &rows)
	return rows, err
}

// Balance sums a player's transactions in one EP pool (FR-16.1).
func (r *Repo) Balance(ctx context.Context, playerID, poolID int64) (int, error) {
	var sum int
	err := r.db.NewSelect().
		ColumnExpr("COALESCE(SUM(amount), 0)").
		TableExpr("enhance_point_transactions").
		Where("player_id = ?", playerID).
		Where("pool_id = ?", poolID).
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

// RaidEP reads the EP pool and award a raid copied from its series (ADR 113).
// A nil pool means the raid gives and spends no points.
func (r *Repo) RaidEP(ctx context.Context, raidID int64) (*int64, int, error) {
	var poolID *int64
	var award int
	if err := r.db.NewSelect().TableExpr("raids").Column("ep_pool_id", "ep_award").
		Where("id = ?", raidID).Scan(ctx, &poolID, &award); err != nil {
		return nil, 0, fmt.Errorf("load raid %d EP pool: %w", raidID, err)
	}
	return poolID, award, nil
}

// RaidHasEP reports whether a raid already has EP transactions or item locks,
// which fix its pool and award (FR-16.11).
func (r *Repo) RaidHasEP(ctx context.Context, raidID int64) (bool, error) {
	transactions, err := r.db.NewSelect().Model((*Transaction)(nil)).Where("raid_id = ?", raidID).Count(ctx)
	if err != nil {
		return false, err
	}
	locks, err := r.db.NewSelect().Model((*ItemLock)(nil)).Where("raid_id = ?", raidID).Count(ctx)
	if err != nil {
		return false, err
	}
	return transactions+locks > 0, nil
}

// ListPools returns every EP pool, oldest first; the first is the default view.
func (r *Repo) ListPools(ctx context.Context) ([]Pool, error) {
	pools := []Pool{}
	err := r.db.NewSelect().Model(&pools).Order("id ASC").Scan(ctx)
	return pools, err
}

// DefaultPoolID is the pool shown when a viewer chose none: the first one.
func (r *Repo) DefaultPoolID(ctx context.Context) (int64, error) {
	var id int64
	err := r.db.NewSelect().Model((*Pool)(nil)).Column("id").Order("id ASC").Limit(1).Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrPoolNotFound
	}
	return id, err
}

// GetPool loads one pool; an unknown ID gives ErrPoolNotFound.
func (r *Repo) GetPool(ctx context.Context, id int64) (*Pool, error) {
	pool := new(Pool)
	err := r.db.NewSelect().Model(pool).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPoolNotFound
	}
	if err != nil {
		return nil, err
	}
	return pool, nil
}

// PoolNameTaken reports whether another pool already uses the name.
func (r *Repo) PoolNameTaken(ctx context.Context, name string, exceptID int64) (bool, error) {
	count, err := r.db.NewSelect().Model((*Pool)(nil)).Where("name = ?", name).Where("id != ?", exceptID).Count(ctx)
	return count > 0, err
}

func (r *Repo) InsertPool(ctx context.Context, pool *Pool) error {
	_, err := r.db.NewInsert().Model(pool).Exec(ctx)
	return err
}

func (r *Repo) UpdatePool(ctx context.Context, pool *Pool) error {
	_, err := r.db.NewUpdate().Model(pool).WherePK().Exec(ctx)
	return err
}

// AuditPool records a pool change in the same transaction (FR-16.10).
func (r *Repo) AuditPool(ctx context.Context, actorID int64, action string, before, after *Pool) error {
	var b any
	if before != nil {
		b = before
	}
	return audit.NewRepo(r.db).Record(ctx, actorID, "ep_pool", after.ID, action, b, after, "")
}

// ListByPlayerInPool lists a player's transactions in one pool, oldest first.
func (r *Repo) ListByPlayerInPool(ctx context.Context, playerID, poolID int64) ([]Transaction, error) {
	ts := []Transaction{}
	err := r.db.NewSelect().Model(&ts).
		Where("player_id = ?", playerID).
		Where("pool_id = ?", poolID).
		Order("created_at ASC").
		Scan(ctx)
	return ts, err
}
