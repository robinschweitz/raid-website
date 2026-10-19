package character

import (
	"context"
	"time"

	"github.com/uptrace/bun"

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

func (r *Repo) correctInTransaction(ctx context.Context, fn func(*Repo, *player.Repo, *audit.Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(r.WithTx(tx), player.NewRepo(tx), audit.NewRepo(tx))
	})
}

func (r *Repo) Create(ctx context.Context, c *Character) error {
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

func (r *Repo) FindByExternalProfile(ctx context.Context, source, profileID string) (*Character, error) {
	c := new(Character)
	err := r.db.NewSelect().Model(c).Where("external_source = ? AND external_profile_id = ?", source, profileID).Order("id ASC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repo) Update(ctx context.Context, c *Character) error {
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
