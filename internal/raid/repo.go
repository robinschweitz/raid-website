package raid

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"raid-website/internal/player"
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

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error { return fn(r.WithTx(tx)) })
}

func (r *Repo) RequireActor(ctx context.Context, actorID int64) error {
	_, err := player.NewRepo(r.db).Get(ctx, actorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrActorNotFound
	}
	return err
}

func (r *Repo) Create(ctx context.Context, rd *Raid) error {
	_, err := r.db.NewInsert().Model(rd).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Raid, error) {
	rd := new(Raid)
	err := r.db.NewSelect().Model(rd).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Join(ErrNotFound, err)
	}
	if err != nil {
		return nil, err
	}
	return rd, nil
}

func (r *Repo) List(ctx context.Context, includeArchived bool) ([]Raid, error) {
	var rs []Raid
	q := r.db.NewSelect().Model(&rs)
	if !includeArchived {
		q = q.Where("archived_at IS NULL")
	}
	err := q.Order("scheduled_start DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (r *Repo) Update(ctx context.Context, rd *Raid) error {
	_, err := r.db.NewUpdate().Model(rd).WherePK().Exec(ctx)
	return err
}

func (r *Repo) FindByName(ctx context.Context, name string) (*Raid, error) {
	rd := new(Raid)
	err := r.db.NewSelect().Model(rd).Where("name = ?", name).Order("id ASC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return rd, nil
}
