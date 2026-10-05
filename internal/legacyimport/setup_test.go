package legacyimport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"raid-website/internal/raidcorrection"
)

func setupConfig() *SetupConfig {
	return &SetupConfig{Name: "ICC25", Leader: "Oxora", FirstDate: "2026-10-05", StartTime: "19:00", Timezone: "Europe/Berlin", Weekdays: []int{1}, IntervalWeeks: 1}
}

func setupCSV() []byte {
	return []byte(",Charakter,Spieler,Klasse,Spec,Rolle\n1,Oxora,Oxora,Paladin,Vergeltung,Mdd\n2,Newtank,Tankplayer,Druide,,Tank\n3,,,,,Rdd\n,Ignorednote,,,,\n,Charakter,Spieler,,,\n1,Secondary,Secondary,,,\n")
}

func TestSetupBootstrapPreviewRosterScheduleAndIdempotence(t *testing.T) {
	s, actor := importFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data := sampleSheet()
	data.Setup = setupCSV()
	s.fetcher = fakeFetcher{data: data}
	preview, err := s.DryRunWithSetup(ctx, "source", "sheet", setupConfig())
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if preview.Created.Raids != 2 || preview.Created.Series != 1 || preview.Created.Attendance != 4 || preview.Setup == nil || preview.Setup.Attendance != 2 {
		t.Fatalf("preview counts: %+v", preview)
	}
	if !strings.Contains(strings.Join(preview.Warnings, " "), "spec missing") || !strings.Contains(strings.Join(preview.Warnings, " "), "secondary tables") {
		t.Fatalf("warnings: %+v", preview.Warnings)
	}
	if rows, err := s.raids.List(ctx, true); err != nil || len(rows) != 0 {
		t.Fatalf("preview wrote raids: %+v %v", rows, err)
	}
	report, err := s.MergeWithSetup(ctx, actor, "source", "sheet", preview.RawHash, "", setupConfig())
	if err != nil || len(report.Errors) != 0 || report.Created != preview.Created {
		t.Fatalf("merge: %+v %v (preview %+v)", report, err, preview.Created)
	}
	mapping, err := s.imports.setupMapping(ctx, "source", "sheet")
	if err != nil {
		t.Fatal(err)
	}
	series, err := s.series.Get(ctx, mapping.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	leader, err := s.players.Get(ctx, series.RaidLeaderID)
	if err != nil || leader.Name != "Oxora" || series.RaidLeaderID == actor {
		t.Fatalf("leader: %+v %v", leader, err)
	}
	rd, err := s.raids.Get(ctx, mapping.RaidID)
	if err != nil || rd.ScheduledStart.Format(time.RFC3339) != "2026-10-05T17:00:00Z" || rd.ArchivedAt != nil || rd.CompletedAt != nil || rd.Status != "PLANNED" {
		t.Fatalf("raid: %+v %v", rd, err)
	}
	if len(series.Weekdays) != 1 || series.Weekdays[0] != 1 || series.IntervalWeeks != 1 || series.StartTime != "19:00" {
		t.Fatalf("series: %+v", series)
	}
	rows, err := s.att.ListByRaid(ctx, rd.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("roster: %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.CharacterName == "Newtank" && row.Spec != "" {
			t.Fatal("invented spec")
		}
	}
	historical, err := s.raids.FindByName(ctx, "ICC25-01")
	if err != nil || historical.ArchivedAt == nil {
		t.Fatalf("history: %+v %v", historical, err)
	}
	// The import exception must not open ordinary edit endpoints on archived history.
	historicalRows, err := s.att.ListByRaid(ctx, historical.ID)
	if err != nil || len(historicalRows) == 0 {
		t.Fatalf("historical roster: %+v %v", historicalRows, err)
	}
	corrections := raidcorrection.NewService(raidcorrection.NewRepo(s.imports.db))
	if _, err := corrections.CorrectAttendance(ctx, historical.ID, historicalRows[0].ID, actor, "ordinary edit", nil); !errors.Is(err, raidcorrection.ErrRaidState) {
		t.Fatalf("ordinary archived correction must be rejected: %v", err)
	}
	repeated, err := s.MergeWithSetup(ctx, actor, "source", "sheet", "", "", setupConfig())
	if err != nil || !repeated.Skipped {
		t.Fatalf("idempotence: %+v %v", repeated, err)
	}
}

func TestSetupConfigurationIsBoundToPreview(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	data.Setup = setupCSV()
	s.fetcher = fakeFetcher{data: data}
	preview, err := s.DryRunWithSetup(ctx, "source", "sheet", setupConfig())
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	changed := setupConfig()
	changed.StartTime = "20:00"
	if _, err := s.MergeWithSetup(ctx, actor, "source", "sheet", preview.RawHash, "", changed); err != ErrSourceChanged {
		t.Fatalf("expected source_changed, got %v", err)
	}
	rows, err := s.raids.List(ctx, true)
	if err != nil || len(rows) != 0 {
		t.Fatalf("wrote raids: %+v %v", rows, err)
	}
}

func TestSetupInvalidInputsBlockWholeImport(t *testing.T) {
	for _, test := range []string{"leader", "date", "timezone", "role", "owner", "empty"} {
		t.Run(test, func(t *testing.T) {
			s, actor := importFixture(t)
			ctx := context.Background()
			data, config := sampleSheet(), setupConfig()
			data.Setup = setupCSV()
			switch test {
			case "leader":
				config.Leader = "Missing"
			case "date":
				config.FirstDate = "2026-10-06"
			case "timezone":
				config.Timezone = "invalid/timezone"
			case "role":
				data.Setup = []byte(strings.ReplaceAll(string(data.Setup), "Mdd", "Unknown"))
			case "owner":
				data.Setup = append(data.Setup[:strings.Index(string(data.Setup), ",Ignorednote")], []byte("4,Oxora,Other,Paladin,Vergeltung,Mdd\n")...)
			case "empty":
				data.Setup = nil
			}
			s.fetcher = fakeFetcher{data: data}
			preview, err := s.DryRunWithSetup(ctx, "source", "sheet", config)
			if err != nil || len(preview.Errors) == 0 || preview.Created != (Counts{}) {
				t.Fatalf("preview: %+v %v", preview, err)
			}
			if _, err := s.MergeWithSetup(ctx, actor, "source", "sheet", preview.RawHash, "", config); err != ErrPreviewRequired {
				t.Fatalf("invalid preview accepted: %v", err)
			}
		})
	}
}

func TestSetupCanBeBootstrappedAfterHistoricalImportAndPreservesLaterEdits(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	s.fetcher = fakeFetcher{data: data}
	previewImport(t, s, "source", "sheet")
	if rep, err := s.Merge(ctx, actor, "source", "sheet", data.Hash, ""); err != nil || len(rep.Errors) != 0 {
		t.Fatalf("history: %+v %v", rep, err)
	}
	data.Setup = setupCSV()
	s.fetcher = fakeFetcher{data: data}
	preview, err := s.DryRunWithSetup(ctx, "source", "sheet", setupConfig())
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if rep, err := s.MergeWithSetup(ctx, actor, "source", "sheet", preview.RawHash, "", setupConfig()); err != nil || len(rep.Errors) != 0 || rep.Created.Series != 1 {
		t.Fatalf("setup: %+v %v", rep, err)
	}
	mapping, err := s.imports.setupMapping(ctx, "source", "sheet")
	if err != nil {
		t.Fatal(err)
	}
	rd, err := s.raids.Get(ctx, mapping.RaidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.Notes = "Manual raid changes"
	if err := s.raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	// A subsequent source roster change must not create players or rewrite setup.
	data.Hash = "setup-source-changed"
	data.Setup = []byte(",Charakter,Spieler,Klasse,Spec,Rolle\n1,Newname,Newname,Paladin,Vergeltung,Mdd\n")
	s.fetcher = fakeFetcher{data: data}
	preview, err = s.DryRunWithSetup(ctx, "source", "sheet", setupConfig())
	if err != nil || len(preview.Errors) != 0 || !preview.Setup.AlreadyImported {
		t.Fatalf("preview repeat: %+v %v", preview, err)
	}
	if rep, err := s.MergeWithSetup(ctx, actor, "source", "sheet", preview.RawHash, "", setupConfig()); err != nil || len(rep.Errors) != 0 || rep.Created != (Counts{}) {
		t.Fatalf("repeat: %+v %v", rep, err)
	}
	rd, err = s.raids.Get(ctx, mapping.RaidID)
	if err != nil || rd.Notes != "Manual raid changes" {
		t.Fatalf("overwritten: %+v %v", rd, err)
	}
}

func TestSetupWriteFailureRollsBackHistoryAndSeries(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	data.Setup = setupCSV()
	s.fetcher = fakeFetcher{data: data}
	preview, err := s.DryRunWithSetup(ctx, "source", "sheet", setupConfig())
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err := s.imports.db.ExecContext(ctx, "CREATE TRIGGER reject_setup BEFORE INSERT ON legacy_setup_mappings BEGIN SELECT RAISE(ABORT, 'injected setup failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MergeWithSetup(ctx, actor, "source", "sheet", preview.RawHash, "", setupConfig()); err == nil {
		t.Fatal("expected injected failure")
	}
	for _, table := range []string{"raids", "raid_series", "raid_attendance", "loot_awards", "enhance_point_transactions", "token_transactions", "legacy_imports", "legacy_setup_mappings"} {
		var count int
		if err := s.imports.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback %s count %d: %v", table, count, err)
		}
	}
}

func TestOlderImportedHistoryArchivalIsAuditedWithoutNewRewards(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	raidID := seedImportedRaid(t, s, actor)
	rd, err := s.raids.Get(ctx, raidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.ArchivedAt = nil // Simulate a pre-ADR029 historical import.
	if err := s.raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	before, err := s.imports.pointTransactionCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	setLoot(s, stableHeader+stableDrop+"a\n", "archive-classification")
	preview, err := s.DryRun(ctx, "source", "sheet")
	if err != nil || len(preview.Errors) != 0 || preview.ArchivedRaids != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", preview.RawHash, "")
	if err != nil || len(rep.Errors) == 0 || rep.ArchivedRaids != 0 {
		t.Fatalf("missing reason: %+v %v", rep, err)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", preview.RawHash, "Classify imported history")
	if err != nil || len(rep.Errors) != 0 || rep.ArchivedRaids != 1 || rep.Created != (Counts{}) {
		t.Fatalf("archive: %+v %v", rep, err)
	}
	after, err := s.imports.pointTransactionCount(ctx)
	if err != nil || before != after {
		t.Fatalf("recreated rewards: %d -> %d, %v", before, after, err)
	}
	var count int
	if err := s.imports.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'import_archive' AND reason = 'Classify imported history'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count %d: %v", count, err)
	}
	rd, err = s.raids.Get(ctx, raidID)
	if err != nil || rd.ArchivedAt == nil || rd.SeriesID != nil {
		t.Fatalf("history archival: %+v %v", rd, err)
	}
}
