package registration

import (
	"context"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/validation"
)

const resetCodeTTL = 72 * time.Hour

// ResetCode stores only the digest of a one-time password reset code.
type ResetCode struct {
	bun.BaseModel     `bun:"table:password_reset_codes"`
	ID                int64      `bun:"id,pk,autoincrement" json:"id"`
	PlayerID          int64      `json:"player_id"`
	CodeHash          string     `json:"-"`
	CreatedByPlayerID *int64     `json:"created_by_player_id"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	UsedAt            *time.Time `json:"used_at"`
	RevokedAt         *time.Time `json:"revoked_at"`
}

// GeneratedResetCode discloses the code once; subsequent reads expose metadata only.
type GeneratedResetCode struct {
	Record ResetCode `json:"record"`
	Code   string    `json:"code"`
}

// CreateResetCode replaces the player's unused codes in one audited transaction.
func (s *Service) CreateResetCode(ctx context.Context, actorID, playerID int64) (*GeneratedResetCode, error) {
	return s.createResetCode(ctx, actorID, playerID, false)
}

func (s *Service) createResetCode(ctx context.Context, actorID, playerID int64, operator bool) (*GeneratedResetCode, error) {
	plain, hash, err := newCode()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := &ResetCode{PlayerID: playerID, CodeHash: hash, CreatedAt: now, ExpiresAt: now.Add(resetCodeTTL)}
	if !operator {
		row.CreatedByPlayerID = &actorID
	}
	err = s.repo.inResetTransaction(ctx, func(repo *Repo) error {
		if !operator {
			actor, err := repo.resetPlayer(ctx, actorID)
			if err != nil {
				return err
			}
			if actor.Role != validation.RoleAdmin || !actor.Active {
				return ErrForbidden
			}
		}
		if _, err := repo.resetPlayer(ctx, playerID); err != nil {
			return err
		}
		if err := repo.revokeResetCodes(ctx, playerID, now); err != nil {
			return err
		}
		if err := repo.createResetCode(ctx, row); err != nil {
			return err
		}
		source := "admin"
		if operator {
			source = "server operator"
		}
		return repo.auditReset(ctx, actorID, playerID, "reset_code_created", map[string]any{"player_id": playerID, "expires_at": row.ExpiresAt, "created_by": source}, source)
	})
	if err != nil {
		return nil, err
	}
	return &GeneratedResetCode{Record: *row, Code: plain}, nil
}
