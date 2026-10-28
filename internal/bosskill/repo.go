package bosskill

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/uptrace/bun"
)

// Repo owns the raid_boss_kills table.
type Repo struct{ db bun.IDB }

// NewRepo constructs the repository.
func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

// WithTx binds the repository to an existing transaction.
func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

// ListByRaid returns the active kills of a raid, oldest first.
func (r *Repo) ListByRaid(ctx context.Context, raidID int64) ([]Kill, error) {
	kills := []Kill{}
	err := r.db.NewSelect().Model(&kills).Where("raid_id = ?", raidID).Where("removed_at IS NULL").Order("created_at ASC", "id ASC").Scan(ctx)
	return kills, err
}

// Get loads one kill, removed or not.
func (r *Repo) Get(ctx context.Context, id int64) (*Kill, error) {
	kill := new(Kill)
	err := r.db.NewSelect().Model(kill).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return kill, err
}

// ExistsActive reports whether the raid has an active kill of the boss.
func (r *Repo) ExistsActive(ctx context.Context, raidID int64, catalogBossID string) (bool, error) {
	return r.db.NewSelect().Model((*Kill)(nil)).Where("raid_id = ?", raidID).Where("catalog_boss_id = ?", catalogBossID).Where("removed_at IS NULL").Exists(ctx)
}

// Create stores a new kill. The partial unique index is the last guard
// against a second active kill of the same boss.
func (r *Repo) Create(ctx context.Context, kill *Kill) error {
	_, err := r.db.NewInsert().Model(kill).Exec(ctx)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: raid_boss_kills.raid_id, raid_boss_kills.catalog_boss_id") {
		return ErrExists
	}
	return err
}

// Update stores a changed kill.
func (r *Repo) Update(ctx context.Context, kill *Kill) error {
	_, err := r.db.NewUpdate().Model(kill).WherePK().Exec(ctx)
	return err
}
