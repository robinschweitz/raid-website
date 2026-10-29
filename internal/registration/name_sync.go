package registration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/character"
)

// SyncLinkedCharacterNames checks stable Rising-Gods profile IDs rather than
// names, so a renamed character keeps its local identity and all references.
// A failed profile lookup leaves local data unchanged and does not stop others.
func (s *Service) SyncLinkedCharacterNames(ctx context.Context) (int, error) {
	chars, err := s.characters.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list characters for Rising-Gods rename sync: %w", err)
	}
	renamed := 0
	var failures []error
	for _, c := range chars {
		if err := ctx.Err(); err != nil {
			return renamed, err
		}
		if c.ExternalSource != ExternalSourceRisingGods || strings.TrimSpace(c.ExternalProfileID) == "" {
			continue
		}
		profile, _, err := s.external.LoadCharacter(ctx, c.ExternalProfileID)
		if err != nil {
			failures = append(failures, fmt.Errorf("profile %s: %w", c.ExternalProfileID, err))
			continue
		}
		if profile.ProfileID != c.ExternalProfileID || strings.TrimSpace(profile.Name) == "" {
			failures = append(failures, fmt.Errorf("profile %s: invalid identity/name", c.ExternalProfileID))
			continue
		}
		if profile.Name == c.Name {
			continue
		}
		changed, err := s.syncCharacterName(ctx, c.ID, c.ExternalProfileID, profile.Name)
		if err != nil {
			failures = append(failures, fmt.Errorf("character %d: %w", c.ID, err))
		} else if changed {
			renamed++
		}
	}
	return renamed, errors.Join(failures...)
}

func (s *Service) syncCharacterName(ctx context.Context, id int64, profileID, newName string) (bool, error) {
	changed := false
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		chars := s.characters.WithTx(tx)
		c, err := chars.Get(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // deleted while syncing
		}
		if err != nil {
			return err
		}
		if c.ExternalSource != ExternalSourceRisingGods || c.ExternalProfileID != profileID || c.Name == newName {
			return nil
		}
		now := time.Now().UTC()
		before := mustJSON(c)
		oldName := c.Name
		c.Name = newName
		c.NameChangedAt = &now
		c.UpdatedAt = now
		if err := chars.Update(ctx, c); err != nil {
			return err
		}
		// The character owner is the required history actor (FK to players).
		// The reason and audit action explicitly mark this as an automated change.
		if err := chars.InsertNameHistory(ctx, &character.NameHistory{
			CharacterID: c.ID, OldName: oldName, NewName: newName,
			ActorID: c.PlayerID, Reason: "Automatischer Rising-Gods-Profilabgleich",
			CreatedAt: now,
		}); err != nil {
			return err
		}
		actorID := c.PlayerID
		if err := audit.NewRepo(tx).Insert(ctx, &audit.Log{
			ActorID: &actorID, EntityType: "character", EntityID: fmt.Sprint(c.ID),
			Action: "auto_rename_rising_gods", BeforeJSON: before, AfterJSON: mustJSON(c),
			Reason: "Automatischer Rising-Gods-Profilabgleich", CreatedAt: now,
		}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}
