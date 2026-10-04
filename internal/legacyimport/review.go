package legacyimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

// review resolves provenance and database conflicts before any mutation.
func (s *Service) review(ctx context.Context, source, sheetID string, p Plan, rep *Report) (Plan, error) {
	gap, err := s.imports.hasLegacyProvenanceGap(ctx, source, sheetID)
	if err != nil {
		return p, err
	}
	if gap {
		if _, err := s.imports.FindByHash(ctx, source, sheetID, rep.RawHash); errors.Is(err, sql.ErrNoRows) {
			rep.Errors = append(rep.Errors, "legacy provenance missing: changed snapshot cannot be imported safely")
		} else if err != nil {
			return p, err
		}
	}
	mappings, err := s.imports.mappings(ctx, source, sheetID)
	if err != nil {
		return p, err
	}
	known := map[string]lootMapping{}
	noID := map[string]bool{}
	for _, row := range mappings {
		known[row.Identity] = row
		if row.ExternalID == "" {
			noID[row.Identity] = true
		}
	}
	history, err := s.imports.List(ctx)
	if err != nil {
		return p, err
	}
	hasPrevious := false
	for _, imp := range history {
		if imp.Source == source && imp.SourceURL == sheetID && imp.Status == "MERGED" {
			hasPrevious = true
		}
	}
	newLoot := make([]LootPlan, 0, len(p.Loot))
	for _, row := range p.Loot {
		if row.ExternalID == "" {
			delete(noID, row.Identity)
		}
		if previous, ok := known[row.Identity]; ok {
			if previous.Fingerprint != row.Fingerprint {
				rep.Errors = append(rep.Errors, fmt.Sprintf("Import ID %q edited: conflict; existing loot will not be overwritten", row.ExternalID))
			}
			continue
		}
		if hasPrevious && row.ExternalID == "" {
			rep.Errors = append(rep.Errors, "ambiguous no-ID loot: unseen content or changed multiplicity")
			continue
		}
		newLoot = append(newLoot, row)
	}
	if len(noID) > 0 {
		rep.Errors = append(rep.Errors, "ambiguous no-ID loot: removed content or changed multiplicity")
	}
	p.Loot = newLoot
	playerIDs := map[string]int64{}
	for _, pl := range p.Players {
		existing, err := s.players.FindByName(ctx, pl.Name)
		if errors.Is(err, sql.ErrNoRows) {
			rep.Created.Players++
		} else if err != nil {
			return p, err
		} else {
			playerIDs[pl.Name] = existing.ID
		}
		if pl.HasPoints && pl.Points != 0 {
			has := false
			if playerIDs[pl.Name] != 0 {
				has, err = s.points.HasTransactionType(ctx, playerIDs[pl.Name], "legacy_open")
				if err != nil {
					return p, err
				}
			}
			if !has {
				rep.Created.Points++
			}
		}
	}
	charIDs := map[string]int64{}
	for _, c := range p.Characters {
		matches, err := s.imports.charactersByName(ctx, c.Name)
		if err != nil {
			return p, err
		}
		owned := 0
		for _, existing := range matches {
			if existing.PlayerID == playerIDs[c.PlayerName] {
				owned++
				charIDs[c.Name] = existing.ID
			}
		}
		if owned > 1 || (len(matches) > 0 && owned == 0) {
			rep.Errors = append(rep.Errors, fmt.Sprintf("character %q: ambiguous identity or ownership mismatch for %q", c.Name, c.PlayerName))
		}
		if len(matches) == 0 {
			rep.Created.Characters++
		}
	}
	for i := range p.Raids {
		rp := &p.Raids[i]
		raidID := int64(0)
		var existing *raid.Raid
		mapping, err := s.imports.raidMapping(ctx, source, sheetID, rp.Key)
		if err == nil {
			existing, err = s.raids.Get(ctx, mapping.RaidID)
			if errors.Is(err, sql.ErrNoRows) {
				rep.Errors = append(rep.Errors, fmt.Sprintf("raid %q: mapped raid missing; provenance conflict", rp.Key))
				continue
			}
			if err != nil {
				return p, err
			}
			raidID = existing.ID
			rp.ExistingID = raidID
		} else if errors.Is(err, sql.ErrNoRows) {
			_, err = s.raids.FindByName(ctx, rp.Key)
			if err == nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("raid %q: name collision without source/sheet provenance", rp.Key))
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return p, err
			}
			rep.Created.Raids++
		} else {
			return p, err
		}
		newRows := 0
		for _, row := range newLoot {
			if row.RaidKey == rp.Key {
				newRows++
			}
		}
		seen := map[string]bool{}
		for _, entry := range rp.Entries {
			key := entry.Player + "\x00" + entry.Character
			if seen[key] {
				continue
			}
			seen[key] = true
			exists := false
			if raidID != 0 && playerIDs[entry.Player] != 0 && charIDs[entry.Character] != 0 {
				exists, err = s.att.Exists(ctx, raidID, playerIDs[entry.Player], charIDs[entry.Character])
				if err != nil {
					return p, err
				}
			}
			if !exists {
				rep.Created.Attendance++
				newRows++
			}
		}
		if existing != nil && newRows > 0 {
			if existing.ArchivedAt != nil || existing.Status == validation.RaidStatusCancelled {
				rep.Errors = append(rep.Errors, fmt.Sprintf("raid %q: new attendance/loot forbidden for cancelled or archived raid", rp.Key))
			} else if existing.Status != validation.RaidStatusCompleted && existing.Status != validation.RaidStatusPlanned && existing.Status != validation.RaidStatusOpen {
				rep.Errors = append(rep.Errors, fmt.Sprintf("raid %q: state does not allow import additions", rp.Key))
			} else {
				p.RequiresReason = true
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("raid %q: reason_required for audited additions to existing imported raid", rp.Key))
				if existing.Status == validation.RaidStatusCompleted {
					// Match the correction service's per-addition eligibility deltas.
					participants := map[string]bool{}
					mainSpec := map[string]bool{}
					for _, pl := range p.Players {
						if playerIDs[pl.Name] == 0 {
							continue
						}
						mainSpec[pl.Name], err = s.loot.PlayerHasMainSpecAward(ctx, raidID, playerIDs[pl.Name])
						if err != nil {
							return p, err
						}
					}
					attendance, err := s.att.ListByRaid(ctx, raidID)
					if err != nil {
						return p, err
					}
					for _, row := range attendance {
						for name, id := range playerIDs {
							if id == row.PlayerID {
								participants[name] = true
							}
						}
					}
					for _, entry := range rp.Entries {
						if !participants[entry.Player] && !mainSpec[entry.Player] {
							rep.Created.Points++
						}
						participants[entry.Player] = true
					}
					for _, row := range newLoot {
						if row.RaidKey != rp.Key || !row.IsMainSpec {
							continue
						}
						if participants[row.PlayerName] && !mainSpec[row.PlayerName] {
							rep.Created.Points++
						}
						mainSpec[row.PlayerName] = true
					}
				}
			}
		}
	}
	rep.Created.Loot = len(newLoot)
	for _, row := range newLoot {
		if row.IsToken {
			rep.Created.Tokens++
		}
	}
	if len(rep.Errors) > 0 {
		rep.Created = Counts{}
	}
	return p, nil
}
