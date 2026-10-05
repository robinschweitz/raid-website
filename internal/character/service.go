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
