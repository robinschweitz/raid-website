package legacyimport

import (
	"context"
	"strings"
	"testing"

	"raid-website/internal/loot"
)

// The sheet's Priority column is the only record of how an item was awarded, so every
// value must map to a recorded award method instead of collapsing into main-spec loot.
func TestSheetPriorityMapsToAwardMethod(t *testing.T) {
	cases := []struct {
		priority  string
		item      string
		want      loot.Method
		wantMain  bool
		wantToken bool
		warn      bool
	}{
		{priority: "Main", item: "Blutfall", want: loot.Main, wantMain: true},
		{priority: "Second", item: "Blutfall", want: loot.Second},
		{priority: "Enhanced", item: "Blutfall", want: loot.Enhanced, wantMain: true},
		{priority: "BoE", item: "Blutfall", want: loot.BOE},
		{priority: "Token", item: "Weiheabzeichen des Eroberers", want: loot.Second, wantToken: true},
		// An armor token stays eligible for +10 EP even when the sheet recorded Main.
		{priority: "Main", item: "Weiheabzeichen des Eroberers", want: loot.Second, wantToken: true, warn: true},
		{priority: "", item: "Blutfall", want: loot.Second},
		{priority: "Sonderpreis", item: "Blutfall", want: loot.Second, warn: true},
		// A Token priority on a real item derives no token; token visibility follows items.
		{priority: "Token", item: "Blutfall", want: loot.Second, warn: true},
	}
	for _, c := range cases {
		rows := []LootArchiveRow{{RaidID: "ICC25-01", TimestampRaw: "09.02.2026", Player: "Amisu", Loot: c.item, Priority: c.priority, ItemID: "50178"}}
		plan := BuildPlan(nil, rows, nil, nil)
		if len(plan.Loot) != 1 {
			t.Fatalf("priority %q: loot plans %d", c.priority, len(plan.Loot))
		}
		got := plan.Loot[0]
		if got.Method != c.want {
			t.Fatalf("priority %q: method %q want %q", c.priority, got.Method, c.want)
		}
		if got.IsMainSpec() != c.wantMain {
			t.Fatalf("priority %q: main spec %v want %v", c.priority, got.IsMainSpec(), c.wantMain)
		}
		if got.IsToken != c.wantToken {
			t.Fatalf("priority %q: token %v want %v", c.priority, got.IsToken, c.wantToken)
		}
		warned := strings.Contains(strings.Join(plan.Warnings, "\n"), "priority")
		if warned != c.warn {
			t.Fatalf("priority %q: warnings %v want %v (%v)", c.priority, warned, c.warn, plan.Warnings)
		}
	}
}

func TestSheetRaidKeyDecodesCatalogContext(t *testing.T) {
	cases := []struct {
		key     string
		want    bool
		inst    string
		raidSze int
	}{
		{key: "ICC25-01", want: true, inst: "631", raidSze: 25},
		{key: "ICC10-07", want: true, inst: "631", raidSze: 10},
		{key: "ICC45-01"},
		{key: "LEGACY-LOOT"},
		{key: "ICC25-1"},
		{key: ""},
	}
	for _, c := range cases {
		inst, size, ok := catalogContext(c.key)
		if ok != c.want || inst != c.inst || size != c.raidSze {
			t.Fatalf("key %q: got %q/%d/%v want %q/%d/%v", c.key, inst, size, ok, c.inst, c.raidSze, c.want)
		}
	}
}

func TestImportedRaidCarriesCatalogContext(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	hash := sampleSheet().Hash
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", hash, "")
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("merge: %+v %v", rep, err)
	}
	mapping, err := s.imports.raidMapping(ctx, "source", "sheet", "ICC25-01")
	if err != nil {
		t.Fatal(err)
	}
	rd, err := s.raids.Get(ctx, mapping.RaidID)
	if err != nil {
		t.Fatal(err)
	}
	if rd.CatalogInstanceID == nil || *rd.CatalogInstanceID != "631" {
		t.Fatalf("catalog instance: %+v", rd.CatalogInstanceID)
	}
	if rd.RaidSize == nil || *rd.RaidSize != 25 {
		t.Fatalf("raid size: %+v", rd.RaidSize)
	}
}

// A loot item that belongs to exactly one bundled boss carries that boss into the
// import; an item that proves nothing keeps an empty boss and is reported.
func TestImportedBossDerivedOnlyFromUnambiguousCatalogItem(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	unique := "ICC25-02,09.02.2026,Amisu,Blutfall,Second,50178,a\n"
	ambiguous := "ICC25-02,09.02.2026,Amisu,Weiheabzeichen des Eroberers,Token,52027,b\n"
	unknown := "ICC25-02,09.02.2026,Amisu,Nicht im Katalog,Second,999999,c\n"
	setLoot(s, stableHeader+unique+ambiguous+unknown, "first")
	rep, err := s.DryRun(ctx, "source", "sheet")
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("preview: %+v %v", rep, err)
	}
	warnings := strings.Join(rep.Warnings, "\n")
	if !strings.Contains(warnings, "52027: drops from several bundled bosses") {
		t.Fatalf("missing ambiguity warning: %s", warnings)
	}
	if !strings.Contains(warnings, "999999: not in the bundled catalog") {
		t.Fatalf("missing catalog warning: %s", warnings)
	}
	if !strings.Contains(warnings, "catalog boss derived for 1 loot rows") {
		t.Fatalf("missing derived boss count: %s", warnings)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", "first", "")
	if err != nil || len(rep.Errors) > 0 || rep.Created.Loot != 3 {
		t.Fatalf("merge: %+v %v", rep, err)
	}
	var rows []struct {
		ItemID        string
		Boss          string
		CatalogBossID *int64
	}
	if err := s.imports.db.NewSelect().TableExpr("loot_awards").Column("item_id", "boss", "catalog_boss_id").Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("awards: %+v", rows)
	}
	for _, row := range rows {
		switch row.ItemID {
		case "50178":
			if row.Boss == "" || row.CatalogBossID == nil {
				t.Fatalf("unique item without boss: %+v", row)
			}
		default:
			if row.Boss != "" || row.CatalogBossID != nil {
				t.Fatalf("unproven boss derived: %+v", row)
			}
		}
	}
}

// An enhanced row from the sheet keeps its method and never charges EP, but it does
// count as main-spec loot for the archive reward.
func TestImportedEnhancedAwardCostsNothingAndBlocksMainSpecEP(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	drop := "ICC25-01,09.02.2026,Amisu,Blutfall,Enhanced,50178,\n"
	setLoot(s, stableHeader+drop, "first")
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", "first", "")
	if err != nil || len(rep.Errors) > 0 || rep.Created.Loot != 1 {
		t.Fatalf("merge: %+v %v", rep, err)
	}
	var rows []struct {
		RaidID       int64
		AwardMethod  string
		IsMainSpec   bool
		EnhancedCost *int
	}
	if err := s.imports.db.NewSelect().TableExpr("loot_awards").Column("raid_id", "award_method", "is_main_spec", "enhanced_cost").Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("awards: %+v", rows)
	}
	if rows[0].AwardMethod != string(loot.Enhanced) || !rows[0].IsMainSpec {
		t.Fatalf("enhanced award: %+v", rows[0])
	}
	if rows[0].EnhancedCost != nil && *rows[0].EnhancedCost != 0 {
		t.Fatalf("enhanced cost: %+v", rows[0].EnhancedCost)
	}
}

// The same rule applies when the sheet adds an enhanced row to an already imported
// raid: the audited correction records the method and never charges the enhanced cost.
// Only the documented eligibility difference is appended.
func TestImportedEnhancedAdditionToExistingRaidKeepsZeroCost(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	raidID := seedImportedRaid(t, s, actor)
	amisu, err := s.players.FindByName(ctx, "Amisu")
	if err != nil {
		t.Fatal(err)
	}
	enhanced := "ICC25-01,09.07.2026,Amisu,Blutfall,Enhanced,50178,b\n"
	setLoot(s, stableHeader+stableDrop+"a\n"+enhanced, "second")
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", "second", "historical enhanced award")
	if err != nil || len(rep.Errors) > 0 || rep.Created.Loot != 1 {
		t.Fatalf("merge: %+v %v", rep, err)
	}
	awards, err := s.loot.ListByRaid(ctx, raidID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, award := range awards {
		if award.AwardMethod != loot.Enhanced {
			continue
		}
		found = true
		if !award.IsMainSpec {
			t.Fatalf("enhanced award not counted as main-spec loot: %+v", award)
		}
		if award.EnhancedCost != 0 {
			t.Fatalf("enhanced cost: %+v", award.EnhancedCost)
		}
	}
	if !found {
		t.Fatalf("enhanced award missing: %+v", awards)
	}
	txns, err := s.points.ListByPlayer(ctx, amisu.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, txn := range txns {
		if txn.Type == "enhanced_loot" {
			t.Fatalf("imported enhanced award charged EP: %+v", txn)
		}
	}
}
