package roster

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

func (r *Repo) CreateGuildRank(ctx context.Context, g *GuildRank) error {
	_, err := r.db.NewInsert().Model(g).Exec(ctx)
	return err
}

func (r *Repo) GetGuildRank(ctx context.Context, id int64) (*GuildRank, error) {
	g := new(GuildRank)
	err := r.db.NewSelect().Model(g).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *Repo) ListGuildRanks(ctx context.Context) ([]GuildRank, error) {
	var gs []GuildRank
	err := r.db.NewSelect().Model(&gs).Order("sort_order ASC", "name ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return gs, nil
}

func (r *Repo) UpdateGuildRank(ctx context.Context, g *GuildRank) error {
	_, err := r.db.NewUpdate().Model(g).WherePK().Exec(ctx)
	return err
}

func (r *Repo) CreateRaidGroup(ctx context.Context, g *RaidGroup) error {
	_, err := r.db.NewInsert().Model(g).Exec(ctx)
	return err
}

func (r *Repo) GetRaidGroup(ctx context.Context, id int64) (*RaidGroup, error) {
	g := new(RaidGroup)
	err := r.db.NewSelect().Model(g).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *Repo) ListRaidGroups(ctx context.Context) ([]RaidGroup, error) {
	var gs []RaidGroup
	err := r.db.NewSelect().Model(&gs).Order("sort_order ASC", "name ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return gs, nil
}

func (r *Repo) UpdateRaidGroup(ctx context.Context, g *RaidGroup) error {
	_, err := r.db.NewUpdate().Model(g).WherePK().Exec(ctx)
	return err
}
