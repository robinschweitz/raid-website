package attendance

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

func (r *Repo) Create(ctx context.Context, a *Attendance) error {
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Attendance, error) {
	a := new(Attendance)
	err := r.db.NewSelect().Model(a).Where("id = ?", id).Scan(ctx)
	return a, err
}

func (r *Repo) Update(ctx context.Context, a *Attendance) error {
	_, err := r.db.NewUpdate().Model(a).WherePK().Exec(ctx)
	return err
}

// Delete hard-removes one roster row. Only the raid correction service calls
// it, for planned or open raids (ADR 076).
func (r *Repo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*Attendance)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) Exists(ctx context.Context, raidID, playerID, characterID int64) (bool, error) {
	count, err := r.db.NewSelect().Model((*Attendance)(nil)).
		Where("raid_id = ?", raidID).
		Where("player_id = ?", playerID).
		Where("character_id = ?", characterID).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// LatestByCharacter returns the character's most recent attendance in any raid.
func (r *Repo) LatestByCharacter(ctx context.Context, characterID int64) (*Attendance, error) {
	a := new(Attendance)
	err := r.db.NewSelect().Model(a).
		Where("character_id = ?", characterID).
		Order("created_at DESC", "id DESC").
		Limit(1).
		Scan(ctx)
	return a, err
}

func (r *Repo) ListByRaid(ctx context.Context, raidID int64) ([]Attendance, error) {
	var as []Attendance
	err := r.db.NewSelect().Model(&as).
		Where("raid_id = ?", raidID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return as, nil
}

// ListByCharacter returns every roster row that records a character.
func (r *Repo) ListByCharacter(ctx context.Context, characterID int64) ([]Attendance, error) {
	var rows []Attendance
	err := r.db.NewSelect().Model(&rows).
		Where("character_id = ?", characterID).
		Order("id ASC").
		Scan(ctx)
	return rows, err
}
