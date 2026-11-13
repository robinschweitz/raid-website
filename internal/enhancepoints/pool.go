package enhancepoints

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"
)

var ErrPoolNotFound = errors.New("ep pool not found")
var ErrPoolNameInvalid = errors.New("ep pool name invalid")
var ErrPoolNameTaken = errors.New("ep pool name taken")

// ErrRaidHasNoPool means a raid without an EP pool cannot spend points
// (ENHANCED awards, item locks; ADR 113).
var ErrRaidHasNoPool = errors.New("raid has no ep pool")

// MaxAward is the highest EP award a series may set (FR-16.11).
const MaxAward = 100

// Pool is one separate EP balance (FR-16.10). Several series may share it.
type Pool struct {
	bun.BaseModel `bun:"table:ep_pools"`

	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	Name      string    `bun:"name,notnull" json:"name"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

// SavePool creates a pool (ID 0) or renames one; only an ADMIN reaches it.
func (s *Service) SavePool(ctx context.Context, actorID int64, pool *Pool) error {
	pool.Name = strings.TrimSpace(pool.Name)
	if pool.Name == "" || utf8.RuneCountInString(pool.Name) > 60 {
		return ErrPoolNameInvalid
	}
	return s.repo.InTransaction(ctx, func(repo *Repo) error {
		taken, err := repo.PoolNameTaken(ctx, pool.Name, pool.ID)
		if err != nil {
			return fmt.Errorf("check pool name: %w", err)
		}
		if taken {
			return ErrPoolNameTaken
		}
		now := time.Now().UTC()
		if pool.ID == 0 {
			pool.CreatedAt, pool.UpdatedAt = now, now
			if err := repo.InsertPool(ctx, pool); err != nil {
				return fmt.Errorf("insert pool: %w", err)
			}
			return repo.AuditPool(ctx, actorID, "create", nil, pool)
		}
		before, err := repo.GetPool(ctx, pool.ID)
		if err != nil {
			return err
		}
		pool.CreatedAt, pool.UpdatedAt = before.CreatedAt, now
		if err := repo.UpdatePool(ctx, pool); err != nil {
			return fmt.Errorf("update pool: %w", err)
		}
		return repo.AuditPool(ctx, actorID, "update", before, pool)
	})
}

// ValidAward reports whether a series EP award is allowed (FR-16.11).
func ValidAward(award int) bool {
	return award >= 0 && award <= MaxAward
}
