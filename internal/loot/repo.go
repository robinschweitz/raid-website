package loot

import (
	"context"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/tokens"
)

type Repo struct {
	db bun.IDB
}

type awardRepositories struct {
	raids      *raid.Repo
	players    *player.Repo
	characters *character.Repo
	attendance *attendance.Repo
	points     *enhancepoints.Repo
	tokens     *tokens.Repo
	audit      *audit.Repo
}

func (r *Repo) awardRepositories() awardRepositories {
	return awardRepositories{raids: raid.NewRepo(r.db), players: player.NewRepo(r.db), characters: character.NewRepo(r.db), attendance: attendance.NewRepo(r.db), points: enhancepoints.NewRepo(r.db), tokens: tokens.NewRepo(r.db), audit: audit.NewRepo(r.db)}
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

// Catalog binds validation to the award transaction.
func (r *Repo) Catalog() *lootcatalog.Service {
	return lootcatalog.NewService(lootcatalog.NewRepo(r.db))
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

// InTransaction keeps loot, derived tokens, and lock consumption atomic.
func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := r.WithTx(tx)
		if err := repo.ReserveWriter(ctx); err != nil {
			return err
		}
		return fn(repo)
	})
}

// ReserveWriter obtains SQLite's writer reservation before any balance reads.
func (r *Repo) ReserveWriter(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "UPDATE raids SET id = id WHERE id = -1")
	return err
}

// GetByRequest looks up the immutable original request association.
func (r *Repo) GetByRequest(ctx context.Context, actor int64, requestID string) (*Award, error) {
	a := new(Award)
	err := r.db.NewSelect().Model(a).Where("awarded_by_player_id = ? AND request_id = ?", actor, requestID).Scan(ctx)
	return a, err
}

func (r *Repo) CreateAward(ctx context.Context, a *Award) error {
	if err := a.DeriveMainSpec(); err != nil {
		return err
	}
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Award, error) {
	a := new(Award)
	err := r.db.NewSelect().Model(a).Where("id = ?", id).Scan(ctx)
	return a, err
}

func (r *Repo) Update(ctx context.Context, a *Award) error {
	if err := a.DeriveMainSpec(); err != nil {
		return err
	}
	_, err := r.db.NewUpdate().Model(a).ExcludeColumn("request_id", "request_hash").WherePK().Exec(ctx)
	return err
}

func (r *Repo) ListByRaid(ctx context.Context, raidID int64) ([]Award, error) {
	var as []Award
	err := r.db.NewSelect().Model(&as).
		Where("raid_id = ?", raidID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (r *Repo) ListByPlayer(ctx context.Context, playerID int64) ([]Award, error) {
	var as []Award
	err := r.db.NewSelect().Model(&as).
		Where("awarded_to_player_id = ?", playerID).
		Order("awarded_at DESC", "id DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (r *Repo) PlayerHasMainSpecAward(ctx context.Context, raidID, playerID int64) (bool, error) {
	count, err := r.db.NewSelect().Model((*Award)(nil)).
		Where("raid_id = ?", raidID).
		Where("awarded_to_player_id = ?", playerID).
		Where("is_main_spec = ?", true).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
