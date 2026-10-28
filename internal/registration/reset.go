package registration

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"raid-website/internal/auth"
	"raid-website/internal/player"
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

// CreateOperatorResetCode is used only by the host command; no HTTP route calls it.
func (s *Service) CreateOperatorResetCode(ctx context.Context, playerID int64, name string) (*GeneratedResetCode, error) {
	if playerID <= 0 {
		p, err := s.repo.operatorResetPlayer(ctx, name)
		if err != nil {
			return nil, err
		}
		playerID = p.ID
	}
	return s.createResetCode(ctx, 0, playerID, true)
}

// ErrResetCodeInvalid covers unknown, expired, revoked and consumed codes alike.
var ErrResetCodeInvalid = errors.New("reset code invalid or expired")

// RedeemResetCode changes the password and revokes every factor/session in one
// transaction; a failed deletion leaves both the credentials and code unchanged.
func (s *Service) RedeemResetCode(ctx context.Context, code, password, address string) error {
	return s.withCodeLimit(ctx, address, func() error {
		if err := auth.ValidatePassword(password); err != nil {
			return err
		}
		now := time.Now().UTC()
		row, err := s.repo.validResetCode(ctx, hashCode(code), now)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrResetCodeInvalid
		}
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(ctx, password)
		if err != nil {
			return err
		}
		return s.repo.inResetTransaction(ctx, func(repo *Repo) error {
			current, err := repo.validResetCode(ctx, hashCode(code), time.Now().UTC())
			if errors.Is(err, sql.ErrNoRows) {
				return ErrResetCodeInvalid
			}
			if err != nil {
				return err
			}
			if current.ID != row.ID {
				return ErrResetCodeInvalid
			}
			removed, err := repo.applyPasswordReset(ctx, row.PlayerID, hash, now)
			if err != nil {
				return err
			}
			if err := repo.consumeResetCode(ctx, row.ID, now); err != nil {
				return err
			}
			if err := repo.revokeResetCodes(ctx, row.PlayerID, now); err != nil {
				return err
			}
			return repo.auditReset(ctx, row.PlayerID, row.PlayerID, "password_reset", map[string]any{"player_id": row.PlayerID, "removed": removed}, "")
		})
	})
}

func (s *Service) withCodeLimit(ctx context.Context, address string, fn func() error) error {
	now := time.Now().UTC()
	id, err := s.repo.reserveCodeAttempt(ctx, address, now)
	if err != nil {
		return err
	}
	err = fn()
	if !errors.Is(err, ErrInvalidCode) && !errors.Is(err, ErrResetCodeInvalid) {
		if cleanupErr := s.repo.releaseCodeAttempt(ctx, id); cleanupErr != nil {
			log.Printf("release code attempt: %v", cleanupErr)
		}
	}
	return err
}

// RedeemNewAccountFromAddress shares the failed-code budget with password reset.
func (s *Service) RedeemNewAccountFromAddress(ctx context.Context, code, username, password, address string) (string, *player.Player, error) {
	var token string
	var p *player.Player
	err := s.withCodeLimit(ctx, address, func() error {
		var err error
		token, p, err = s.RedeemNewAccount(ctx, code, username, password)
		return err
	})
	return token, p, err
}
