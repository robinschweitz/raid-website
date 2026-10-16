package wishlist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
)

// Service provides wishlist operations.
type Service struct {
	repo       *Repo
	characters *character.Repo
	points     *enhancepoints.Repo
}

// NewService constructs a wishlist service.
func NewService(db bun.IDB) *Service {
	return &Service{
		repo:       NewRepo(db),
		characters: character.NewRepo(db),
		points:     enhancepoints.NewRepo(db),
	}
}

// WithTx binds the service to a transaction.
func (s *Service) WithTx(tx bun.Tx) *Service {
	return &Service{
		repo:       s.repo.WithTx(tx),
		characters: character.NewRepo(tx),
		points:     enhancepoints.NewRepo(tx),
	}
}

func (s *Service) charOwner(ctx context.Context, charID int64) (playerID int64, err error) {
	ch, err := s.characters.Get(ctx, charID)
	if err != nil {
		if errors.Is(err, character.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return 0, ErrCharacterNotFound
		}
		return 0, err
	}
	if ch == nil {
		return 0, ErrCharacterNotFound
	}
	return ch.PlayerID, nil
}

// Owner returns the player who owns the character.
func (s *Service) Owner(ctx context.Context, charID int64) (int64, error) {
	if charID <= 0 {
		return 0, ErrInvalid
	}
	return s.charOwner(ctx, charID)
}

// ListByCharacter returns the wishlist for a character.
func (s *Service) ListByCharacter(ctx context.Context, charID int64) ([]Wish, error) {
	if charID <= 0 {
		return nil, ErrInvalid
	}
	return s.repo.listByCharacter(ctx, charID)
}

// Add adds an item to a character's wishlist (actor must own the character).
func (s *Service) Add(ctx context.Context, actorID, charID int64, itemID, itemName string, priority int, notes string, choice RollChoice) (*Wish, error) {
	if charID <= 0 || itemID == "" || itemName == "" {
		return nil, ErrInvalid
	}
	choice, err := choice.normalize()
	if err != nil {
		return nil, err
	}
	owner, err := s.charOwner(ctx, charID)
	if err != nil {
		return nil, err
	}
	if owner != actorID {
		return nil, ErrNotOwner
	}
	w := &Wish{
		PlayerID:    owner,
		CharacterID: charID,
		ItemID:      itemID,
		ItemName:    itemName,
		Priority:    priority,
		Notes:       notes,
		Spec:        choice.Spec,
		IsEnhanced:  choice.IsEnhanced,
	}
	if err := s.repo.add(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// Update changes how the owner rolls on a wish and its priority and notes
// (actor must own the character).
func (s *Service) Update(ctx context.Context, actorID, charID int64, itemID string, priority int, notes string, choice RollChoice) (*Wish, error) {
	if charID <= 0 || itemID == "" {
		return nil, ErrInvalid
	}
	choice, err := choice.normalize()
	if err != nil {
		return nil, err
	}
	owner, err := s.charOwner(ctx, charID)
	if err != nil {
		return nil, err
	}
	if owner != actorID {
		return nil, ErrNotOwner
	}
	return s.repo.update(ctx, charID, itemID, priority, notes, choice)
}

// AddonExport renders the character's wishlist for the RaidWishlist
// addon, with the owner's EP balance for enhanced rolls.
func (s *Service) AddonExport(ctx context.Context, charID int64, now time.Time) (string, error) {
	if charID <= 0 {
		return "", ErrInvalid
	}
	ch, err := s.characters.Get(ctx, charID)
	if err != nil {
		if errors.Is(err, character.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return "", ErrCharacterNotFound
		}
		return "", fmt.Errorf("load character %d: %w", charID, err)
	}
	wishes, err := s.repo.listByCharacter(ctx, charID)
	if err != nil {
		return "", fmt.Errorf("list wishlist of character %d: %w", charID, err)
	}
	balance, err := s.points.Balance(ctx, ch.PlayerID)
	if err != nil {
		return "", fmt.Errorf("load EP balance of player %d: %w", ch.PlayerID, err)
	}
	return BuildAddonExport(ch.Name, now, balance, wishes), nil
}

// Remove removes an item from a character's wishlist (actor must own the character).
func (s *Service) Remove(ctx context.Context, actorID, charID int64, itemID string) error {
	if charID <= 0 || itemID == "" {
		return ErrInvalid
	}
	owner, err := s.charOwner(ctx, charID)
	if err != nil {
		return err
	}
	if owner != actorID {
		return ErrNotOwner
	}
	return s.repo.remove(ctx, charID, itemID)
}

// BulkByItemForRaid returns interests for a single item in a raid.
func (s *Service) BulkByItemForRaid(ctx context.Context, raidID int64, itemID string) ([]RaidWishlistInterest, error) {
	return s.repo.BulkByItemForRaid(ctx, raidID, itemID)
}

// BulkByItemsForRaid returns all interests for a raid (grouped by item_id on read).
func (s *Service) BulkByItemsForRaid(ctx context.Context, raidID int64) ([]RaidWishlistInterest, error) {
	return s.repo.BulkByItemsForRaid(ctx, raidID)
}
