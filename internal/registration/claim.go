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
	"raid-website/internal/validation"
)

// ClaimResult is the outcome of ClaimCharacter: either the character now
// belongs to the actor, or a request waits for an Admin's code (ADR 116).
// IsOwnedByOther marks a request made because the character already belongs to
// another player, so the client can tell the player to contact an Admin.
type ClaimResult struct {
	IsClaimed      bool                 `json:"claimed"`
	IsOwnedByOther bool                 `json:"owned_by_other"`
	Character      *character.Character `json:"character,omitempty"`
	Request        *Request             `json:"request,omitempty"`
}

// ClaimCharacter gives a MEMBER or higher a free Rising-Gods character at once
// and falls back to a claim request otherwise (FR-4.6, ADR 116). A character is
// free when no character holds the profile or the name, or the one that does
// belongs to the actor. A character of any other player, with or without a
// login, always needs an Admin.
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
		return s.claimByRequest(ctx, actorID, profileID, false)
	}
	profile, _, err := s.loadUsableProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	var claimed *character.Character
	isOwnedByOther := false
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Checked again inside the write transaction so a request created
		// meanwhile is not bypassed.
		if existing, err := NewRepo(tx).FindBlockingRequest(ctx, ExternalSourceRisingGods, profile.ProfileID); err != nil {
			return err
		} else if existing != nil {
			return ErrRequestExists
		}
		target, isFree, err := freeClaimTarget(ctx, s.characters.WithTx(tx), actorID, profile)
		if err != nil {
			return err
		}
		if !isFree {
			isOwnedByOther = true
			return nil
		}
		claimed, err = s.applyFreeClaim(ctx, tx, actorID, target, profile)
		return err
	})
	if err != nil {
		return nil, err
	}
	if isOwnedByOther {
		return s.claimByRequest(ctx, actorID, profileID, true)
	}
	return &ClaimResult{IsClaimed: true, Character: claimed}, nil
}

func (s *Service) claimByRequest(ctx context.Context, actorID int64, profileID string, isOwnedByOther bool) (*ClaimResult, error) {
	req, err := s.CreateRequest(ctx, &actorID, profileID)
	if err != nil {
		return nil, err
	}
	return &ClaimResult{IsClaimed: false, IsOwnedByOther: isOwnedByOther, Request: req}, nil
}

// freeClaimTarget returns the existing character the claim takes over (nil for
// a new character) and whether the claim may happen without an Admin: only when
// no character holds the profile or the name, or every one that does is the
// actor's own (FR-4.6, ADR 116).
func freeClaimTarget(ctx context.Context, chars *character.Repo, actorID int64, profile ExternalCharacter) (*character.Character, bool, error) {
	if linked, err := chars.FindByExternalProfile(ctx, ExternalSourceRisingGods, profile.ProfileID); err == nil {
		return linked, linked.PlayerID == actorID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	sameName, err := chars.ListByName(ctx, profile.Name)
	if err != nil {
		return nil, false, err
	}
	var target *character.Character
	for i := range sameName {
		if sameName[i].PlayerID != actorID {
			return nil, false, nil
		}
		if target == nil {
			target = &sameName[i]
		}
	}
	return target, true, nil
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
	if target.ExternalProfileID == profile.ProfileID && target.Name == profile.Name {
		return target, nil
	}
	// The target is the actor's own character (freeClaimTarget): link the
	// profile, take a rename from it, and keep specs the owner chose (FR-6.16).
	before := *target
	oldName := target.Name
	target.Name = profile.Name
	target.Class = profile.Class
	if target.SpecsChosenAt == nil {
		target.MainSpec = profile.MainSpec
		target.SecondSpec = profile.SecondSpec
	}
	target.GearScore = profile.GearScore
	target.Active = true
	target.ExternalSource = ExternalSourceRisingGods
	target.ExternalProfileID = profile.ProfileID
	target.ExternalProfileURL = profile.ProfileURL
	target.UpdatedAt = now
	if err := chars.Update(ctx, target); err != nil {
		return nil, fmt.Errorf("link claimed character %d: %w", target.ID, err)
	}
	if target.Name != oldName {
		if err := chars.InsertNameHistory(ctx, &character.NameHistory{CharacterID: target.ID, OldName: oldName, NewName: target.Name, ActorID: actorID, Reason: "Charakter übernommen", CreatedAt: now}); err != nil {
			return nil, fmt.Errorf("record claimed rename: %w", err)
		}
	}
	return target, auditRepo.Record(ctx, actorID, "character", target.ID, "claim_character", before, target, "")
}
