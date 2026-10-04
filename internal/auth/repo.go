package auth

import (
	"context"
	"time"

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

// InTransaction runs account changes using a transaction-bound player repository.
func (r *Repo) InTransaction(ctx context.Context, fn func(*player.Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(player.NewRepo(tx))
	})
}

func (r *Repo) CreateSession(ctx context.Context, s *Session) error {
	_, err := r.db.NewInsert().Model(s).Exec(ctx)
	return err
}

func (r *Repo) FindSessionByHash(ctx context.Context, tokenHash string) (*Session, error) {
	s := new(Session)
	err := r.db.NewSelect().Model(s).Where("token_hash = ?", tokenHash).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *Repo) TouchSession(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.NewUpdate().
		Model((*Session)(nil)).
		Set("last_seen_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *Repo) DeleteSession(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*Session)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) DeleteByHash(ctx context.Context, tokenHash string) error {
	_, err := r.db.NewDelete().Model((*Session)(nil)).Where("token_hash = ?", tokenHash).Exec(ctx)
	return err
}

func (r *Repo) DeleteExpired(ctx context.Context, before time.Time) error {
	_, err := r.db.NewDelete().Model((*Session)(nil)).Where("expires_at < ?", before).Exec(ctx)
	return err
}
