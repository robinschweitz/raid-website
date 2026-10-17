package enhancepoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"raid-website/internal/validation"
)

var ErrLockNotAllowed = errors.New("lock not allowed")
var ErrLockBalanceTooLow = errors.New("balance >= 150 required for lock")
var ErrLockNotFound = errors.New("lock not found")
var ErrLockNotActive = errors.New("lock not active")
var ErrLockConflict = errors.New("competing active item locks")
var ErrLockItemNameRequired = errors.New("item name required")
var ErrLockRaidNotFound = errors.New("lock raid not found")
var ErrLockRaidClosed = errors.New("raid no longer accepts locks")

// ItemsMatch uses IDs when both are present, otherwise exact trimmed names.
// Empty names never match through the fallback.
func ItemsMatch(itemID, itemName, otherItemID, otherItemName string) bool {
	itemID, otherItemID = strings.TrimSpace(itemID), strings.TrimSpace(otherItemID)
	if itemID != "" && otherItemID != "" {
		return itemID == otherItemID
	}
	itemName, otherItemName = strings.TrimSpace(itemName), strings.TrimSpace(otherItemName)
	return itemName != "" && itemName == otherItemName
}

// ResolveActiveLock returns the sole matching active lock, nil if none exists,
// or ErrLockConflict if legacy data contains multiple matching active locks.
// Callers must supply locks for a single raid; no arbitrary winner is chosen.
func ResolveActiveLock(locks []ItemLock, itemID, itemName string) (*ItemLock, error) {
	var match *ItemLock
	for i := range locks {
		if locks[i].Status != validation.LockStatusActive || !ItemsMatch(itemID, itemName, locks[i].ItemID, locks[i].ItemName) {
			continue
		}
		if match != nil {
			return nil, ErrLockConflict
		}
		match = &locks[i]
	}
	return match, nil
}

// CreateLock creates an owned lock without debiting points.
func (s *Service) CreateLock(ctx context.Context, lock *ItemLock, actorID int64) error {
	if actorID <= 0 || actorID != lock.PlayerID {
		return ErrNotOwner
	}
	lock.ItemID = strings.TrimSpace(lock.ItemID)
	lock.ItemName = strings.TrimSpace(lock.ItemName)
	lock.Boss = strings.TrimSpace(lock.Boss)
	lock.Notes = strings.TrimSpace(lock.Notes)
	if lock.ItemName == "" {
		return ErrLockItemNameRequired
	}
	return s.repo.InTransaction(ctx, func(repo *Repo) error {
		status, isArchived, err := repo.GetRaidLockState(ctx, lock.RaidID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLockRaidNotFound
		}
		if err != nil {
			return fmt.Errorf("load lock raid: %w", err)
		}
		if isArchived || status == validation.RaidStatusCompleted || status == validation.RaidStatusCancelled {
			return ErrLockRaidClosed
		}
		balance, err := repo.Balance(ctx, lock.PlayerID)
		if err != nil {
			return fmt.Errorf("load lock balance: %w", err)
		}
		if !CanLock(balance) {
			return ErrLockBalanceTooLow
		}
		existing, err := repo.FindActiveLockByItem(ctx, lock.RaidID, lock.ItemID, lock.ItemName)
		if err != nil {
			return fmt.Errorf("check competing locks: %w", err)
		}
		if existing != nil {
			return ErrLockConflict
		}
		now := time.Now().UTC()
		lock.Status = validation.LockStatusActive
		lock.ActorID = actorID
		lock.LockedAt = now
		lock.CreatedAt = now
		lock.UpdatedAt = now
		if err := repo.CreateLock(ctx, lock); err != nil {
			return fmt.Errorf("create lock: %w", err)
		}
		return repo.AuditLock(ctx, actorID, "create", nil, lock)
	})
}

// CancelLock cancels an active lock belonging to both the player and actor.
func (s *Service) CancelLock(ctx context.Context, playerID, lockID, actorID int64) (*ItemLock, error) {
	if actorID <= 0 || actorID != playerID {
		return nil, ErrNotOwner
	}
	var lock *ItemLock
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		var err error
		lock, err = repo.GetLock(ctx, lockID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLockNotFound
		}
		if err != nil {
			return fmt.Errorf("load lock: %w", err)
		}
		if lock.PlayerID != playerID {
			return ErrLockNotFound
		}
		if lock.Status != validation.LockStatusActive {
			return ErrLockNotActive
		}
		before := *lock
		now := time.Now().UTC()
		lock.Status = validation.LockStatusCancelled
		lock.CancelledAt = &now
		lock.UpdatedAt = now
		if err := repo.UpdateLock(ctx, lock); err != nil {
			return fmt.Errorf("cancel lock: %w", err)
		}
		return repo.AuditLock(ctx, actorID, "cancel", &before, lock)
	})
	if err != nil {
		return nil, err
	}
	return lock, nil
}

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
