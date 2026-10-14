package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"raid-website/internal/audit"
)

var (
	ErrAdoptionReasonRequired = errors.New("sync adoption reason is required")
	ErrAdoptionSelection      = errors.New("sync adoption selection is invalid")
)

type SyncAdoption struct {
	Raids      []SyncRaidCandidate      `json:"raids"`
	Players    []SyncPlayerCandidate    `json:"players"`
	Characters []SyncCharacterCandidate `json:"characters"`
}

type SyncRaidCandidate struct {
	RaidKey string  `json:"raid_key"`
	RaidIDs []int64 `json:"raid_ids"`
	Mapped  int64   `json:"mapped_raid_id,omitempty"`
}
type SyncPlayerCandidate struct {
	PlayerName        string  `json:"player_name"`
	PlayerIDs         []int64 `json:"player_ids"`
	Mapped            int64   `json:"mapped_player_id,omitempty"`
	RequiresSelection bool    `json:"requires_selection"`
}
type SyncCharacterCandidate struct {
	CharacterName string                  `json:"character_name"`
	PlayerName    string                  `json:"player_name"`
	Characters    []SyncCharacterIdentity `json:"characters"`
	Mapped        int64                   `json:"mapped_character_id,omitempty"`
}
type SyncCharacterIdentity struct {
	CharacterID int64 `json:"character_id"`
	PlayerID    int64 `json:"player_id"`
}
type SyncPlayerSelection struct {
	PlayerName string `json:"player_name"`
	PlayerID   int64  `json:"player_id"`
}
type SyncCharacterSelection struct {
	CharacterName string `json:"character_name"`
	PlayerName    string `json:"player_name"`
	CharacterID   int64  `json:"character_id"`
}
type SyncAdoptionInput struct {
	Reason     string                   `json:"reason"`
	Players    []SyncPlayerSelection    `json:"players"`
	Characters []SyncCharacterSelection `json:"characters"`
}

func (s *Service) DryRunSyncAdoption(ctx context.Context, source, sheetID string) (SyncAdoption, error) {
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return SyncAdoption{}, err
	}
	p, _, err := planFromSheet(data)
	if err != nil {
		return SyncAdoption{}, err
	}
	var result SyncAdoption
	err = s.transaction(ctx, false, func(bound *Service) error {
		var err error
		result, err = bound.syncAdoptionCandidates(ctx, source, sheetID, p)
		return err
	})
	return result, err
}

// AdoptSync records exact singleton matches and administrator-selected candidates atomically.
func (s *Service) AdoptSync(ctx context.Context, actorID int64, source, sheetID string, input SyncAdoptionInput) (SyncAdoption, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return SyncAdoption{}, ErrAdoptionReasonRequired
	}
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return SyncAdoption{}, err
	}
	p, _, err := planFromSheet(data)
	if err != nil {
		return SyncAdoption{}, err
	}
	var result SyncAdoption
	err = s.transaction(ctx, true, func(bound *Service) error {
		if _, err := bound.players.Get(ctx, actorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrActorNotFound
			}
			return err
		}
		candidates, err := bound.syncAdoptionCandidates(ctx, source, sheetID, p)
		if err != nil {
			return err
		}
		playerSelections := map[string]int64{}
		for _, selection := range input.Players {
			if selection.PlayerName == "" || selection.PlayerID <= 0 || playerSelections[selection.PlayerName] != 0 {
				return ErrAdoptionSelection
			}
			playerSelections[selection.PlayerName] = selection.PlayerID
		}
		characterSelections := map[string]int64{}
		for _, selection := range input.Characters {
			key := selection.PlayerName + "\x00" + selection.CharacterName
			if selection.PlayerName == "" || selection.CharacterName == "" || selection.CharacterID <= 0 || characterSelections[key] != 0 {
				return ErrAdoptionSelection
			}
			characterSelections[key] = selection.CharacterID
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		playerIDs := map[string]int64{}
		for _, candidate := range candidates.Players {
			id := candidate.Mapped
			if id == 0 && len(candidate.PlayerIDs) == 1 {
				id = candidate.PlayerIDs[0]
			}
			if selected := playerSelections[candidate.PlayerName]; selected != 0 {
				if !containsID(candidate.PlayerIDs, selected) {
					return ErrAdoptionSelection
				}
				id = selected
			} else if id == 0 && (candidate.RequiresSelection || len(candidate.PlayerIDs) > 1) {
				return fmt.Errorf("%w: player %q", ErrAdoptionSelection, candidate.PlayerName)
			}
			if id != 0 {
				playerIDs[candidate.PlayerName] = id
			}
		}
		for _, candidate := range candidates.Raids {
			if candidate.Mapped != 0 || len(candidate.RaidIDs) != 1 {
				continue
			}
			if err := bound.imports.insertSyncRaidMapping(ctx, source, sheetID, candidate.RaidKey, candidate.RaidIDs[0], actorID, now); err != nil {
				return err
			}
			if err := bound.auditSyncMapping(ctx, actorID, "raid", candidate.RaidKey, candidate.RaidIDs[0], input.Reason, now); err != nil {
				return err
			}
		}
		for _, candidate := range candidates.Players {
			if candidate.Mapped != 0 || playerIDs[candidate.PlayerName] == 0 {
				continue
			}
			id := playerIDs[candidate.PlayerName]
			if err := bound.imports.insertSyncPlayerMapping(ctx, source, sheetID, candidate.PlayerName, id, actorID, now); err != nil {
				return err
			}
			if err := bound.auditSyncMapping(ctx, actorID, "player", candidate.PlayerName, id, input.Reason, now); err != nil {
				return err
			}
		}
		for _, candidate := range candidates.Characters {
			if candidate.Mapped != 0 {
				continue
			}
			key := candidate.PlayerName + "\x00" + candidate.CharacterName
			id := characterSelections[key]
			if id == 0 && len(candidate.Characters) == 1 {
				id = candidate.Characters[0].CharacterID
			}
			if id == 0 && len(candidate.Characters) > 1 {
				return fmt.Errorf("%w: character %q", ErrAdoptionSelection, candidate.CharacterName)
			}
			if id == 0 {
				continue
			}
			if !containsCharacter(candidate.Characters, id, playerIDs[candidate.PlayerName]) {
				return ErrAdoptionSelection
			}
			if err := bound.imports.insertSyncCharacterMapping(ctx, source, sheetID, candidate.CharacterName, candidate.PlayerName, id, playerIDs[candidate.PlayerName], actorID, now); err != nil {
				return err
			}
			if err := bound.auditSyncMapping(ctx, actorID, "character", candidate.PlayerName+":"+candidate.CharacterName, id, input.Reason, now); err != nil {
				return err
			}
		}
		result, err = bound.syncAdoptionCandidates(ctx, source, sheetID, p)
		return err
	})
	return result, err
}

func (s *Service) syncAdoptionCandidates(ctx context.Context, source, sheetID string, p Plan) (SyncAdoption, error) {
	result := SyncAdoption{Raids: []SyncRaidCandidate{}, Players: []SyncPlayerCandidate{}, Characters: []SyncCharacterCandidate{}}
	for _, rp := range p.Raids {
		candidate := SyncRaidCandidate{RaidKey: rp.Key, RaidIDs: []int64{}}
		if mapping, err := s.imports.syncRaidMapping(ctx, source, sheetID, rp.Key); err == nil {
			candidate.Mapped = mapping.RaidID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		if err := s.imports.db.NewSelect().TableExpr("raids").Column("id").Where("name = ?", rp.Key).Scan(ctx, &candidate.RaidIDs); err != nil {
			return result, err
		}
		result.Raids = append(result.Raids, candidate)
	}
	for _, pl := range p.Players {
		candidate := SyncPlayerCandidate{PlayerName: pl.Name, PlayerIDs: []int64{}}
		if id, err := s.imports.syncPlayerID(ctx, source, sheetID, pl.Name); err == nil {
			candidate.Mapped = id
		} else if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		if err := s.imports.db.NewSelect().TableExpr("players").Column("id").Where("name = ?", pl.Name).Scan(ctx, &candidate.PlayerIDs); err != nil {
			return result, err
		}
		candidate.RequiresSelection = candidate.Mapped == 0 && len(candidate.PlayerIDs) == 0
		result.Players = append(result.Players, candidate)
	}
	for _, ch := range p.Characters {
		candidate := SyncCharacterCandidate{CharacterName: ch.Name, PlayerName: ch.PlayerName, Characters: []SyncCharacterIdentity{}}
		if id, err := s.imports.syncCharacterID(ctx, source, sheetID, ch.Name, ch.PlayerName); err == nil {
			candidate.Mapped = id
		} else if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		matches, err := s.imports.charactersByName(ctx, ch.Name)
		if err != nil {
			return result, err
		}
		for _, match := range matches {
			candidate.Characters = append(candidate.Characters, SyncCharacterIdentity{CharacterID: match.ID, PlayerID: match.PlayerID})
		}
		result.Characters = append(result.Characters, candidate)
	}
	// A legacy sheet may use a character name where the website stores the
	// account owner under another name. Offer that owner for explicit approval.
	for _, candidate := range result.Characters {
		for i := range result.Players {
			if result.Players[i].PlayerName != candidate.PlayerName || result.Players[i].Mapped != 0 {
				continue
			}
			for _, character := range candidate.Characters {
				if !containsID(result.Players[i].PlayerIDs, character.PlayerID) {
					result.Players[i].PlayerIDs = append(result.Players[i].PlayerIDs, character.PlayerID)
				}
			}
		}
	}
	return result, nil
}

func (s *Service) auditSyncMapping(ctx context.Context, actorID int64, kind, key string, targetID int64, reason string, now time.Time) error {
	after, err := json.Marshal(map[string]any{"kind": kind, "key": key, "target_id": targetID})
	if err != nil {
		return err
	}
	return s.imports.writeAudit(ctx, &audit.Log{ActorID: &actorID, EntityType: "legacy_sync_mapping", EntityID: kind + ":" + key, Action: "adopt", BeforeJSON: "null", AfterJSON: string(after), Reason: reason, CreatedAt: now})
}

func containsID(ids []int64, wanted int64) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}
func containsCharacter(rows []SyncCharacterIdentity, characterID, playerID int64) bool {
	for _, row := range rows {
		if row.CharacterID == characterID && row.PlayerID == playerID {
			return true
		}
	}
	return false
}
