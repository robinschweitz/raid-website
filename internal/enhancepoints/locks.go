package enhancepoints

import (
	"errors"
	"time"

	"raid-website/internal/validation"
)

var ErrLockNotAllowed = errors.New("lock not allowed")
var ErrLockBalanceTooLow = errors.New("balance >= 150 required for lock")

// CanLock returns true if balance >= 150
func CanLock(balance int) bool {
	return balance >= 150
}

// ExpireActiveOnEnd marks locks expired at end/completion (no cost)
func ExpireActiveOnEnd(activeLocks []ItemLock) []ItemLock {
	exp := make([]ItemLock, len(activeLocks))
	now := time.Now().UTC()
	for i, l := range activeLocks {
		exp[i] = l
		if exp[i].Status == validation.LockStatusActive {
			exp[i].Status = validation.LockStatusExpired
			t := now
			exp[i].ExpiredAt = &t
		}
	}
	return exp
}
