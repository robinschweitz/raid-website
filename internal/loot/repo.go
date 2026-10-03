package loot

import (
	"context"

	"github.com/uptrace/bun"
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

func (r *Repo) CreateAward(ctx context.Context, a *Award) error {
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *Repo) ListByRaid(ctx context.Context, raidID int64) ([]Award, error) {
	var as []Award
	err := r.db.NewSelect().Model(&as).
		Where("raid_id = ?", raidID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (r *Repo) PlayerHasMainSpecAward(ctx context.Context, raidID, playerID int64) (bool, error) {
	count, err := r.db.NewSelect().Model((*Award)(nil)).
		Where("raid_id = ?", raidID).
		Where("awarded_to_player_id = ?", playerID).
		Where("is_main_spec = ?", true).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
