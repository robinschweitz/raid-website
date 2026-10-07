package wishlist

import (
	"errors"
	"time"

	"github.com/uptrace/bun"
)

var (
	// ErrNotOwner indicates the caller does not own the character.
	ErrNotOwner = errors.New("wishlist: not owner")
	// ErrNotFound indicates the requested wish does not exist.
	ErrNotFound = errors.New("wishlist: not found")
	// ErrConflict indicates a unique constraint violation.
	ErrConflict = errors.New("wishlist: conflict")
	// ErrCharacterNotFound indicates the character does not exist.
	ErrCharacterNotFound = errors.New("wishlist: character not found")
	// ErrInvalid indicates invalid input.
	ErrInvalid = errors.New("wishlist: invalid")
)

// Wish is a single item on a character's wishlist.
type Wish struct {
	bun.BaseModel `bun:"table:character_wishlist"`
	ID            int64     `bun:"id,pk,autoincrement"`
	PlayerID      int64     `bun:"player_id"`
	CharacterID   int64     `bun:"character_id"`
	ItemID        string    `bun:"item_id"`
	ItemName      string    `bun:"item_name"`
	Priority      int       `bun:"priority"`
	Notes         string    `bun:"notes"`
	CreatedAt     time.Time `bun:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at"`
}
