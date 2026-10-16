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

// Spec says which roll a wish is for: main spec (/rnd 100) or second spec (/rnd 99).
type Spec string

// Wish specs.
const (
	SpecMain   Spec = "MAIN"
	SpecSecond Spec = "SECOND"
)

// Valid reports whether the spec is known.
func (s Spec) Valid() bool {
	return s == SpecMain || s == SpecSecond
}

// Wish is a single item on a character's wishlist. IsEnhanced asks the
// in-game addon to roll enhanced (1+X..100+X, X = min(EP, 50), ADR 023);
// only main-spec wishes can be enhanced.
type Wish struct {
	bun.BaseModel `bun:"table:character_wishlist"`
	ID            int64     `bun:"id,pk,autoincrement" json:"id"`
	PlayerID      int64     `bun:"player_id" json:"player_id"`
	CharacterID   int64     `bun:"character_id" json:"character_id"`
	ItemID        string    `bun:"item_id" json:"item_id"`
	ItemName      string    `bun:"item_name" json:"item_name"`
	Priority      int       `bun:"priority" json:"priority"`
	Notes         string    `bun:"notes" json:"notes"`
	Spec          Spec      `bun:"spec" json:"spec"`
	IsEnhanced    bool      `bun:"is_enhanced" json:"is_enhanced"`
	CreatedAt     time.Time `bun:"created_at" json:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at" json:"updated_at"`
}

// RollChoice is how the owner wants to roll on a wish.
type RollChoice struct {
	Spec       Spec
	IsEnhanced bool
}

// normalize defaults an empty spec to main spec and rejects enhanced
// second-spec rolls, which the raid rules do not have.
func (c RollChoice) normalize() (RollChoice, error) {
	if c.Spec == "" {
		c.Spec = SpecMain
	}
	if !c.Spec.Valid() || (c.IsEnhanced && c.Spec != SpecMain) {
		return c, ErrInvalid
	}
	return c, nil
}
