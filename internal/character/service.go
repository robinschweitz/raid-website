package character

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

var (
	ErrNotFound       = errors.New("character not found")
	ErrInvalidActor   = errors.New("actor does not exist")
	ErrForbidden      = errors.New("character correction requires admin")
	ErrReasonRequired = errors.New("reason is required")
	ErrInvalidName    = errors.New("name cannot be empty")
	ErrInvalidClass   = errors.New("unknown class")
	ErrPlayerNotFound = errors.New("target player not found")
	ErrSameOwner      = errors.New("character already belongs to this player")
)

// Patch contains only fields that a character correction may change.
type Patch struct {
	Name       *string `json:"name"`
	Class      *string `json:"class"`
	MainSpec   *string `json:"main_spec"`
	SecondSpec *string `json:"second_spec"`
	IsMain     *bool   `json:"is_main"`
	Active     *bool   `json:"active"`
	Reason     string  `json:"reason"`
}

// Service applies audited character corrections.
type Service struct {
	repo *Repo
}

type ownerSnapshot struct {
	*Character
	OwnerName string `json:"owner_name"`
}

// NewService constructs a correction service using the existing repository.
func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

// Correct applies a partial update and records its history atomically.
func (s *Service) Correct(ctx context.Context, actorID, characterID int64, patch Patch) (*Character, error) {
	patch.Reason = strings.TrimSpace(patch.Reason)
	if patch.Name != nil && *patch.Name == "" {
		return nil, ErrInvalidName
	}
	if patch.Class != nil {
		class := validation.NormalizeClassName(*patch.Class)
		if _, ok := validation.ClassSpecs[class]; !ok {
			return nil, ErrInvalidClass
		}
		patch.Class = &class
	}
	var corrected *Character
	err := s.repo.correctInTransaction(ctx, func(chars *Repo, players *player.Repo, audits *audit.Repo) error {
		actor, err := players.Get(ctx, actorID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActor
		}
		if err != nil {
			return fmt.Errorf("load correction actor: %w", err)
		}
		if !actor.Role.AtLeast(validation.RoleAdmin) {
			return ErrForbidden
		}
		c, err := chars.Get(ctx, characterID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load character for correction: %w", err)
		}
		before, err := json.Marshal(c)
		if err != nil {
			return fmt.Errorf("encode character before correction: %w", err)
		}
		oldName := c.Name
		if patch.Name != nil {
			c.Name = *patch.Name
		}
		if patch.Class != nil {
			c.Class = *patch.Class
		}
		if patch.MainSpec != nil {
			c.MainSpec = *patch.MainSpec
		}
		if patch.SecondSpec != nil {
			c.SecondSpec = *patch.SecondSpec
		}
		if patch.IsMain != nil {
			c.IsMain = *patch.IsMain
		}
		if patch.Active != nil {
			c.Active = *patch.Active
		}
		now := time.Now().UTC()
		c.UpdatedAt = now
		action := "update"
		if c.Name != oldName {
			c.NameChangedAt = &now
			action = "rename"
		}
		after, err := json.Marshal(c)
		if err != nil {
			return fmt.Errorf("encode character after correction: %w", err)
		}
		if err := chars.Update(ctx, c); err != nil {
			return fmt.Errorf("update corrected character: %w", err)
		}
		if c.Name != oldName {
			if err := chars.InsertNameHistory(ctx, &NameHistory{CharacterID: c.ID, OldName: oldName, NewName: c.Name, ActorID: actorID, Reason: patch.Reason, CreatedAt: now}); err != nil {
				return fmt.Errorf("record character name history: %w", err)
			}
		}
		if err := audits.Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "character", EntityID: strconv.FormatInt(c.ID, 10), Action: action, BeforeJSON: string(before), AfterJSON: string(after), Reason: patch.Reason, CreatedAt: now}); err != nil {
			return fmt.Errorf("audit character correction: %w", err)
		}
		corrected = c
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("correct character %d: %w", characterID, err)
	}
	return corrected, nil
}

// Transfer moves a live character to another player. Raid records stay as the
// historical snapshots they were when recorded; wishlist ownership follows it.
func (s *Service) Transfer(ctx context.Context, actorID, characterID, targetPlayerID int64) (*Character, error) {
	if targetPlayerID <= 0 {
		return nil, ErrPlayerNotFound
	}
	var transferred *Character
	err := s.repo.correctInTransaction(ctx, func(chars *Repo, players *player.Repo, audits *audit.Repo) error {
		actor, err := players.Get(ctx, actorID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActor
		}
		if err != nil {
			return fmt.Errorf("load transfer actor: %w", err)
		}
		if !actor.Role.AtLeast(validation.RoleAdmin) {
			return ErrForbidden
		}
		c, err := chars.Get(ctx, characterID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load character for transfer: %w", err)
		}
		if c.PlayerID == targetPlayerID {
			return ErrSameOwner
		}
		source, err := players.Get(ctx, c.PlayerID)
		if err != nil {
			return fmt.Errorf("load current character owner: %w", err)
		}
		target, err := players.Get(ctx, targetPlayerID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPlayerNotFound
		}
		if err != nil {
			return fmt.Errorf("load target character owner: %w", err)
		}

		now := time.Now().UTC()
		before := ownerSnapshot{Character: c, OwnerName: source.Name}
		updated := *c
		updated.PlayerID = target.ID
		updated.UpdatedAt = now
		after := ownerSnapshot{Character: &updated, OwnerName: target.Name}
		if err := chars.Update(ctx, &updated); err != nil {
			return fmt.Errorf("update character owner: %w", err)
		}
		if err := chars.UpdateWishlistOwner(ctx, updated.ID, target.ID, now); err != nil {
			return fmt.Errorf("update wishlist owner: %w", err)
		}
		if err := audits.Record(ctx, actorID, "character", updated.ID, "transfer_owner", before, after, ""); err != nil {
			return fmt.Errorf("audit character transfer: %w", err)
		}
		transferred = &updated
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("transfer character %d: %w", characterID, err)
	}
	return transferred, nil
}
