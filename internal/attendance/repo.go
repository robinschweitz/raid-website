package attendance

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

func (r *Repo) Create(ctx context.Context, a *Attendance) error {
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *Repo) Exists(ctx context.Context, raidID, playerID, characterID int64) (bool, error) {
	count, err := r.db.NewSelect().Model((*Attendance)(nil)).
		Where("raid_id = ?", raidID).
		Where("player_id = ?", playerID).
		Where("character_id = ?", characterID).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repo) ListByRaid(ctx context.Context, raidID int64) ([]Attendance, error) {
	var as []Attendance
	err := r.db.NewSelect().Model(&as).
		Where("raid_id = ?", raidID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return as, nil
}
