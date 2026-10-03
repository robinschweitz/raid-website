package poll

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

func (r *Repo) CreatePoll(ctx context.Context, p *Poll) error {
	_, err := r.db.NewInsert().Model(p).Exec(ctx)
	return err
}

func (r *Repo) GetPoll(ctx context.Context, id int64) (*Poll, error) {
	p := new(Poll)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repo) ListPolls(ctx context.Context) ([]Poll, error) {
	var ps []Poll
	err := r.db.NewSelect().Model(&ps).Order("id DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

func (r *Repo) UpdatePoll(ctx context.Context, p *Poll) error {
	_, err := r.db.NewUpdate().Model(p).WherePK().Exec(ctx)
	return err
}

func (r *Repo) UpsertVote(ctx context.Context, v *Vote) error {
	_, err := r.db.NewInsert().Model(v).
		On("CONFLICT (poll_id, player_id) DO UPDATE").
		Set("choice = EXCLUDED.choice").
		Set("voted_at = EXCLUDED.voted_at").
		Exec(ctx)
	return err
}

func (r *Repo) ListVotes(ctx context.Context, pollID int64) ([]Vote, error) {
	var vs []Vote
	err := r.db.NewSelect().Model(&vs).Where("poll_id = ?", pollID).Order("id ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return vs, nil
}

func (r *Repo) FindVote(ctx context.Context, pollID, playerID int64) (*Vote, error) {
	v := new(Vote)
	err := r.db.NewSelect().Model(v).
		Where("poll_id = ?", pollID).
		Where("player_id = ?", playerID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return v, nil
}
