package wishlist

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

// Repo owns wishlist persistence.
type Repo struct {
	db bun.IDB
}

// NewRepo constructs a wishlist repository.
func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

// WithTx binds the repository to a transaction.
func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

func (r *Repo) listByCharacter(ctx context.Context, charID int64) ([]Wish, error) {
	rows := []Wish{}
	if err := r.db.NewSelect().
		Model(&rows).
		Where("character_id = ?", charID).
		Order("priority DESC, id ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) add(ctx context.Context, wish *Wish) error {
	if wish.ItemID == "" || strings.TrimSpace(wish.ItemName) == "" {
		return ErrInvalid
	}
	if wish.CharacterID <= 0 || wish.PlayerID <= 0 {
		return ErrInvalid
	}
	now := time.Now().UTC()
	if wish.CreatedAt.IsZero() {
		wish.CreatedAt = now
	}
	wish.UpdatedAt = now
	if _, err := r.db.NewInsert().Model(wish).Exec(ctx); err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (r *Repo) remove(ctx context.Context, charID int64, itemID string) error {
	if charID <= 0 || itemID == "" {
		return ErrInvalid
	}
	res, err := r.db.NewDelete().
		Model((*Wish)(nil)).
		Where("character_id = ? AND item_id = ?", charID, itemID).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RaidWishlistInterest is a character interested in an item for a given raid.
type RaidWishlistInterest struct {
	CharacterID   int64  `bun:"character_id"`
	CharacterName string `bun:"character_name"`
	PlayerID      int64  `bun:"player_id"`
	PlayerName    string `bun:"player_name"`
	ItemID        string `bun:"item_id"`
}

// BulkByItemForRaid returns interests for a single item in a raid.
func (r *Repo) BulkByItemForRaid(ctx context.Context, raidID int64, itemID string) ([]RaidWishlistInterest, error) {
	if raidID <= 0 || itemID == "" {
		return []RaidWishlistInterest{}, nil
	}
	rows := []RaidWishlistInterest{}
	err := r.db.NewSelect().
		TableExpr("character_wishlist cw").
		Column("cw.character_id", "cw.item_id").
		ColumnExpr("COALESCE(a.character_name, c.name) AS character_name").
		Column("cw.player_id").
		ColumnExpr("COALESCE(p.name, p.username) AS player_name").
		Join("JOIN characters c ON c.id = cw.character_id").
		Join("JOIN players p ON p.id = cw.player_id").
		Join("LEFT JOIN raid_attendance a ON a.raid_id = ? AND a.character_id = cw.character_id", raidID).
		Where("cw.item_id = ?", itemID).
		Order("character_name ASC").
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []RaidWishlistInterest{}, nil
		}
		return nil, err
	}
	return rows, nil
}

// BulkByItemsForRaid returns all interests for a raid (grouped by item_id on read).
func (r *Repo) BulkByItemsForRaid(ctx context.Context, raidID int64) ([]RaidWishlistInterest, error) {
	if raidID <= 0 {
		return []RaidWishlistInterest{}, nil
	}
	rows := []RaidWishlistInterest{}
	err := r.db.NewSelect().
		TableExpr("character_wishlist cw").
		Column("cw.character_id", "cw.item_id").
		ColumnExpr("COALESCE(a.character_name, c.name) AS character_name").
		Column("cw.player_id").
		ColumnExpr("COALESCE(p.name, p.username) AS player_name").
		Join("JOIN characters c ON c.id = cw.character_id").
		Join("JOIN players p ON p.id = cw.player_id").
		Join("LEFT JOIN raid_attendance a ON a.raid_id = ? AND a.character_id = cw.character_id", raidID).
		Order("cw.item_id ASC, character_name ASC").
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []RaidWishlistInterest{}, nil
		}
		return nil, err
	}
	return rows, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(strings.ToLower(s), "unique")
}
