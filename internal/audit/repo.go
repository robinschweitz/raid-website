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

func (r *Repo) List(ctx context.Context) ([]Entry, error) {
	var entries []Entry
	err := r.db.NewSelect().
		TableExpr("audit_log AS audit").
		ColumnExpr("audit.id, audit.actor_id, players.name AS actor_name, audit.entity_type, audit.entity_id, audit.action, audit.before_json, audit.after_json, audit.reason, audit.created_at").
		Join("LEFT JOIN players ON players.id = audit.actor_id").
		OrderExpr("audit.created_at DESC, audit.id DESC").
		Scan(ctx, &entries)
	if err != nil {
		return nil, err
	}
	return entries, nil
}
