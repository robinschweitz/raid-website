package enhancepoints

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

var ErrInsufficientBalance = errors.New("insufficient balance")
var ErrInvalidCost = errors.New("invalid cost")
var ErrNotOwner = errors.New("self-service requires ownership")

// Service handles self-service enhance points operations.
type Service struct {
	repo *Repo
}

// NewService constructs an enhance points service.
func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

// RollResult describes a committed enhance roll.
type RollResult struct {
	PlayerID     int64 `json:"player_id"`
	Cost         int   `json:"cost"`
	Roll         int   `json:"roll"`
	Min          int   `json:"min"`
	Max          int   `json:"max"`
	BalanceAfter int   `json:"balance_after"`
}

// Roll checks ownership and atomically reads the balance, rolls and debits cost.
func (s *Service) Roll(ctx context.Context, playerID, actorID int64) (RollResult, error) {
	if actorID <= 0 || actorID != playerID {
		return RollResult{}, ErrNotOwner
	}
	var result RollResult
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		balance, err := repo.Balance(ctx, playerID)
		if err != nil {
			return fmt.Errorf("load roll balance: %w", err)
		}
		cost, err := RollCost(balance)
		if err != nil {
			return err
		}
		minRoll, maxRoll := RollRange(cost)
		result = RollResult{PlayerID: playerID, Cost: cost, Roll: minRoll + rand.IntN(maxRoll-minRoll+1), Min: minRoll, Max: maxRoll, BalanceAfter: balance - cost}
		if err := repo.InsertTransaction(ctx, &Transaction{
			PlayerID: playerID, Amount: -cost, Type: "enhance_roll",
			Reason: "enhance roll cost", ActorID: actorID, CreatedAt: time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("record roll cost: %w", err)
		}
		return nil
	})
	if err != nil {
		return RollResult{}, err
	}
	return result, nil
}

// ComputeBalance sums transactions for playerID
func ComputeBalance(transactions []Transaction) int {
	b := 0
	for _, t := range transactions {
		b += t.Amount
	}
	return b
}

// RollCost returns X = min(balance, 50). Cannot split; must be >=1 if balance>=1
func RollCost(balance int) (int, error) {
	if balance <= 0 {
		return 0, ErrInsufficientBalance
	}
	x := balance
	if x > 50 {
		x = 50
	}
	if x < 1 {
		return 0, ErrInvalidCost
	}
	return x, nil
}

// RollRange returns 1-100 + X (conceptually)
func RollRange(x int) (min int, max int) {
	return 1, 100 + x
}
