package lootcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/uptrace/bun"
	"raid-website/internal/db"
	"raid-website/internal/migrations"
	catalogdata "raid-website/scripts/data"
)

func setup(t *testing.T) (*Service, *bun.DB) {
	t.Helper()
	ctx := context.Background()
	b, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	m, err := migrations.NewMigrator(b.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return NewService(NewRepo(b.DB)), b.DB
}

func exec(t *testing.T, b *bun.DB, query string, args ...any) {
	t.Helper()
	if _, err := b.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, b *bun.DB, query string) int {
	t.Helper()
	var result int
	if err := b.NewRaw(query).Scan(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestColdStartAndIdempotentRestart(t *testing.T) {
	s, b := setup(t)
	ctx := context.Background()
	before, err := s.List(ctx)
	if err != nil || len(before.Instances) != 1 || len(before.Drops) == 0 {
		t.Fatalf("automatic source: %+v %v", before.Sources, err)
	}
	report, err := s.EnsureBundled(ctx)
	if err != nil || report.Inserted != len(before.Instances)+len(before.Bosses)+len(before.Items)+len(before.Drops) || report.Updated != 0 {
		t.Fatalf("initialization: %+v %v", report, err)
	}
	restarted := NewService(NewRepo(b))
	repeat, err := restarted.EnsureBundled(ctx)
	if err != nil || repeat.Inserted != 0 || repeat.Updated != 0 || repeat.Retained != 0 || repeat.Unchanged != report.Inserted {
		t.Fatalf("restart: %+v %v", repeat, err)
	}
	after, err := restarted.List(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("restart changed source")
	}
	if count(t, b, "SELECT COUNT(*) FROM loot_catalog_initializations") != 2 {
		t.Fatal("startup report missing")
	}
	if count(t, b, "SELECT COUNT(*) FROM loot_catalog_sources") != 0 {
		t.Fatal("manual source history was written")
	}
}

func TestLegacyConflictsReportedAndHistoryRetained(t *testing.T) {
	s, b := setup(t)
	ctx := context.Background()
	c, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Sources[0]
	exec(t, b, "INSERT INTO loot_catalog_sources VALUES (?, ?, ?, ?)", p.Source.ID, p.Source.Revision, "old-different-hash", `{"old":"unchanged"}`)
	exec(t, b, "INSERT INTO loot_catalog_instances VALUES ('631', 'Old ICC', 'custom')")
	exec(t, b, "INSERT INTO loot_catalog_instances VALUES ('999999', 'Custom extra', 'custom')")
	exec(t, b, "INSERT INTO loot_catalog_bosses VALUES ('36612', '631', 'Old boss', 'custom')")
	exec(t, b, "INSERT INTO loot_catalog_items (item_id,name,source_id) VALUES ('50759','Old item','custom')")
	exec(t, b, "INSERT INTO loot_catalog_drops VALUES ('36612','50759',10,'manual-old-path',1,'custom')")
	exec(t, b, "INSERT INTO players (id,name,created_at,updated_at) VALUES (1,'Historical player','2020-01-01','2020-01-01')")
	exec(t, b, "INSERT INTO raids (id,name,raid_leader_id,status,catalog_instance_id,created_at,updated_at) VALUES (1,'History',1,'COMPLETED','999999','2020-01-01','2020-01-01')")
	exec(t, b, "INSERT INTO loot_awards (id,raid_id,boss,item_id,item_name,awarded_to_player_id,awarded_by_player_id,source,catalog_boss_id,awarded_at,created_at,updated_at) VALUES (1,1,'Historical snapshot','50759','Historical item',1,1,'manual','36612','2020-01-01','2020-01-01','2020-01-01')")
	exec(t, b, "INSERT INTO enhance_point_transactions (player_id,raid_id,amount,type,reason,actor_id,created_at) VALUES (1,1,91,'ADJUSTMENT','opening',1,'2020-01-01')")
	report, err := s.EnsureBundled(ctx)
	if err != nil || report.Updated != 3 || report.Retained != 2 {
		t.Fatalf("legacy initialization: updates=%d retained=%d err=%v", report.Updated, report.Retained, err)
	}
	var stored string
	if err := b.NewRaw("SELECT report FROM loot_catalog_initializations ORDER BY id DESC LIMIT 1").Scan(ctx, &stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, "Old ICC") || !strings.Contains(stored, "Historical") && !strings.Contains(stored, "Custom extra") || !strings.Contains(stored, report.SourceHash) {
		t.Fatal("missing detailed before/after report")
	}
	if count(t, b, "SELECT COUNT(*) FROM loot_catalog_sources WHERE raw_hash='old-different-hash' AND metadata='{\"old\":\"unchanged\"}'") != 1 {
		t.Fatal("source history overwritten")
	}
	if count(t, b, "SELECT COUNT(*) FROM raids WHERE name='History' AND catalog_instance_id='999999' AND status='COMPLETED'") != 1 || count(t, b, "SELECT COUNT(*) FROM loot_awards WHERE boss='Historical snapshot' AND item_name='Historical item'") != 1 || count(t, b, "SELECT SUM(amount) FROM enhance_point_transactions") != 91 {
		t.Fatal("guild history changed")
	}
	extra := "999999"
	if !errors.Is(s.ValidateAssociation(ctx, &extra, nil), ErrInvalid) {
		t.Fatal("DB-only instance selectable")
	}
	items, err := s.Items(ctx, Filter{BossID: "36612", RaidSize: 10, Mode: ModeNormal})
	if err != nil || len(items) != 12 {
		t.Fatal("old drop affected authoritative catalog")
	}
}

func TestMirrorRollbackAndConcurrentInitialization(t *testing.T) {
	s, b := setup(t)
	exec(t, b, "CREATE TRIGGER reject_catalog_item BEFORE INSERT ON loot_catalog_items BEGIN SELECT RAISE(ABORT, 'fixture mirror failure'); END")
	if _, err := s.EnsureBundled(context.Background()); err == nil {
		t.Fatal("mirror failure ignored")
	}
	for _, query := range []string{"SELECT COUNT(*) FROM loot_catalog_instances", "SELECT COUNT(*) FROM loot_catalog_bosses", "SELECT COUNT(*) FROM loot_catalog_items", "SELECT COUNT(*) FROM loot_catalog_initializations"} {
		if count(t, b, query) != 0 {
			t.Fatal("partial mirror/report committed")
		}
	}
	exec(t, b, "DROP TRIGGER reject_catalog_item")
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.EnsureBundled(context.Background()); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count(t, b, "SELECT COUNT(*) FROM loot_catalog_initializations") != 4 {
		t.Fatal("concurrent report missing")
	}
}

func TestGenuineMarrowgarModesAndProvenance(t *testing.T) {
	s := NewService(NewRepo(nil))
	ctx := context.Background()
	for _, tc := range []struct {
		size  int
		mode  Mode
		count int
	}{{10, ModeNormal, 12}, {10, ModeHeroic, 12}, {25, ModeNormal, 17}, {25, ModeHeroic, 17}, {0, ModeNormal, 29}, {0, ModeHeroic, 29}, {0, "", 58}} {
		items, err := s.Items(ctx, Filter{BossID: "36612", RaidSize: tc.size, Mode: tc.mode})
		if err != nil || len(items) != tc.count {
			t.Fatalf("size=%d mode=%s count=%d want=%d: %v", tc.size, tc.mode, len(items), tc.count, err)
		}
		seen := map[string]bool{}
		for _, item := range items {
			key := fmt.Sprintf("%s/%s/%d", item.ItemID, item.Mode, item.RaidSize)
			if seen[key] || item.Conditional || len(item.SourceVariants) == 0 {
				t.Fatalf("invalid selector: %+v", item)
			}
			for _, variant := range item.SourceVariants {
				if variant.LootMode != 1 || variant.Difficulty == "" {
					t.Fatalf("invalid source variant: %+v", item)
				}
			}
			seen[key] = true
		}
	}
	items, _ := s.Items(ctx, Filter{BossID: "36612", RaidSize: 10, Mode: ModeNormal})
	direct := false
	for _, item := range items {
		for _, variant := range item.SourceVariants {
			if strings.HasSuffix(variant.Difficulty, "source-lootmode-path:1") {
				direct = true
			}
		}
	}
	if !direct {
		t.Fatal("direct provenance lost")
	}
	for _, mode := range []Mode{"UNKNOWN", "heroic", "2", "NORMAL/HEROIC"} {
		if _, err := s.Items(ctx, Filter{Mode: mode}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted mode %s", mode)
		}
	}
}

func TestSourceModesNeverInferHeroicFromLootMask(t *testing.T) {
	for _, tc := range []struct {
		drop        Drop
		mode        Mode
		conditional bool
		invalid     bool
	}{
		{Drop{Difficulty: "raid-difficulty:0/source-lootmode-path:1/2", LootMode: 2}, ModeNormal, true, false},
		{Drop{Difficulty: "raid-difficulty:2/source-lootmode-path:1", LootMode: 1}, ModeHeroic, false, false},
		{Drop{Difficulty: "raid-difficulty:3/source-creature:35268 / source-lootmode-path:1", LootMode: 1}, ModeHeroic, false, false},
		{Drop{Difficulty: "raid-difficulty:1/source-creature:29417 / source-lootmode-path:1/1", LootMode: 1}, ModeNormal, false, false},
		{Drop{Difficulty: "raid-difficulty:0/thorim/hardmode:true/source-chest:194313 / source-lootmode-path:1/1", LootMode: 1}, ModeHardmode, true, false},
		{Drop{Difficulty: "raid-difficulty:0/thorim/hardmode:false/source-chest:194312 / source-lootmode-path:1", LootMode: 1}, ModeNormal, false, false},
		{Drop{Difficulty: "raid-difficulty:0/phase:2/source-lootmode-path:1", LootMode: 1}, ModeNormal, true, false},
		{Drop{Difficulty: "raid-difficulty:9/source-lootmode-path:1", LootMode: 1}, "", false, true},
		{Drop{Difficulty: "unknown", LootMode: 1}, "", false, true},
	} {
		mode, conditional, err := sourceGameMode(tc.drop)
		if tc.invalid {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted %+v", tc.drop)
			}
			continue
		}
		if err != nil || mode != tc.mode || conditional != tc.conditional {
			t.Fatalf("%+v: %s %t %v", tc.drop, mode, conditional, err)
		}
	}
}

// The localized ICC source has normal and heroic modes but no hardmode entries.
func TestLocalizedICCHasNoHardmodeSelections(t *testing.T) {
	s := NewService(NewRepo(nil))
	ctx := context.Background()
	catalog, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, boss := range catalog.Bosses {
		normal, err := s.Items(ctx, Filter{BossID: boss.ID, RaidSize: 10, Mode: ModeNormal})
		if err != nil {
			t.Fatal(err)
		}
		hardmode, err := s.Items(ctx, Filter{BossID: boss.ID, RaidSize: 10, Mode: ModeHardmode})
		if err != nil {
			t.Fatal(err)
		}
		if len(normal) == 0 || len(hardmode) != 0 {
			t.Fatalf("boss %s: normal=%d hardmode=%d", boss.ID, len(normal), len(hardmode))
		}
	}
}

// Boss derivation from an item is only allowed when exactly one boss drops it, so the
// lookup must report every candidate instead of picking one.
func TestBossesByItemReportsEveryCandidate(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()
	if _, err := svc.EnsureBundled(ctx); err != nil {
		t.Fatal(err)
	}
	byItem, err := svc.BossesByItem(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byItem["not-an-item"]; ok {
		t.Fatal("unknown item reported bosses")
	}
	ambiguous := 0
	for itemID, bosses := range byItem {
		if len(bosses) == 0 {
			t.Fatalf("item %s without boss", itemID)
		}
		for i := 1; i < len(bosses); i++ {
			if bosses[i-1].ID >= bosses[i].ID {
				t.Fatalf("item %s bosses not sorted: %+v", itemID, bosses)
			}
		}
		if len(bosses) > 1 {
			ambiguous++
		}
	}
	// The armor tokens drop from several bosses and must stay ambiguous.
	for _, itemID := range []string{"52025", "52026", "52027"} {
		if len(byItem[itemID]) < 2 {
			t.Fatalf("token %s: %d bosses", itemID, len(byItem[itemID]))
		}
	}
	for _, itemID := range []string{"50178"} {
		if len(byItem[itemID]) != 1 {
			t.Fatalf("item %s: %d bosses", itemID, len(byItem[itemID]))
		}
	}
	if ambiguous == 0 {
		t.Fatal("no ambiguous item reported")
	}
}

func TestBundleCopiesAndInvalidSource(t *testing.T) {
	s := NewService(NewRepo(nil))
	original, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := json.Marshal(original)
	original.Sources[0].Notices[0] = "mutated"
	original.Instances[0].Name = "mutated"
	original.Bosses[0].Name = "mutated"
	original.Items[0].Name = "mutated"
	*original.Items[0].Binding = 999
	*original.Items[0].Quality = 999
	original.Drops[0].Difficulty = "mutated"
	items, _ := s.Items(context.Background(), Filter{BossID: "36612"})
	*items[0].Binding = 999
	items[0].SourceVariants[0].Difficulty = "mutated"
	resolvedBoss, resolvedItem, err := s.ValidateDrop(context.Background(), "631", nil, &items[0].BossID, items[0].ItemID)
	if err != nil || resolvedBoss.ID != "36612" {
		t.Fatal(err)
	}
	*resolvedItem.Binding = 999
	after, _ := s.List(context.Background())
	actual, _ := json.Marshal(after)
	if string(actual) != string(expected) {
		t.Fatal("caller mutated shared bundle")
	}
	for _, raw := range []string{"{}", "invalid", strings.Replace(catalogdata.JSON(), "\"10_normal\"", "\"invalid_mode\"", 1)} {
		if _, err := decodeBundled(raw); err == nil {
			t.Fatal("invalid bundle accepted")
		}
	}
}
