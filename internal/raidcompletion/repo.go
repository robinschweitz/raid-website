package raidcompletion

import (
	"context"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
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
	audit      *audit.Repo
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db, raids: raid.NewRepo(db), attendance: attendance.NewRepo(db), loot: loot.NewRepo(db), points: enhancepoints.NewRepo(db), series: raidseries.NewRepo(db), audit: audit.NewRepo(db)}
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

// declinedPlayers returns the players who announced an absence for the raid.
func (r *Repo) declinedPlayers(ctx context.Context, raidID int64) (map[int64]bool, error) {
	var ids []int64
	if err := r.db.NewSelect().TableExpr("raid_absences").Column("player_id").Where("raid_id = ?", raidID).Scan(ctx, &ids); err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
