package legacyimport

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"raid-website/internal/audit"
	"raid-website/internal/loot"
	"raid-website/internal/raidcorrection"
)

func TestLootArchiveRecipientIsCharacter(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	data.Character = append(data.Character, []byte("\"Opinoid\",\"Amisu\",\"Priester\",\"Disziplin\",\"\",\"\",\"\"\n")...)
	data.Loot = []byte(strings.ReplaceAll(lootCSV, "\"Amisu\"", "\"Opinoid\""))
	s.fetcher = fakeFetcher{data: data}
	preview := previewImport(t, s, "source", "sheet")
	if preview.Created.Players != 2 || preview.Created.Characters != 3 || preview.Created.Attendance != 2 {
		t.Fatalf("character recipient created a separate player or invented attendance: %+v", preview)
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, "")
	if err != nil || len(rep.Errors) != 0 {
		t.Fatalf("merge: %+v %v", rep, err)
	}
	rd, err := s.raids.FindByName(ctx, "ICC25-01")
	if err != nil {
		t.Fatal(err)
	}
	awards, err := s.loot.ListByRaid(ctx, rd.ID)
	if err != nil || len(awards) != 2 {
		t.Fatalf("loot: %+v %v", awards, err)
	}
	owner, err := s.players.FindByName(ctx, "Amisu")
	if err != nil {
		t.Fatal(err)
	}
	award := awards[1]
	if award.AwardedToPlayerID != owner.ID || award.AwardedToCharacterID == nil || award.AwardedToCharacterName != "Opinoid" {
		t.Fatalf("recipient is not the character and its owner: %+v", award)
	}
	c, err := s.chars.Get(ctx, *award.AwardedToCharacterID)
	if err != nil || c.Name != "Opinoid" || c.PlayerID != owner.ID {
		t.Fatalf("character: %+v %v", c, err)
	}
	if !loot.IsTokenAward(award.ItemName) || award.AwardedToPlayerID != owner.ID {
		t.Fatalf("token loot must belong to the character owner: %+v", award)
	}
	c.Name = "Renamed"
	if err := s.chars.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	corrections := raidcorrection.NewService(raidcorrection.NewRepo(s.imports.db))
	corrected, err := corrections.CorrectLoot(ctx, rd.ID, award.ID, actor, "Notiz ergänzen", func(row *loot.Award) { row.Notes = "Korrigierte Notiz" })
	if err != nil || corrected.AwardedToCharacterName != "Opinoid" {
		t.Fatalf("rename or note correction changed historical name: %+v %v", corrected, err)
	}
	roster, err := s.att.ListByRaid(ctx, rd.ID)
	if err != nil || len(roster) != 2 {
		t.Fatalf("roster: %+v %v", roster, err)
	}
	corrected, err = corrections.CorrectLoot(ctx, rd.ID, award.ID, actor, "Empfänger korrigieren", func(row *loot.Award) {
		row.CatalogBossID = awards[0].CatalogBossID
		row.ItemID = awards[0].ItemID
		row.AwardedToPlayerID = roster[1].PlayerID
		row.AwardedToCharacterID = &roster[1].CharacterID
	})
	if err != nil || corrected.AwardedToCharacterName != "" {
		t.Fatalf("recipient correction kept previous character name: %+v %v", corrected, err)
	}
}

func TestLootOnlyCharacterPreservesNameWithoutInventingRoster(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	setLoot(s, stableHeader+"ICC25-02,09.02.2026,Opinoid,Blutfall,Second,50178,opinoid-drop\n", "loot-only")
	preview := previewImport(t, s, "source", "sheet")
	if preview.Created.Characters != 3 || preview.Created.Attendance != 2 || !strings.Contains(strings.Join(preview.Warnings, "\n"), "created with own player") {
		t.Fatalf("loot-only preview: %+v", preview)
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", "loot-only", "")
	if err != nil || len(rep.Errors) != 0 {
		t.Fatalf("merge: %+v %v", rep, err)
	}
	rd, err := s.raids.FindByName(ctx, "ICC25-02")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.loot.ListByRaid(ctx, rd.ID)
	if err != nil || len(rows) != 1 || rows[0].AwardedToCharacterName != "Opinoid" || rows[0].AwardedToCharacterID == nil {
		t.Fatalf("missing loot-only identity: %+v %v", rows, err)
	}
	roster, err := s.att.ListByRaid(ctx, rd.ID)
	if err != nil || len(roster) != 0 {
		t.Fatalf("invented roster: %+v %v", roster, err)
	}
}

func TestIncrementalLootUsesCharacterOwnerAndAuditsName(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	data.Character = append(data.Character, []byte("\"Opinoid\",\"Amisu\",\"Priester\",\"Disziplin\",\"\",\"\",\"\"\n")...)
	data.Loot = []byte(stableHeader + stableDrop + "first\n")
	s.fetcher = fakeFetcher{data: data}
	previewImport(t, s, "source", "sheet")
	if rep, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, ""); err != nil || len(rep.Errors) != 0 {
		t.Fatalf("initial merge: %+v %v", rep, err)
	}
	data.Loot = append(data.Loot, []byte("ICC25-01,09.02.2026,Opinoid,Blutfall,Main,50178,second\n")...)
	data.Hash = "incremental-character-loot"
	s.fetcher = fakeFetcher{data: data}
	preview := previewImport(t, s, "source", "sheet")
	if preview.Created != (Counts{Loot: 1, Points: 1}) {
		t.Fatalf("expected eligibility correction for owner Amisu: %+v", preview)
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, "Historischen Loot ergänzen")
	if err != nil || len(rep.Errors) != 0 || rep.Created != preview.Created {
		t.Fatalf("incremental merge: %+v %v", rep, err)
	}
	rd, err := s.raids.FindByName(ctx, "ICC25-01")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.loot.ListByRaid(ctx, rd.ID)
	if err != nil || len(rows) != 2 || rows[1].AwardedToCharacterName != "Opinoid" || rows[1].AwardedToCharacterID == nil {
		t.Fatalf("incremental identity: %+v %v", rows, err)
	}
	owner, err := s.players.FindByName(ctx, "Amisu")
	if err != nil {
		t.Fatal(err)
	}
	balance, err := s.points.Balance(ctx, owner.ID)
	if err != nil || balance != 20 || rows[1].AwardedToPlayerID != owner.ID {
		t.Fatalf("owner eligibility correction: balance=%d award=%+v err=%v", balance, rows[1], err)
	}
	logs, err := audit.NewRepo(s.imports.db).ListByEntity(ctx, "loot_award", strconv.FormatInt(rows[1].ID, 10))
	if err != nil || len(logs) != 1 {
		t.Fatalf("loot audit: %+v %v", logs, err)
	}
	var snapshot loot.Award
	if err := json.Unmarshal([]byte(logs[0].AfterJSON), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.AwardedToCharacterName != "Opinoid" || snapshot.AwardedToPlayerID != owner.ID || logs[0].ActorID == nil || *logs[0].ActorID != actor || logs[0].Reason != "Historischen Loot ergänzen" {
		t.Fatalf("audit identity: %+v %+v", snapshot, logs[0])
	}
	replay, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, "")
	if err != nil || !replay.Skipped {
		t.Fatalf("replay: %+v %v", replay, err)
	}
}
