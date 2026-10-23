package recruitment

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

type row struct {
	bun.BaseModel `bun:"table:raid_recruitment_templates"`
	ID            int64 `bun:"id,pk,autoincrement"`
	SeriesID      *int64
	RaidID        *int64
	Text          string
	SetupJSON     string
	UpdatedBy     int64
	UpdatedAt     time.Time
}

// Repo owns the raid_recruitment_templates table.
type Repo struct {
	db bun.IDB
}

// NewRepo constructs the repository.
func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) runInTx(ctx context.Context, fn func(context.Context, bun.Tx) error) error {
	return r.db.RunInTx(ctx, nil, fn)
}

// find returns the template stored for series_id or raid_id `col`, or nil.
func (r *Repo) find(ctx context.Context, col string, id int64) (*row, error) {
	found := new(row)
	err := r.db.NewSelect().Model(found).Where(col+" = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return found, nil
}

// upsert stores the template for series_id or raid_id `col`.
func (r *Repo) upsert(ctx context.Context, col string, id int64, text, setupJSON string, updatedBy int64, updatedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO raid_recruitment_templates ("+col+", text, setup_json, updated_by, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT("+col+") WHERE "+col+" IS NOT NULL DO UPDATE SET text = excluded.text, setup_json = excluded.setup_json, updated_by = excluded.updated_by, updated_at = excluded.updated_at", id, text, setupJSON, updatedBy, updatedAt)
	return err
}
