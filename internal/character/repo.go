package character

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/player"
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

func (r *Repo) correctInTransaction(ctx context.Context, fn func(*Repo, *player.Repo, *attendance.Repo, *audit.Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(r.WithTx(tx), player.NewRepo(tx), attendance.NewRepo(tx), audit.NewRepo(tx))
	})
}

func (r *Repo) Create(ctx context.Context, c *Character) error {
	c.ExternalProfileJSON = ""
	_, err := r.db.NewInsert().Model(c).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Character, error) {
	c := new(Character)
	err := r.db.NewSelect().Model(c).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*Character, error) {
	c := new(Character)
	err := r.db.NewSelect().Model(c).Where("name = ?", name).Order("id ASC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ListByName returns every character with exactly this name, oldest first.
func (r *Repo) ListByName(ctx context.Context, name string) ([]Character, error) {
	var rows []Character
	err := r.db.NewSelect().Model(&rows).Where("name = ?", name).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) FindByExternalProfile(ctx context.Context, source, profileID string) (*Character, error) {
	c := new(Character)
	err := r.db.NewSelect().Model(c).Where("external_source = ? AND external_profile_id = ?", source, profileID).Order("id ASC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repo) Update(ctx context.Context, c *Character) error {
	c.ExternalProfileJSON = ""
	_, err := r.db.NewUpdate().Model(c).WherePK().Exec(ctx)
	return err
}

// UpdateWishlistOwner keeps the wishlist's denormalized owner in sync with its character.
func (r *Repo) UpdateWishlistOwner(ctx context.Context, characterID, playerID int64, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().Table("character_wishlist").
		Set("player_id = ?", playerID).
		Set("updated_at = ?", updatedAt).
		Where("character_id = ?", characterID).
		Exec(ctx)
	return err
}

func (r *Repo) ListByPlayer(ctx context.Context, playerID int64) ([]Character, error) {
	var cs []Character
	err := r.db.NewSelect().Model(&cs).
		Where("player_id = ?", playerID).
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return cs, nil
}

// List returns the character directory, including inactive historical references.
func (r *Repo) List(ctx context.Context) ([]Character, error) {
	var rows []Character
	err := r.db.NewSelect().Model(&rows).Order("name ASC", "id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) InsertNameHistory(ctx context.Context, h *NameHistory) error {
	_, err := r.db.NewInsert().Model(h).Exec(ctx)
	return err
}

func (r *Repo) ListNameHistory(ctx context.Context, characterID int64) ([]NameHistory, error) {
	var hs []NameHistory
	err := r.db.NewSelect().Model(&hs).
		Where("character_id = ?", characterID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return hs, nil
}

func (r *Repo) runInTx(ctx context.Context, fn func(context.Context, bun.Tx) error) error {
	return r.db.RunInTx(ctx, nil, fn)
}

// AcquireWriteLock takes SQLite's writer lock at the start of a transaction by
// running a write that changes no row.
func (r *Repo) AcquireWriteLock(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "UPDATE players SET id = id WHERE id = -1")
	return err
}

// CountMergeConflicts counts the raids and wishlist items on which both
// characters have a record.
func (r *Repo) CountMergeConflicts(ctx context.Context, sourceID, targetID int64) (int, error) {
	queries := []string{
		"SELECT COUNT(*) FROM raid_attendance s JOIN raid_attendance t ON s.raid_id=t.raid_id WHERE s.character_id=? AND t.character_id=?",
		"SELECT COUNT(*) FROM raid_signups s JOIN raid_signups t ON s.raid_id=t.raid_id WHERE s.character_id=? AND t.character_id=?",
		"SELECT COUNT(*) FROM character_wishlist s JOIN character_wishlist t ON s.item_id=t.item_id WHERE s.character_id=? AND t.character_id=?",
	}
	total := 0
	for _, query := range queries {
		var n int
		if err := r.db.QueryRowContext(ctx, query, sourceID, targetID).Scan(&n); err != nil {
			return 0, fmt.Errorf("check merge conflicts: %w", err)
		}
		total += n
	}
	return total, nil
}

// MoveReferences points every record of the source character at the target and
// refreshes the owner and name snapshots stored beside them.
func (r *Repo) MoveReferences(ctx context.Context, sourceID int64, target *Character, now time.Time) error {
	statements := []struct {
		query string
		args  []any
	}{
		{"UPDATE character_name_history SET character_id=? WHERE character_id=?", []any{target.ID, sourceID}},
		{"UPDATE raid_attendance SET player_id=?, character_id=?, character_name=?, updated_at=? WHERE character_id=?", []any{target.PlayerID, target.ID, target.Name, now, sourceID}},
		{"UPDATE loot_awards SET awarded_to_player_id=?, awarded_to_character_id=?, awarded_to_character_name=?, updated_at=? WHERE awarded_to_character_id=?", []any{target.PlayerID, target.ID, target.Name, now, sourceID}},
		{"UPDATE raid_signups SET player_id=?, character_id=?, updated_at=? WHERE character_id=?", []any{target.PlayerID, target.ID, now, sourceID}},
		{"UPDATE character_wishlist SET character_id=?, player_id=?, updated_at=? WHERE character_id=?", []any{target.ID, target.PlayerID, now, sourceID}},
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("move character references: %w", err)
		}
	}
	return nil
}

// InsertMergeAlias stores the merged character's old name.
func (r *Repo) InsertMergeAlias(ctx context.Context, sourceName string, targetID, sourceID int64, createdAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, "INSERT INTO character_merge_aliases (source_name, target_character_id, source_character_id, created_at) VALUES (?, ?, ?, ?)", sourceName, targetID, sourceID, createdAt); err != nil {
		return fmt.Errorf("store character alias: %w", err)
	}
	return nil
}

// Delete removes a character row.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM characters WHERE id=?", id); err != nil {
		return fmt.Errorf("delete source character: %w", err)
	}
	return nil
}
