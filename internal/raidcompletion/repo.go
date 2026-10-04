package raidcompletion

import (
	"context"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
)

// Repo owns the transaction boundary for raid completion and its ledgers.
type Repo struct {
	db         bun.IDB
	raids      *raid.Repo
	attendance *attendance.Repo
	loot       *loot.Repo
	points     *enhancepoints.Repo
	series     *raidseries.Repo
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db, raids: raid.NewRepo(db), attendance: attendance.NewRepo(db), loot: loot.NewRepo(db), points: enhancepoints.NewRepo(db), series: raidseries.NewRepo(db)}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := r.WithTx(tx)
		if err := repo.raids.SerializeLifecycle(ctx); err != nil {
			return err
		}
		return fn(repo)
	})
}
