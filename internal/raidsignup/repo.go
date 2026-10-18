package raidsignup

import (
	"context"
	"github.com/uptrace/bun"
	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"time"
)

type signupRow struct {
	bun.BaseModel `bun:"table:raid_signups"`
	ID            int64 `bun:"id,pk,autoincrement"`
	RaidID        int64
	PlayerID      int64
	CharacterID   int64
	Spec          string
	IsWithdrawn   bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Repo owns signup persistence and the roster transaction boundary.
type Repo struct {
	db          bun.IDB
	raids       *raid.Repo
	players     *player.Repo
	characters  *character.Repo
	attendance  *attendance.Repo
	corrections *raidcorrection.Service
	audit       *audit.Repo
}

// NewRepo constructs the signup repository.
func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db, raids: raid.NewRepo(db), players: player.NewRepo(db), characters: character.NewRepo(db), attendance: attendance.NewRepo(db), corrections: raidcorrection.NewService(raidcorrection.NewRepo(db)), audit: audit.NewRepo(db)}
}

// WithTx binds all collaborating repositories to one transaction.
func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

func (r *Repo) transaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := loot.NewRepo(tx).ReserveWriter(ctx); err != nil {
			return err
		}
		return fn(r.WithTx(tx))
	})
}

func (r *Repo) enabled(ctx context.Context, raidID int64) (bool, error) {
	return r.db.NewSelect().TableExpr("raid_signup_settings").Where("raid_id = ? AND is_enabled = TRUE", raidID).Exists(ctx)
}

func (r *Repo) setEnabled(ctx context.Context, raidID int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO raid_signup_settings (raid_id, is_enabled) VALUES (?, ?) ON CONFLICT(raid_id) DO UPDATE SET is_enabled = excluded.is_enabled", raidID, enabled)
	return err
}

func (r *Repo) list(ctx context.Context, raidID int64) ([]signupRow, error) {
	rows := []signupRow{}
	err := r.db.NewSelect().Model(&rows).Where("raid_id = ?", raidID).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) get(ctx context.Context, raidID, id int64) (*signupRow, error) {
	row := new(signupRow)
	err := r.db.NewSelect().Model(row).Where("raid_id = ? AND id = ?", raidID, id).Scan(ctx)
	return row, err
}

func (r *Repo) save(ctx context.Context, row *signupRow) error {
	if row.ID == 0 {
		_, err := r.db.NewInsert().Model(row).Exec(ctx)
		return err
	}
	_, err := r.db.NewUpdate().Model(row).WherePK().Exec(ctx)
	return err
}

type absenceRow struct {
	bun.BaseModel `bun:"table:raid_absences"`
	ID            int64 `bun:"id,pk,autoincrement"`
	RaidID        int64
	PlayerID      int64
	Reason        string
	Status        string
	CreatedAt     time.Time
}

func (r *Repo) absences(ctx context.Context, raidID int64) ([]absenceRow, error) {
	rows := []absenceRow{}
	err := r.db.NewSelect().Model(&rows).Where("raid_id = ?", raidID).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) absence(ctx context.Context, raidID, playerID int64) (*absenceRow, error) {
	row := new(absenceRow)
	err := r.db.NewSelect().Model(row).Where("raid_id = ? AND player_id = ?", raidID, playerID).Scan(ctx)
	return row, err
}

func (r *Repo) saveAbsence(ctx context.Context, row *absenceRow) error {
	if row.ID == 0 {
		_, err := r.db.NewInsert().Model(row).Exec(ctx)
		return err
	}
	_, err := r.db.NewUpdate().Model(row).WherePK().Exec(ctx)
	return err
}

func (r *Repo) deleteAbsence(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().TableExpr("raid_absences").Where("id = ?", id).Exec(ctx)
	return err
}
