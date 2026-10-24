package unclaimedloot

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
)

// Repo owns the unclaimed_loot table.
type Repo struct{ db bun.IDB }

// NewRepo constructs the repository.
func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

// ListByRaid returns a raid's entries that were not removed, oldest first.
func (r *Repo) ListByRaid(ctx context.Context, raidID int64) ([]Entry, error) {
	entries := []Entry{}
	err := r.db.NewSelect().Model(&entries).Where("raid_id = ?", raidID).Where("archived_at IS NULL").Order("created_at ASC", "id ASC").Scan(ctx)
	return entries, err
}

// AddonLineKeys maps the addon lines already recorded for a raid to their entry.
func (r *Repo) AddonLineKeys(ctx context.Context, raidID int64) (map[string]int64, error) {
	var entries []Entry
	if err := r.db.NewSelect().Model(&entries).Where("raid_id = ?", raidID).Where("addon_line_key IS NOT NULL").Scan(ctx); err != nil {
		return nil, err
	}
	keys := make(map[string]int64, len(entries))
	for _, entry := range entries {
		keys[*entry.AddonLineKey] = entry.ID
	}
	return keys, nil
}

func (r *Repo) get(ctx context.Context, id int64) (*Entry, error) {
	entry := new(Entry)
	err := r.db.NewSelect().Model(entry).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return entry, err
}

// insert stores a new entry and its audit entry in one transaction.
func (r *Repo) insert(ctx context.Context, entry *Entry, actorID int64, reason string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(entry).Exec(ctx); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, "unclaimed_loot", entry.ID, "create", nil, entry, reason)
	})
}

// archive stores a removal and its audit entry in one transaction.
func (r *Repo) archive(ctx context.Context, before, entry *Entry, actorID int64, reason string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model(entry).WherePK().Exec(ctx); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, "unclaimed_loot", entry.ID, "archive", before, entry, reason)
	})
}
