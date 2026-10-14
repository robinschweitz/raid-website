package legacyimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
	"sort"
)

// review resolves provenance and database conflicts before any mutation.
func (s *Service) review(ctx context.Context, source, sheetID string, p Plan, rep *Report, syncOnly bool) (Plan, error) {
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
	permanent := map[string]bool{}
	for _, row := range mappings {
		known[row.Identity] = row
		if row.ExternalID == "" {
			noID[row.Identity] = true
		} else {
			permanent[row.Identity] = true
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
			if row.ExternalID != "" {
				delete(permanent, row.Identity)
				stored, err := s.loot.Get(ctx, previous.LootAwardID)
				if err != nil {
					return p, err
				}
				if !lootPlanMatchesAward(row, stored) {
					row.SyncAction = "correct"
				} else if stored.WithdrawnAt != nil {
					row.SyncAction = "restore"
				}
				if row.SyncAction != "" {
					row.MappingID, row.LootAwardID = previous.ID, previous.LootAwardID
					newLoot = append(newLoot, row)
				}
			} else if previous.Fingerprint != row.Fingerprint {
				rep.Errors = append(rep.Errors, fmt.Sprintf("loot row without Import ID edited: conflict; existing loot will not be overwritten"))
			}
			continue
		}
		if hasPrevious && row.ExternalID == "" {
			rep.Errors = append(rep.Errors, "ambiguous no-ID loot: unseen content or changed multiplicity")
			continue
		}
		newLoot = append(newLoot, row)
	}
	for identity := range permanent {
		previous := known[identity]
		stored, err := s.loot.Get(ctx, previous.LootAwardID)
		if err != nil {
			return p, err
		}
		if stored.WithdrawnAt != nil {
			continue
		}
		newLoot = append(newLoot, LootPlan{MappingID: previous.ID, LootAwardID: previous.LootAwardID, RaidKey: "", SyncAction: "withdraw"})
	}
	if len(noID) > 0 {
		rep.Errors = append(rep.Errors, "ambiguous no-ID loot: removed content or changed multiplicity")
	}
	p.Loot = newLoot
	if err := s.deriveLootBosses(ctx, p.Loot, rep); err != nil {
		return p, err
	}
	checkedSyncRaids := map[int64]bool{}
	for _, row := range p.Loot {
		if row.SyncAction == "" {
			continue
		}
		stored, err := s.loot.Get(ctx, row.LootAwardID)
		if err != nil {
			return p, err
		}
		if checkedSyncRaids[stored.RaidID] {
			continue
		}
		checkedSyncRaids[stored.RaidID] = true
		rd, err := s.raids.Get(ctx, stored.RaidID)
		if err != nil {
			return p, err
		}
		if rd.Status == validation.RaidStatusCancelled || (rd.ArchivedAt != nil && rd.Status != validation.RaidStatusCompleted) {
			rep.Errors = append(rep.Errors, fmt.Sprintf("raid %d: loot sync forbidden for cancelled or inconsistent archived raid", rd.ID))
			continue
		}
		if rd.ArchivedAt != nil {
			p.RequiresReason = true
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("raid %d: reason_required for audited loot sync", rd.ID))
		}
	}
	playerIDs := map[string]int64{}
	for _, pl := range p.Players {
		var existingID int64
		var err error
		if syncOnly {
			existingID, err = s.imports.syncPlayerID(ctx, source, sheetID, pl.Name)
		} else {
			row, findErr := s.players.FindByName(ctx, pl.Name)
			err = findErr
			if err == nil {
				existingID = row.ID
			}
		}
		if errors.Is(err, sql.ErrNoRows) {
			if !syncOnly {
				rep.Created.Players++
			}
		} else if err != nil {
			return p, err
		} else {
			playerIDs[pl.Name] = existingID
		}
		if !syncOnly && pl.HasPoints && pl.Points != 0 {
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
		if syncOnly {
			id, err := s.imports.syncCharacterID(ctx, source, sheetID, c.Name, c.PlayerName)
			if err == nil {
				charIDs[c.Name] = id
				continue
			}
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return p, err
		}
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
		var mappingRaidID int64
		var mappingErr error
		if syncOnly {
			var mapping *syncRaidMapping
			mapping, mappingErr = s.imports.syncRaidMapping(ctx, source, sheetID, rp.Key)
			if mapping != nil {
				mappingRaidID = mapping.RaidID
			}
		} else {
			var mapping *raidMapping
			mapping, mappingErr = s.imports.raidMapping(ctx, source, sheetID, rp.Key)
			if mapping != nil {
				mappingRaidID = mapping.RaidID
			}
		}
		err = mappingErr
		if err == nil {
			existing, err = s.raids.Get(ctx, mappingRaidID)
			if errors.Is(err, sql.ErrNoRows) {
				rep.Errors = append(rep.Errors, fmt.Sprintf("raid %q: mapped raid missing; provenance conflict", rp.Key))
				continue
			}
			if err != nil {
				return p, err
			}
			raidID = existing.ID
			rp.ExistingID = raidID
			if existing.Status == validation.RaidStatusCompleted && existing.ArchivedAt == nil {
				p.RequiresReason = true
				rep.ArchivedRaids++
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("raid %q: reason_required to classify existing imported history as archived without awarding EP again", rp.Key))
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			if syncOnly {
				continue
			}
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
			if row.RaidKey == rp.Key && row.SyncAction == "" {
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
				if exists {
					rows, err := s.att.ListByRaid(ctx, raidID)
					if err != nil {
						return p, err
					}
					for _, stored := range rows {
						if stored.PlayerID == playerIDs[entry.Player] && stored.CharacterID == charIDs[entry.Character] && (stored.CharacterName != entry.Character || stored.Class != entry.Class || stored.Spec != entry.Spec || stored.Role != entry.Role) {
							rep.Warnings = append(rep.Warnings, fmt.Sprintf("raid %q character %q: roster snapshot metadata differs; existing values preserved, explicit audited correction required", rp.Key, entry.Character))
						}
					}
				}
			}
			if !exists {
				rep.Created.Attendance++
				newRows++
			}
		}
		if existing != nil && newRows > 0 {
			if existing.Status == validation.RaidStatusCancelled || (existing.ArchivedAt != nil && existing.Status != validation.RaidStatusCompleted) {
				rep.Errors = append(rep.Errors, fmt.Sprintf("raid %q: new attendance/loot forbidden for cancelled or inconsistent archived raid", rp.Key))
			} else if existing.Status != validation.RaidStatusCompleted && existing.Status != validation.RaidStatusPlanned && existing.Status != validation.RaidStatusOpen {
				rep.Errors = append(rep.Errors, fmt.Sprintf("raid %q: state does not allow import additions", rp.Key))
			} else {
				if existing.ArchivedAt != nil {
					p.RequiresReason = true
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("raid %q: reason_required for audited additions to archived imported raid", rp.Key))
				}
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
						if row.RaidKey != rp.Key || !row.IsMainSpec() {
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
	for _, row := range newLoot {
		switch row.SyncAction {
		case "correct":
			rep.Updated++
		case "withdraw":
			rep.Withdrawn++
		case "restore":
			rep.Restored++
		default:
			rep.Created.Loot++
		}
	}
	if p.Setup != nil {
		rep.Created.Series++
		rep.Created.Raids++
		rep.Created.Attendance += len(p.Setup.Entries)
	}
	for _, row := range newLoot {
		if row.SyncAction == "" && row.IsToken {
			rep.Created.Tokens++
		}
	}
	if len(rep.Errors) > 0 {
		rep.Created = Counts{}
		rep.ArchivedRaids = 0
	}
	return p, nil
}

func lootPlanMatchesAward(row LootPlan, stored *loot.Award) bool {
	if row.ItemID != stored.ItemID || row.Item != stored.ItemName || row.CharacterName != stored.AwardedToCharacterName || row.Method != stored.AwardMethod {
		return false
	}
	return row.Date == nil || row.Date.Equal(stored.AwardedAt)
}

// deriveLootBosses fills the boss of every imported loot row whose bundled catalog item
// belongs to exactly one boss. An item that drops from several bosses, or is not in the
// catalog at all, proves nothing and stays unassigned with a reported warning.
func (s *Service) deriveLootBosses(ctx context.Context, rows []LootPlan, rep *Report) error {
	byItem, err := s.loot.Catalog().BossesByItem(ctx)
	if err != nil {
		return err
	}
	resolved := 0
	ambiguous := map[string]int{}
	missing := map[string]int{}
	for i := range rows {
		row := &rows[i]
		if row.Boss != "" || row.ItemID == "" {
			continue
		}
		bosses := byItem[row.ItemID]
		switch len(bosses) {
		case 0:
			missing[row.ItemID]++
		case 1:
			row.Boss, row.CatalogBossID = bosses[0].Name, bosses[0].ID
			resolved++
		default:
			ambiguous[row.ItemID]++
		}
	}
	for _, itemID := range sortedCounts(ambiguous) {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("loot item %s: drops from several bundled bosses in %d rows; boss stays unknown", itemID, ambiguous[itemID]))
	}
	for _, itemID := range sortedCounts(missing) {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("loot item %s: not in the bundled catalog in %d rows; boss stays unknown", itemID, missing[itemID]))
	}
	if resolved > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("catalog boss derived for %d loot rows", resolved))
	}
	return nil
}

func sortedCounts(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
