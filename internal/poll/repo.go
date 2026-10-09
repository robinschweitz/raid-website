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

func (r *Repo) CreatePollWithOptions(ctx context.Context, p *Poll, labels []string) error {
	p.Options = []Option{}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := r.WithTx(tx).CreatePoll(ctx, p); err != nil {
			return err
		}
		for i, label := range labels {
			opt := &Option{PollID: p.ID, Label: label, SortOrder: i + 1}
			if _, err := tx.NewInsert().Model(opt).Exec(ctx); err != nil {
				return err
			}
			p.Options = append(p.Options, *opt)
		}
		return nil
	})
}

func (r *Repo) GetPoll(ctx context.Context, id int64) (*Poll, error) {
	p := new(Poll)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	options, err := r.ListOptions(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if options == nil {
		options = []Option{}
	}
	p.Options = options
	return p, nil
}

func (r *Repo) ListOptions(ctx context.Context, pollID int64) ([]Option, error) {
	var opts []Option
	err := r.db.NewSelect().Model(&opts).Where("poll_id = ?", pollID).Order("sort_order ASC, id ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = []Option{}
	}
	return opts, nil
}

func (r *Repo) ListPolls(ctx context.Context) ([]Poll, error) {
	var ps []Poll
	err := r.db.NewSelect().Model(&ps).Order("id DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.loadOptions(ctx, ps); err != nil {
		return nil, err
	}
	for i := range ps {
		if ps[i].Options == nil {
			ps[i].Options = []Option{}
		}
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

func (r *Repo) UpsertVoteWithChoices(ctx context.Context, v *Vote, optionIDsByLabel map[string]int64) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(v).
			On("CONFLICT (poll_id, player_id) DO UPDATE").
			Set("choice = EXCLUDED.choice").
			Set("voted_at = EXCLUDED.voted_at").
			Exec(ctx); err != nil {
			return err
		}
		if v.ID == 0 {
			if err := tx.NewSelect().Model(v).
				Where("poll_id = ?", v.PollID).
				Where("player_id = ?", v.PlayerID).
				Scan(ctx); err != nil {
				return err
			}
		}
		if _, err := tx.NewDelete().Model((*VoteChoice)(nil)).Where("vote_id = ?", v.ID).Exec(ctx); err != nil {
			return err
		}
		if optionIDsByLabel != nil {
			for i, label := range v.Choices {
				vc := &VoteChoice{VoteID: v.ID, OptionID: optionIDsByLabel[label], Label: label, SortOrder: i + 1}
				if _, err := tx.NewInsert().Model(vc).Exec(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *Repo) ListVotes(ctx context.Context, pollID int64) ([]Vote, error) {
	var vs []Vote
	err := r.db.NewSelect().Model(&vs).Where("poll_id = ?", pollID).Order("id ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.loadVoteChoices(ctx, vs); err != nil {
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
	choices, err := r.ListVoteChoices(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	if choices == nil {
		choices = []string{}
	}
	v.Choices = choices
	if len(v.Choices) == 0 && v.Choice != "" {
		v.Choices = []string{v.Choice}
	}
	return v, nil
}

func (r *Repo) ListVoteChoices(ctx context.Context, voteID int64) ([]string, error) {
	var choices []VoteChoice
	if err := r.db.NewSelect().Model(&choices).Where("vote_id = ?", voteID).Order("sort_order ASC, id ASC").Scan(ctx); err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(choices))
	for _, choice := range choices {
		labels = append(labels, choice.Label)
	}
	return labels, nil
}

func (r *Repo) loadOptions(ctx context.Context, polls []Poll) error {
	if len(polls) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(polls))
	byID := make(map[int64]*Poll, len(polls))
	for i := range polls {
		ids = append(ids, polls[i].ID)
		byID[polls[i].ID] = &polls[i]
	}
	var opts []Option
	if err := r.db.NewSelect().Model(&opts).Where("poll_id IN (?)", bun.In(ids)).Order("poll_id ASC, sort_order ASC, id ASC").Scan(ctx); err != nil {
		return err
	}
	for _, opt := range opts {
		p := byID[opt.PollID]
		if p != nil {
			p.Options = append(p.Options, opt)
		}
	}
	return nil
}

func (r *Repo) loadVoteChoices(ctx context.Context, votes []Vote) error {
	if len(votes) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(votes))
	byID := make(map[int64]*Vote, len(votes))
	for i := range votes {
		ids = append(ids, votes[i].ID)
		byID[votes[i].ID] = &votes[i]
	}
	var choices []VoteChoice
	if err := r.db.NewSelect().Model(&choices).Where("vote_id IN (?)", bun.In(ids)).Order("vote_id ASC, sort_order ASC, id ASC").Scan(ctx); err != nil {
		return err
	}
	for _, choice := range choices {
		v := byID[choice.VoteID]
		if v != nil {
			v.Choices = append(v.Choices, choice.Label)
		}
	}
	for i := range votes {
		if len(votes[i].Choices) == 0 && votes[i].Choice != "" {
			votes[i].Choices = []string{votes[i].Choice}
		}
		if votes[i].Choices == nil {
			votes[i].Choices = []string{}
		}
	}
	return nil
}
