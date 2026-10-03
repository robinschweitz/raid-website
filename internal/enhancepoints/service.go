package enhancepoints

import (
	"errors"
)

var ErrInsufficientBalance = errors.New("insufficient balance")
var ErrInvalidCost = errors.New("invalid cost")

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
