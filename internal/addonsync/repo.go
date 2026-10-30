package addonsync

import (
	"context"
	"github.com/uptrace/bun"
	"raid-website/internal/bosskill"
	"raid-website/internal/loot"
)

// Repo owns the transaction boundary for all-or-nothing addon loot imports.
type Repo struct{ db bun.IDB }

// NewRepo constructs the addon import transaction boundary.
func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

// WithTx binds the coordinator to an existing transaction.
func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

// InTransaction reserves the writer before reading import state.
func (r *Repo) InTransaction(ctx context.Context, fn func(bun.Tx) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := loot.NewRepo(tx).ReserveWriter(ctx); err != nil {
			return err
		}
		return fn(tx)
	})
}

// Kills binds kill reads to this coordinator's database or transaction.
func (r *Repo) Kills() *bosskill.Repo { return bosskill.NewRepo(r.db) }
