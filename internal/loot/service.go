package loot

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"raid-website/internal/tokens"
)

// Service coordinates loot awards with derived token transactions.
type Service struct {
	db     *bun.DB
	awards *Repo
	tokens *tokens.Repo
}

func NewService(db *bun.DB, awards *Repo, tokenRepo *tokens.Repo) *Service {
	return &Service{db: db, awards: awards, tokens: tokenRepo}
}

// Award persists a loot award and, when the item is an armor token,
// records the derived token transaction in the same database transaction.
func (s *Service) Award(ctx context.Context, a *Award) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	awards := s.awards.WithTx(tx)
	tokenRepo := s.tokens.WithTx(tx)

	if err := awards.CreateAward(ctx, a); err != nil {
		return fmt.Errorf("create award: %w", err)
	}

	if tokens.IsTokenAward(a.ItemName) {
		raidID := a.RaidID
		awardID := a.ID
		t := &tokens.Transaction{
			PlayerID:    a.AwardedToPlayerID,
			RaidID:      &raidID,
			LootAwardID: &awardID,
			Amount:      tokens.DeriveTokenAmount(1),
			Type:        "award",
			Reason:      a.ItemName,
			RefType:     "loot_award",
			RefID:       fmt.Sprintf("%d", a.ID),
			ActorID:     a.AwardedByPlayerID,
			CreatedAt:   a.AwardedAt,
		}
		if err := tokenRepo.InsertTransaction(ctx, t); err != nil {
			return fmt.Errorf("derive token: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit award: %w", err)
	}
	return nil
}
