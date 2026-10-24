package accountmerge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

var ErrSourceRequired = errors.New("source player is required")
var ErrSamePlayer = errors.New("source and target must differ")
var ErrSourceInactive = errors.New("source player is already inactive")
var ErrLockConflict = errors.New("source and target have competing active item locks")

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

// Merge deletes source after moving every reference to target.
func (s *Service) Merge(ctx context.Context, targetID, sourceID, actorID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if sourceID <= 0 {
		return ErrSourceRequired
	}
	if sourceID == targetID {
		return ErrSamePlayer
	}
	if actorID <= 0 || reason == "" {
		return errors.New("actor and reason are required")
	}

	return s.repo.InTransaction(ctx, func(repo *Repo) error {
		target, err := repo.GetPlayer(ctx, targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("target: %w", err)
		}
		if err != nil {
			return fmt.Errorf("load target player: %w", err)
		}
		source, err := repo.GetPlayer(ctx, sourceID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("source: %w", err)
		}
		if err != nil {
			return fmt.Errorf("load source player: %w", err)
		}
		if !source.Active {
			return ErrSourceInactive
		}

		// The target keeps its own role, so only an Admin source that no active
		// Admin target replaces can remove the last Admin (SC-047).
		if source.Role == validation.RoleAdmin && !(target.Role == validation.RoleAdmin && target.Active) {
			admins, err := repo.CountActiveAdmins(ctx)
			if err != nil {
				return fmt.Errorf("count active admins: %w", err)
			}
			if admins <= 1 {
				return player.ErrLastAdmin
			}
		}

		sourceLocks, err := repo.ListLocks(ctx, sourceID)
		if err != nil {
			return fmt.Errorf("list source locks: %w", err)
		}
		targetLocks, err := repo.ListActiveLocks(ctx, targetID)
		if err != nil {
			return fmt.Errorf("list target locks: %w", err)
		}
		for _, sourceLock := range sourceLocks {
			if sourceLock.Status != validation.LockStatusActive {
				continue
			}
			for _, targetLock := range targetLocks {
				if sourceLock.RaidID == targetLock.RaidID && enhancepoints.ItemsMatch(sourceLock.ItemID, sourceLock.ItemName, targetLock.ItemID, targetLock.ItemName) {
					return ErrLockConflict
				}
			}
		}

		now := time.Now().UTC()
		characters, err := repo.ListCharacters(ctx, sourceID)
		if err != nil {
			return fmt.Errorf("list source characters: %w", err)
		}
		for _, row := range characters {
			before, after := row, row
			after.PlayerID = targetID
			after.UpdatedAt = now
			if err := repo.UpdateCharacterOwner(ctx, row.ID, targetID, now); err != nil {
				return fmt.Errorf("transfer character %d: %w", row.ID, err)
			}
			if err := repo.Audit(ctx, actorID, "character", row.ID, "account_merge_transfer", before, after, reason, now); err != nil {
				return err
			}
		}
		for _, row := range sourceLocks {
			before, after := row, row
			after.PlayerID = targetID
			after.UpdatedAt = now
			if err := repo.UpdateLockOwner(ctx, row.ID, targetID, now); err != nil {
				return fmt.Errorf("transfer lock %d: %w", row.ID, err)
			}
			if err := repo.Audit(ctx, actorID, "item_lock", row.ID, "account_merge_transfer", before, after, reason, now); err != nil {
				return err
			}
		}

		if err := repo.MoveHistoricalReferences(ctx, sourceID, targetID); err != nil {
			return fmt.Errorf("transfer historical references: %w", err)
		}

		revoked, err := repo.RevokeSourceAuth(ctx, sourceID)
		if err != nil {
			return fmt.Errorf("revoke source authentication: %w", err)
		}
		if err := repo.Audit(ctx, actorID, "player", sourceID, "account_merge_revoke_auth", nil, revoked, reason, now); err != nil {
			return err
		}

		if err := repo.Audit(ctx, actorID, "player", sourceID, "account_merge_delete", *source, map[string]any{"id": sourceID, "deleted": true, "merged_into": targetID}, reason, now); err != nil {
			return err
		}
		if err := repo.Audit(ctx, actorID, "player", targetID, "account_merge_target", *target, *target, reason, now); err != nil {
			return err
		}
		if err := repo.DeletePlayer(ctx, sourceID); err != nil {
			return fmt.Errorf("delete source player: %w", err)
		}
		return nil
	})
}

func auditJSON(v any) (string, error) {
	if v == nil {
		return "null", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
