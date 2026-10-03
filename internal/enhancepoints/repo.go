package enhancepoints

import (
	"context"

	"github.com/uptrace/bun"

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

func (r *Repo) UpdateLock(ctx context.Context, l *ItemLock) error {
	_, err := r.db.NewUpdate().Model(l).WherePK().Exec(ctx)
	return err
}
