package enhancepoints

import "errors"

var ErrInsufficientBalance = errors.New("insufficient balance")
var ErrNotOwner = errors.New("self-service requires ownership")

// Service handles self-service item locks, never generates or spends on rolls.
type Service struct{ repo *Repo }

// NewService constructs an enhance points service.
func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// ComputeBalance sums append-only transactions.
func ComputeBalance(transactions []Transaction) int {
	balance := 0
	for _, transaction := range transactions {
		balance += transaction.Amount
	}
	return balance
}

// RollCost explains the automatic cost of an enhanced winner, not a spend action.
func RollCost(balance int) (int, error) {
	if balance <= 0 {
		return 0, ErrInsufficientBalance
	}
	if balance > 50 {
		return 50, nil
	}
	return balance, nil
}

// RollRange explains the shifted outside roll range; negative values have no effect.
func RollRange(x int) (min int, max int) {
	if x < 0 {
		x = 0
	}
	return 1 + x, 100 + x
}
