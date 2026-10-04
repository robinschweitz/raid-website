package legacyimport

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/tokens"
)

const raidCSV = `"Raid ID","Timestamp","Position","Charakter","Rolle","Klasse","Spec","Spalte 1"
"ICC25-01","09.02.2026","1","Nylama","Tank","Todesritter","Blut (Tank)",""
"ICC25-01","09.02.2026","2","Amisu","Heiler","Priester","Disziplin",""
`

const lootCSV = `"Raid ID","Timestamp","Spieler","Loot","Priority","ItemId"
"ICC25-01","09.02.2026","Nylama","Blutfall","Main","50178"
"ICC25-01","09.02.2026","Amisu","Weiheabzeichen des Eroberers","Token","52027"
`

const charCSV = `"Charakter","Spieler","Klasse","Main Spec","Second Spec","",""
"Nylama","Nylama","Todesritter","","","",""
"Amisu","Amisu","Priester","Disziplin","","",""
`

const pointsCSV = `"Spieler","Punkte","Inaktiv"
"Nylama","170","Nein"
"Amisu","30",""
`

type fakeFetcher struct {
	data SheetData
	err  error
}

func (f fakeFetcher) Fetch(_ context.Context, _ string) (SheetData, error) {
	return f.data, f.err
}

func sampleSheet() SheetData {
	return SheetData{
		Raid:      []byte(raidCSV),
		Loot:      []byte(lootCSV),
		Character: []byte(charCSV),
		Points:    []byte(pointsCSV),
		Hash:      "sample-hash-1",
	}
}

func setup(t *testing.T) (*bun.DB, *Service, *player.Repo, *enhancepoints.Repo, *tokens.Repo, *loot.Repo, *raid.Repo) {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "imp.db") + "?_pragma=foreign_keys(1)"
	bdb, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = bdb.Close() })
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	players := player.NewRepo(bdb.DB)
	chars := character.NewRepo(bdb.DB)
	raids := raid.NewRepo(bdb.DB)
	att := attendance.NewRepo(bdb.DB)
	lootRepo := loot.NewRepo(bdb.DB)
	tokenRepo := tokens.NewRepo(bdb.DB)
	points := enhancepoints.NewRepo(bdb.DB)
	imports := NewRepo(bdb.DB)

	svc := NewService(imports, players, chars, raids, att, lootRepo, points, tokenRepo, fakeFetcher{data: sampleSheet()})
	return bdb.DB, svc, players, points, tokenRepo, lootRepo, raids
}

func TestBuildPlan(t *testing.T) {
	plan, rows, err := planFromSheet(sampleSheet())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if rows != 8 {
		t.Fatalf("rows: want 8 got %d", rows)
	}
	if len(plan.Players) != 2 {
		t.Fatalf("players: want 2 got %d", len(plan.Players))
	}
	if len(plan.Characters) != 2 {
		t.Fatalf("characters: want 2 got %d", len(plan.Characters))
	}
	if len(plan.Raids) != 1 {
		t.Fatalf("raids: want 1 got %d", len(plan.Raids))
	}
	if len(plan.Raids[0].Entries) != 2 {
		t.Fatalf("attendance: want 2 got %d", len(plan.Raids[0].Entries))
	}
	if len(plan.Loot) != 2 {
		t.Fatalf("loot: want 2 got %d", len(plan.Loot))
	}
	tokensPlanned := 0
	for _, l := range plan.Loot {
		if l.IsToken {
			tokensPlanned++
		}
	}
	if tokensPlanned != 1 {
		t.Fatalf("tokens: want 1 got %d", tokensPlanned)
	}
}

func TestMergeThenIdempotent(t *testing.T) {
	ctx := context.Background()
	bdb, svc, players, points, tokenRepo, lootRepo, raids := setup(t)

	now := time.Now().UTC()
	actor := &player.Player{Name: "Importer", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := players.Create(ctx, actor); err != nil {
		t.Fatalf("create actor: %v", err)
	}

	previewImport(t, svc, "google_sheets", "sheet")
	rep, err := svc.Merge(ctx, actor.ID, "google_sheets", "sheet", sampleSheet().Hash, "")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if rep.Created.Players != 2 {
		t.Fatalf("created players: want 2 got %d", rep.Created.Players)
	}
	if rep.Created.Characters != 2 {
		t.Fatalf("created characters: want 2 got %d", rep.Created.Characters)
	}
	if rep.Created.Raids != 1 {
		t.Fatalf("created raids: want 1 got %d", rep.Created.Raids)
	}
	if rep.Created.Attendance != 2 {
		t.Fatalf("created attendance: want 2 got %d", rep.Created.Attendance)
	}
	if rep.Created.Loot != 2 {
		t.Fatalf("created loot: want 2 got %d", rep.Created.Loot)
	}
	if rep.Created.Tokens != 1 {
		t.Fatalf("created tokens: want 1 got %d", rep.Created.Tokens)
	}
	if rep.Created.Points != 2 {
		t.Fatalf("created points: want 2 got %d", rep.Created.Points)
	}

	nylama, err := players.FindByName(ctx, "Nylama")
	if err != nil {
		t.Fatalf("find nylama: %v", err)
	}
	bal, err := points.Balance(ctx, nylama.ID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != 170 {
		t.Fatalf("nylama balance: want 170 got %d", bal)
	}

	amisu, err := players.FindByName(ctx, "Amisu")
	if err != nil {
		t.Fatalf("find amisu: %v", err)
	}
	amisuBal, err := points.Balance(ctx, amisu.ID)
	if err != nil {
		t.Fatalf("balance amisu: %v", err)
	}
	if amisuBal != 30 {
		t.Fatalf("amisu balance: want 30 got %d", amisuBal)
	}

	txns, err := tokenRepo.ListByPlayer(ctx, amisu.ID)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	total := 0
	for _, tx := range txns {
		total += tx.Amount
	}
	if total != 1 {
		t.Fatalf("amisu token total: want 1 got %d", total)
	}

	rd, err := raids.FindByName(ctx, "ICC25-01")
	if err != nil {
		t.Fatalf("find raid: %v", err)
	}
	awards, err := lootRepo.ListByRaid(ctx, rd.ID)
	if err != nil {
		t.Fatalf("list loot: %v", err)
	}
	if len(awards) != 2 {
		t.Fatalf("raid loot: want 2 got %d", len(awards))
	}
	_ = bdb

	// Re-merge identical data must be a no-op.
	rep2, err := svc.Merge(ctx, actor.ID, "google_sheets", "sheet", "", "")
	if err != nil {
		t.Fatalf("re-merge: %v", err)
	}
	if !rep2.Skipped {
		t.Fatalf("re-merge: expected skipped")
	}
	bal2, _ := points.Balance(ctx, nylama.ID)
	if bal2 != 170 {
		t.Fatalf("re-merge changed balance: want 170 got %d", bal2)
	}
}
