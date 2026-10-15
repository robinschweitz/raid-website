package player

import (
	"context"
	"database/sql"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
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

func (r *Repo) Create(ctx context.Context, p *Player) error {
	_, err := r.db.NewInsert().Model(p).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Player, error) {
	p := new(Player)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*Player, error) {
	p := new(Player)
	err := r.db.NewSelect().Model(p).Where("name = ?", name).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repo) FindByUsername(ctx context.Context, username string) (*Player, error) {
	p := new(Player)
	err := r.db.NewSelect().Model(p).Where("username = ?", username).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// FindActiveAdmin returns a real account that can be used for server-initiated audits.
func (r *Repo) FindActiveAdmin(ctx context.Context) (*Player, error) {
	p := new(Player)
	err := r.db.NewSelect().Model(p).
		Where("role = ?", validation.RoleAdmin).
		Where("active = ?", true).
		OrderExpr("id ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return p, nil
}

func (r *Repo) Update(ctx context.Context, p *Player) error {
	_, err := r.db.NewUpdate().Model(p).WherePK().Exec(ctx)
	return err
}

func (r *Repo) List(ctx context.Context) ([]Player, error) {
	var ps []Player
	err := r.db.NewSelect().Model(&ps).Order("name ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return ps, nil
}
