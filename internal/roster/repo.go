package roster

import (
	"context"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
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

// audited runs one write and its audit entry in a single transaction (ADR 077).
// The entity ID is read after the write so inserts can report their new ID.
func (r *Repo) audited(ctx context.Context, actorID int64, entityType, action string, before any, after any, id func() int64, write func(bun.Tx) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := write(tx); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, entityType, id(), action, before, after, "")
	})
}

func (r *Repo) CreateGuildRank(ctx context.Context, g *GuildRank, actorID int64) error {
	return r.audited(ctx, actorID, "guild_rank", "create", nil, g, func() int64 { return g.ID }, func(tx bun.Tx) error {
		_, err := tx.NewInsert().Model(g).Exec(ctx)
		return err
	})
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

func (r *Repo) UpdateGuildRank(ctx context.Context, g *GuildRank, before GuildRank, actorID int64) error {
	return r.audited(ctx, actorID, "guild_rank", "update", before, g, func() int64 { return g.ID }, func(tx bun.Tx) error {
		_, err := tx.NewUpdate().Model(g).WherePK().Exec(ctx)
		return err
	})
}

func (r *Repo) CreateRaidGroup(ctx context.Context, g *RaidGroup, actorID int64) error {
	return r.audited(ctx, actorID, "raid_group", "create", nil, g, func() int64 { return g.ID }, func(tx bun.Tx) error {
		_, err := tx.NewInsert().Model(g).Exec(ctx)
		return err
	})
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

func (r *Repo) UpdateRaidGroup(ctx context.Context, g *RaidGroup, before RaidGroup, actorID int64) error {
	return r.audited(ctx, actorID, "raid_group", "update", before, g, func() int64 { return g.ID }, func(tx bun.Tx) error {
		_, err := tx.NewUpdate().Model(g).WherePK().Exec(ctx)
		return err
	})
}
