package legacyimport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMixedIdentitiesReorderAndStableAppend(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	setLoot(s, stableHeader+stableDrop+"a\n"+stableDrop+"\n"+stableDrop+"\n", "first")
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", "first", "")
	if err != nil || rep.Created.Loot != 3 {
		t.Fatalf("initial: %+v %v", rep, err)
	}
	setLoot(s, stableHeader+stableDrop+"\n"+stableDrop+"\n"+stableDrop+"a\n", "reordered")
	preview := previewImport(t, s, "source", "sheet")
	if preview.Created != (Counts{}) {
		t.Fatalf("reordered preview: %+v", preview)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", "reordered", "")
	if err != nil || rep.Created != (Counts{}) {
		t.Fatalf("reordered: %+v %v", rep, err)
	}
	// Prepending an explicit ID with identical content must not shift no-ID occurrences.
	setLoot(s, stableHeader+stableDrop+"b\n"+stableDrop+"\n"+stableDrop+"a\n"+stableDrop+"\n", "appended")
	preview = previewImport(t, s, "source", "sheet")
	if preview.Created != (Counts{Loot: 1}) {
		t.Fatalf("append preview: %+v", preview)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", "appended", "additional historical drop")
	if err != nil || rep.Created != (Counts{Loot: 1}) {
		t.Fatalf("append: %+v %v", rep, err)
	}
	mappings, err := s.imports.mappings(ctx, "source", "sheet")
	if err != nil || len(mappings) != 4 {
		t.Fatalf("mappings: %+v %v", mappings, err)
	}
}

func TestPreviewPersistsOnlyMetadata(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	previewImport(t, s, "source", "sheet")
	if has, err := s.imports.hasPreview(ctx, "source", "sheet", sampleSheet().Hash); err != nil || !has {
		t.Fatalf("preview evidence: %v %v", has, err)
	}
	if _, err := s.players.FindByName(ctx, "Nylama"); err == nil {
		t.Fatal("preview created domain player")
	}
	if rows, err := s.List(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("imports: %+v %v", rows, err)
	}
	if rows, err := s.imports.mappings(ctx, "source", "sheet"); err != nil || len(rows) != 0 {
		t.Fatalf("mappings: %+v %v", rows, err)
	}
	// Evidence survives service recreation and has no invented expiry.
	restarted := *s
	restarted.imports = NewRepo(s.imports.db)
	if rep, err := restarted.Merge(ctx, actor, "source", "sheet", sampleSheet().Hash, ""); err != nil || rep.Created.Loot != 2 {
		t.Fatalf("persistent preview: %+v %v", rep, err)
	}
}

func TestInvalidPreviewDoesNotAuthorize(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	setLoot(s, stableHeader+stableDrop+"a\n"+stableDrop+"a\n", "duplicate")
	preview, err := s.DryRun(ctx, "source", "sheet")
	if err != nil || len(preview.Errors) == 0 {
		t.Fatalf("invalid preview: %+v %v", preview, err)
	}
	if has, err := s.imports.hasPreview(ctx, "source", "sheet", "duplicate"); err != nil || has {
		t.Fatalf("invalid evidence: %v %v", has, err)
	}
	if _, err := s.Merge(ctx, actor, "source", "sheet", "duplicate", ""); !errors.Is(err, ErrPreviewRequired) {
		t.Fatal(err)
	}
}

func TestMergeRevalidatesSuccessfulPreview(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	previewImport(t, s, "source", "sheet")
	// Simulate newly discovered legacy provenance after a successful review.
	if err := s.imports.Insert(ctx, &Import{ActorID: actor, Source: "source", SourceURL: "sheet", RawHash: "older", Status: "MERGED"}); err != nil {
		t.Fatal(err)
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", sampleSheet().Hash, "")
	if err != nil || len(rep.Errors) == 0 || rep.Created != (Counts{}) {
		t.Fatalf("revalidation: %+v %v", rep, err)
	}
}

func TestSheetDatesAndAwardTimes(t *testing.T) {
	for _, tc := range []struct{ date, utc string }{
		{"09.02.2026", "2026-02-08T23:00:00Z"},
		{"09.07.2026", "2026-07-08T22:00:00Z"},
	} {
		t.Run(tc.date, func(t *testing.T) {
			parsed, ok := parseSheetDate(tc.date)
			if !ok || parsed.Format(time.RFC3339) != tc.utc || parsed.Location() != time.UTC {
				t.Fatalf("date: %v %v", parsed, ok)
			}
			s, actor := importFixture(t)
			setLoot(s, stableHeader+strings.ReplaceAll(stableDrop, "09.02.2026", tc.date)+"a\n", "dated")
			previewImport(t, s, "source", "sheet")
			if rep, err := s.Merge(context.Background(), actor, "source", "sheet", "dated", ""); err != nil || rep.Created.Loot != 1 {
				t.Fatalf("merge: %+v %v", rep, err)
			}
			raid, err := s.raids.FindByName(context.Background(), "ICC25-01")
			if err != nil {
				t.Fatal(err)
			}
			awards, err := s.loot.ListByRaid(context.Background(), raid.ID)
			if err != nil || len(awards) != 1 || awards[0].AwardedAt.UTC().Format(time.RFC3339) != tc.utc {
				t.Fatalf("award time: %+v %v", awards, err)
			}
			if raid.ScheduledStart == nil || raid.ScheduledStart.UTC().Format(time.RFC3339) != "2026-02-08T23:00:00Z" {
				t.Fatalf("raid time: %+v", raid)
			}
		})
	}
}

func TestMissingAndInvalidLootDatesWarnAndFallback(t *testing.T) {
	for _, date := range []string{"", "31.02.2026", "not-a-date"} {
		name := date
		if name == "" {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			s, actor := importFixture(t)
			setLoot(s, stableHeader+strings.ReplaceAll(stableDrop, "09.02.2026", date)+"a\n", "fallback")
			preview := previewImport(t, s, "source", "sheet")
			if len(preview.Warnings) == 0 || !strings.Contains(strings.Join(preview.Warnings, "\n"), "using import time for award") {
				t.Fatalf("warnings: %+v", preview)
			}
			// SQLite/Bun stores timestamps at microsecond precision.
			before := time.Now().UTC().Truncate(time.Microsecond)
			rep, err := s.Merge(context.Background(), actor, "source", "sheet", "fallback", "")
			after := time.Now().UTC()
			if err != nil || rep.Created.Loot != 1 {
				t.Fatalf("merge: %+v %v", rep, err)
			}
			raid, err := s.raids.FindByName(context.Background(), "ICC25-01")
			if err != nil {
				t.Fatal(err)
			}
			awards, err := s.loot.ListByRaid(context.Background(), raid.ID)
			if err != nil || len(awards) != 1 {
				t.Fatalf("awards: %+v %v", awards, err)
			}
			if awards[0].AwardedAt.Before(before) || awards[0].AwardedAt.After(after) {
				t.Fatalf("fallback time: %v", awards[0].AwardedAt)
			}
		})
	}
}
