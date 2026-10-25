package player

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/validation"
)

// ErrLastAdmin is returned when a change would leave no active ADMIN (SC-047).
var ErrLastAdmin = errors.New("last active admin")

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

// AccountAudit is the audit snapshot of an account change. The password hash
// is never part of a player snapshot; only whether it changed is recorded.
type AccountAudit struct {
	*Player
	PasswordChanged bool `json:"password_changed"`
}

// Audit records a player or account change in the same transaction (ADR 077).
func (r *Repo) Audit(ctx context.Context, actorID, playerID int64, action string, before, after any) error {
	return audit.NewRepo(r.db).Record(ctx, actorID, "player", playerID, action, before, after, "")
}

// CreateAudited creates a player and records who created it.
func (r *Repo) CreateAudited(ctx context.Context, p *Player, actorID int64) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := r.WithTx(tx)
		if err := repo.Create(ctx, p); err != nil {
			return err
		}
		return repo.Audit(ctx, actorID, p.ID, "create", nil, p)
	})
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

// CountActiveAdmins returns the number of active ADMIN accounts.
func (r *Repo) CountActiveAdmins(ctx context.Context) (int, error) {
	count, err := r.db.NewSelect().Model((*Player)(nil)).
		Where("role = ?", validation.RoleAdmin).
		Where("active = ?", true).
		Count(ctx)
	return int(count), err
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
