package registration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

// ClaimResult is the outcome of ClaimCharacter: either the character now
// belongs to the actor, or a request waits for an Admin's code (ADR 116).
type ClaimResult struct {
	IsClaimed bool                 `json:"claimed"`
	Character *character.Character `json:"character,omitempty"`
	Request   *Request             `json:"request,omitempty"`
}

// ClaimCharacter gives a MEMBER or higher a free Rising-Gods character at once
// and falls back to a claim request otherwise (FR-4.6, ADR 116). A character is
// free when no character holds the profile or the name, or the one that does
// belongs to the actor or to a player without a login.
func (s *Service) ClaimCharacter(ctx context.Context, actorID int64, profileID string) (*ClaimResult, error) {
	if existing, err := s.repo.FindBlockingRequest(ctx, ExternalSourceRisingGods, profileID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrRequestExists
	}
	actor, err := s.players.Get(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("load claiming player %d: %w", actorID, err)
	}
	if !actor.Role.AtLeast(validation.RoleMember) {
		return s.claimByRequest(ctx, actorID, profileID)
	}
	profile, _, err := s.loadUsableProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	var claimed *character.Character
	needsAdmin := false
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Checked again inside the write transaction so a request created
		// meanwhile is not bypassed.
		if existing, err := NewRepo(tx).FindBlockingRequest(ctx, ExternalSourceRisingGods, profile.ProfileID); err != nil {
			return err
		} else if existing != nil {
			return ErrRequestExists
		}
		target, isFree, err := freeClaimTarget(ctx, s.players.WithTx(tx), s.characters.WithTx(tx), actorID, profile)
		if err != nil {
			return err
		}
		if !isFree {
			needsAdmin = true
			return nil
		}
		claimed, err = s.applyFreeClaim(ctx, tx, actorID, target, profile)
		return err
	})
	if err != nil {
		return nil, err
	}
	if needsAdmin {
		return s.claimByRequest(ctx, actorID, profileID)
	}
	return &ClaimResult{IsClaimed: true, Character: claimed}, nil
}

func (s *Service) claimByRequest(ctx context.Context, actorID int64, profileID string) (*ClaimResult, error) {
	req, err := s.CreateRequest(ctx, &actorID, profileID)
	if err != nil {
		return nil, err
	}
	return &ClaimResult{IsClaimed: false, Request: req}, nil
}

// freeClaimTarget returns the existing character the claim takes over (nil for
// a new character) and whether the claim may happen without an Admin.
func freeClaimTarget(ctx context.Context, players *player.Repo, chars *character.Repo, actorID int64, profile ExternalCharacter) (*character.Character, bool, error) {
	if linked, err := chars.FindByExternalProfile(ctx, ExternalSourceRisingGods, profile.ProfileID); err == nil {
		isFree, err := ownerAllowsClaim(ctx, players, actorID, linked.PlayerID)
		return linked, isFree, err
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	sameName, err := chars.ListByName(ctx, profile.Name)
	if err != nil {
		return nil, false, err
	}
	var target *character.Character
	for i := range sameName {
		candidate := &sameName[i]
		isFree, err := ownerAllowsClaim(ctx, players, actorID, candidate.PlayerID)
		if err != nil {
			return nil, false, err
		}
		if !isFree {
			return nil, false, nil
		}
		if target == nil || candidate.PlayerID == actorID && target.PlayerID != actorID {
			target = candidate
		}
	}
	return target, true, nil
}

// ownerAllowsClaim reports whether a character of ownerID may go to actorID
// without an Admin: it is the actor's own, or its owner has no login.
func ownerAllowsClaim(ctx context.Context, players *player.Repo, actorID, ownerID int64) (bool, error) {
	if ownerID == actorID {
		return true, nil
	}
	owner, err := players.Get(ctx, ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return owner.Username == nil && owner.PasswordHash == "", nil
}

func (s *Service) applyFreeClaim(ctx context.Context, tx bun.Tx, actorID int64, target *character.Character, profile ExternalCharacter) (*character.Character, error) {
	chars := s.characters.WithTx(tx)
	now := time.Now().UTC()
	auditRepo := audit.NewRepo(tx)
	if target == nil {
		created := characterFromProfile(actorID, profile, "", now)
		if err := chars.Create(ctx, created); err != nil {
			return nil, fmt.Errorf("create claimed character: %w", err)
		}
		return created, auditRepo.Record(ctx, actorID, "character", created.ID, "claim_character", nil, created, "")
	}
	if target.PlayerID == actorID && target.ExternalProfileID == profile.ProfileID {
		return target, nil
	}
	before := *target
	target.PlayerID = actorID
	target.Class = profile.Class
	target.MainSpec = profile.MainSpec
	target.SecondSpec = profile.SecondSpec
	target.GearScore = profile.GearScore
	target.Active = true
	target.ExternalSource = ExternalSourceRisingGods
	target.ExternalProfileID = profile.ProfileID
	target.ExternalProfileURL = profile.ProfileURL
	target.UpdatedAt = now
	if err := chars.Update(ctx, target); err != nil {
		return nil, fmt.Errorf("move claimed character %d: %w", target.ID, err)
	}
	return target, auditRepo.Record(ctx, actorID, "character", target.ID, "claim_character", before, target, "")
}
