package legacyimport

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/validation"
)

func TestImportedRosterSnapshotsUseRaidArchiveAndPreserveRenamedHistory(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	data.Character = []byte(strings.ReplaceAll(charCSV, "Todesritter", "Magier"))
	s.fetcher = fakeFetcher{data: data}
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, "")
	if err != nil || rep.Created.Attendance != 2 {
		t.Fatalf("merge: %+v %v", rep, err)
	}
	rd, err := s.raids.FindByName(ctx, "ICC25-01")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.att.ListByRaid(ctx, rd.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("attendance: %+v %v", rows, err)
	}
	if rows[0].CharacterName != "Nylama" || rows[0].Class != "Todesritter" || rows[0].Spec != "Blut (Tank)" || rows[0].Role != validation.RoleTank {
		t.Fatalf("source overwritten by character tab: %+v", rows[0])
	}
	c, err := s.chars.Get(ctx, rows[0].CharacterID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Class != "Magier" || c.MainSpec != "" {
		t.Fatalf("live character mutated: %+v", c)
	}
	oldName := c.Name
	c.Name, c.MainSpec = "Renamed", "Current spec"
	if err := s.chars.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := s.chars.InsertNameHistory(ctx, &character.NameHistory{CharacterID: c.ID, OldName: oldName, NewName: c.Name, ActorID: actor, Reason: "renamed", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	data.Hash = "renamed-live-character"
	s.fetcher = fakeFetcher{data: data}
	previewImport(t, s, "source", "sheet")
	rep, err = s.Merge(ctx, actor, "source", "sheet", data.Hash, "")
	if err != nil || len(rep.Errors) != 0 || rep.Created.Attendance != 0 || rep.Created.Characters != 0 {
		t.Fatalf("rename replay: %+v %v", rep, err)
	}
	stored, err := s.att.Get(ctx, rows[0].ID)
	if err != nil || stored.CharacterName != oldName || stored.Spec != "Blut (Tank)" {
		t.Fatalf("renamed snapshot: %+v %v", stored, err)
	}
	now := time.Now().UTC()
	rd.ArchivedAt = &now
	if err := s.raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	data.Raid = []byte(strings.ReplaceAll(raidCSV, "Blut (Tank)", "Frost"))
	data.Hash = "edited-roster-metadata"
	s.fetcher = fakeFetcher{data: data}
	preview := previewImport(t, s, "source", "sheet")
	if !strings.Contains(strings.Join(preview.Warnings, "\n"), "roster snapshot metadata differs") {
		t.Fatalf("missing metadata warning: %+v", preview)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", data.Hash, "not an automatic correction")
	if err != nil || rep.Created.Attendance != 0 || !strings.Contains(strings.Join(rep.Warnings, "\n"), "explicit audited correction required") {
		t.Fatalf("metadata replay: %+v %v", rep, err)
	}
	stored, err = s.att.Get(ctx, rows[0].ID)
	if err != nil || stored.Spec != "Blut (Tank)" {
		t.Fatalf("import overwrote snapshot: %+v %v", stored, err)
	}
	stored.CharacterName, stored.Class, stored.Spec = "", "", ""
	if err := s.att.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	data.Hash = "unknown-migrated-metadata"
	s.fetcher = fakeFetcher{data: data}
	preview = previewImport(t, s, "source", "sheet")
	if !strings.Contains(strings.Join(preview.Warnings, "\n"), "roster snapshot metadata differs") {
		t.Fatalf("missing unknown-history warning: %+v", preview)
	}
	if _, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, ""); err != nil {
		t.Fatal(err)
	}
	stored, err = s.att.Get(ctx, rows[0].ID)
	if err != nil || stored.CharacterName != "" || stored.Class != "" || stored.Spec != "" {
		t.Fatalf("history fabricated: %+v %v", stored, err)
	}
}

func TestIncrementalImportedRosterPreservesUnknownSourceMetadata(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	previewImport(t, s, "source", "sheet")
	if rep, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, ""); err != nil || len(rep.Errors) != 0 {
		t.Fatalf("initial merge: %+v %v", rep, err)
	}
	rd, err := s.raids.FindByName(ctx, "ICC25-01")
	if err != nil {
		t.Fatal(err)
	}
	data.Raid = append(data.Raid, []byte("\"ICC25-01\",\"09.02.2026\",\"3\",\"Historicalalt\",\"Heiler\",\"Unknown historical class\",\"\",\"\"\n")...)
	data.Character = append(data.Character, []byte("\"Historicalalt\",\"Amisu\",\"Magier\",\"Feuer\",\"Frost\",\"\",\"\"\n")...)
	data.Hash = "incremental-unknown-roster"
	s.fetcher = fakeFetcher{data: data}
	preview := previewImport(t, s, "source", "sheet")
	warnings := strings.Join(preview.Warnings, "\n")
	if preview.Created.Attendance != 1 || !strings.Contains(warnings, "unknown roster class") || !strings.Contains(warnings, "missing roster spec") {
		t.Fatalf("preview metadata warnings: %+v", preview)
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, " ")
	if err != nil || len(rep.Errors) == 0 || !strings.Contains(strings.Join(rep.Errors, "\n"), "reason_required") {
		t.Fatalf("reason guard: %+v %v", rep, err)
	}
	rows, err := s.att.ListByRaid(ctx, rd.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("reason failure wrote attendance: %+v %v", rows, err)
	}
	reason := "additional historical source row"
	rep, err = s.Merge(ctx, actor, "source", "sheet", data.Hash, reason)
	if err != nil || len(rep.Errors) != 0 || rep.Created.Attendance != preview.Created.Attendance {
		t.Fatalf("incremental merge: %+v %v", rep, err)
	}
	rows, err = s.att.ListByRaid(ctx, rd.ID)
	if err != nil || len(rows) != 3 {
		t.Fatalf("incremental attendance: %+v %v", rows, err)
	}
	stored := rows[2]
	if stored.CharacterName != "Historicalalt" || stored.Class != "Unknown historical class" || stored.Spec != "" || stored.Role != validation.RoleHeal {
		t.Fatalf("source snapshot replaced: %+v", stored)
	}
	live, err := s.chars.Get(ctx, stored.CharacterID)
	if err != nil || live.Class != "Magier" || live.MainSpec != "Feuer" {
		t.Fatalf("live defaults changed: %+v %v", live, err)
	}
	logs, err := audit.NewRepo(s.imports.db).ListByEntity(ctx, "attendance", strconv.FormatInt(stored.ID, 10))
	if err != nil || len(logs) != 1 {
		t.Fatalf("audit: %+v %v", logs, err)
	}
	var audited attendance.Attendance
	if err := json.Unmarshal([]byte(logs[0].AfterJSON), &audited); err != nil {
		t.Fatal(err)
	}
	if logs[0].ActorID == nil || *logs[0].ActorID != actor || logs[0].Reason != reason || logs[0].BeforeJSON != "null" || audited.CharacterName != stored.CharacterName || audited.Class != stored.Class || audited.Spec != "" {
		t.Fatalf("incorrect audit snapshot: %+v %+v", logs[0], audited)
	}
	data.Raid = []byte(strings.ReplaceAll(string(data.Raid), "\"Unknown historical class\",\"\"", "\"Magier\",\"Feuer\""))
	data.Hash = "changed-existing-roster-metadata"
	s.fetcher = fakeFetcher{data: data}
	preview = previewImport(t, s, "source", "sheet")
	if !strings.Contains(strings.Join(preview.Warnings, "\n"), "roster snapshot metadata differs") {
		t.Fatalf("missing mismatch warning: %+v", preview)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", data.Hash, "")
	if err != nil || len(rep.Errors) != 0 || rep.Created.Attendance != 0 {
		t.Fatalf("metadata replay: %+v %v", rep, err)
	}
	unchanged, err := s.att.Get(ctx, stored.ID)
	if err != nil || unchanged.Class != stored.Class || unchanged.Spec != stored.Spec {
		t.Fatalf("metadata overwritten: %+v %v", unchanged, err)
	}
}
