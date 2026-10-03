package audit

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

func (r *Repo) Insert(ctx context.Context, l *Log) error {
	_, err := r.db.NewInsert().Model(l).Exec(ctx)
	return err
}

func (r *Repo) ListByEntity(ctx context.Context, entityType, entityID string) ([]Log, error) {
	var logs []Log
	err := r.db.NewSelect().Model(&logs).
		Where("entity_type = ?", entityType).
		Where("entity_id = ?", entityID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return logs, nil
}
