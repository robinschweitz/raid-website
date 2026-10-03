package tokens

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

func (r *Repo) HasForLootAward(ctx context.Context, lootAwardID int64) (bool, error) {
	count, err := r.db.NewSelect().Model((*Transaction)(nil)).
		Where("loot_award_id = ?", lootAwardID).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
