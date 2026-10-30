package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

type addonPreviewRow struct {
	LineKey           string             `json:"line_key"`
	ItemID            string             `json:"item_id"`
	ItemName          string             `json:"item_name"`
	State             string             `json:"state"`
	CharacterID       *int64             `json:"character_id"`
	CharacterName     string             `json:"character_name"`
	CatalogBossID     *string            `json:"catalog_boss_id"`
	BossOptions       []lootcatalog.Boss `json:"boss_options"`
	AwardMethod       string             `json:"award_method"`
	Roll              *int               `json:"roll"`
	Spec              string             `json:"spec"`
	RollRange         string             `json:"roll_range"`
	WinnerName        string             `json:"winner_name"`
	EnhancedPoints    int                `json:"enhanced_points"`
	IsUnclaimed       bool               `json:"is_unclaimed"`
	IsImportable      bool               `json:"is_importable"`
	IsAlreadyImported bool               `json:"is_already_imported"`
	Warnings          []string           `json:"warnings"`
	Errors            []string           `json:"errors"`
}

type addonPreview struct {
	HeaderRaidID     int64             `json:"header_raid_id"`
	Rows             []addonPreviewRow `json:"rows"`
	RecipientOptions []struct {
		CharacterID   int64  `json:"character_id"`
		CharacterName string `json:"character_name"`
	} `json:"recipient_options"`
	Warnings []string `json:"warnings"`
}

type addonImportResult struct {
	LineKey       string `json:"line_key"`
	Status        string `json:"status"`
	AwardID       *int64 `json:"award_id"`
	UnclaimedID   *int64 `json:"unclaimed_id"`
	CharacterName string `json:"character_name"`
	Code          string `json:"code"`
}

func (f *correctionFixture) addonPreview(payload string) addonPreview {
	f.t.Helper()
	body := f.request(http.MethodPost, "/addon-loot-import/preview", map[string]any{"payload": payload}, http.StatusOK)
	var out struct{ Data addonPreview }
	if err := json.Unmarshal(body, &out); err != nil {
		f.t.Fatal(err)
	}
	return out.Data
}

func (f *correctionFixture) addonImport(token, payload string, selections []map[string]any) []addonImportResult {
	return f.addonImportStatus(token, payload, selections, http.StatusOK)
}
func (f *correctionFixture) addonImportStatus(token, payload string, selections []map[string]any, want int) []addonImportResult {
	f.t.Helper()
	status, body := do(f.t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/addon-loot-import", f.srv.URL, f.raidID), token, map[string]any{"payload": payload, "selections": selections})
	if status != want {
		f.t.Fatalf("import: %d %s", status, body)
	}
	var out struct{ Data []addonImportResult }
	if err := json.Unmarshal(body, &out); err != nil {
		f.t.Fatal(err)
	}
	return out.Data
}

func (f *correctionFixture) characterName(id int64) string {
	f.t.Helper()
	c, err := character.NewRepo(f.db).Get(context.Background(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return c.Name
}

func TestAddonInviteExportUsesCurrentNamesAndBench(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	benchID := f.attend(f.second, f.secondChar)
	ctx := context.Background()
	bench, err := attendance.NewRepo(f.db).Get(ctx, benchID)
	if err != nil {
		t.Fatal(err)
	}
	bench.IsBench = true
	if err := attendance.NewRepo(f.db).Update(ctx, bench); err != nil {
		t.Fatal(err)
	}
	renamed, err := character.NewRepo(f.db).Get(ctx, f.firstChar)
	if err != nil {
		t.Fatal(err)
	}
	renamed.Name = "Umbenannt"
	if err := character.NewRepo(f.db).Update(ctx, renamed); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		query string
		want  string
	}{
		{"", fmt.Sprintf("RS2;INV;%d;Correction raid;25\nUmbenannt;DD;Magier;0", f.raidID)},
		{"?bench=1", fmt.Sprintf("RS2;INV;%d;Correction raid;25\nUmbenannt;DD;Magier;0\nCorrection1;DD;Magier;1", f.raidID)},
	} {
		status, body := do(t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/addon-invite-export%s", f.srv.URL, f.raidID, tc.query), f.memberToken, nil)
		if status != http.StatusOK {
			t.Fatalf("export %q: %d %s", tc.query, status, body)
		}
		var out struct{ Data struct{ Text string } }
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if out.Data.Text != tc.want {
			t.Fatalf("export %q:\n%s\nwant:\n%s", tc.query, out.Data.Text, tc.want)
		}
	}
}

// The invite list carries the raid's date and start time in the series'
// timezone (realm time), which RaidLedger needs for its calendar event. A raid
// without a series has no known timezone and keeps the short header.
func TestAddonInviteExportCarriesSeriesStartTime(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	ctx := context.Background()
	raids := raid.NewRepo(f.db)
	rd, err := raids.Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.November, 5, 19, 0, 0, 0, time.UTC)
	rd.ScheduledStart = &start
	if err := raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	export := func() string {
		status, body := do(t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/addon-invite-export", f.srv.URL, f.raidID), f.memberToken, nil)
		if status != http.StatusOK {
			t.Fatalf("export: %d %s", status, body)
		}
		var out struct{ Data struct{ Text string } }
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out.Data.Text
	}
	if got, want := export(), fmt.Sprintf("RS2;INV;%d;Correction raid;25\nCorrection0;DD;Magier;0", f.raidID); got != want {
		t.Fatalf("without series:\n%s\nwant:\n%s", got, want)
	}

	series := &raidseries.Series{Name: "Donnerstag", RaidLeaderID: f.actor, Weekdays: []int{4}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
	if err := raidseries.NewService(raidseries.NewRepo(f.db)).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	rd.SeriesID = &series.ID
	if err := raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	if got, want := export(), fmt.Sprintf("RS2;INV;%d;Correction raid;25;2026-11-05;20:00\nCorrection0;DD;Magier;0", f.raidID); got != want {
		t.Fatalf("with series:\n%s\nwant:\n%s", got, want)
	}
}

func TestAddonLootPreviewResolvesBossRecipientAndProblems(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	first := f.characterName(f.firstChar)
	creatureBoss, creatureItem := uniqueCreatureDrop(t, f)
	payload := strings.Join([]string{
		fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000", f.raidID+1),
		fmt.Sprintf("1|1760000100|%s|%s|%s|given", creatureBoss, creatureItem, strings.ToUpper(first)),
		fmt.Sprintf("2|1760000200|-|%s|%s|given", f.tokenItem, first),
		fmt.Sprintf("3|1760000300|-|%s|Unbekannt|given", f.ordinaryItem),
		fmt.Sprintf("4|1760000400|-|%s|-|-", f.ordinaryItem),
		"kaputt",
		"6|1760000500|-|1|" + first + "|given",
	}, "\n")
	preview := f.addonPreview(payload)
	if preview.HeaderRaidID != f.raidID+1 || len(preview.Warnings) != 1 {
		t.Fatalf("raid mismatch must warn: %+v", preview)
	}
	if len(preview.Rows) != 6 {
		t.Fatalf("rows: %+v", preview.Rows)
	}
	ordinary := preview.Rows[0]
	if !ordinary.IsImportable || ordinary.CatalogBossID == nil || *ordinary.CatalogBossID != creatureBoss || ordinary.CharacterID == nil || *ordinary.CharacterID != f.firstChar || ordinary.AwardMethod != "MAIN" || ordinary.ItemName == "" {
		t.Fatalf("ordinary row: %+v", ordinary)
	}
	token := preview.Rows[1]
	if !token.IsImportable || token.AwardMethod != "SECOND" || len(token.BossOptions) < 2 || token.CatalogBossID != nil {
		t.Fatalf("token row must be SECOND with an open boss choice: %+v", token)
	}
	hinted := f.addonPreview(fmt.Sprintf("RS1|LOOT|%d|Recorder|1\n1|2|%s|%s|%s|given", f.raidID, tokenBossCreature(t, token.BossOptions), f.tokenItem, first)).Rows[0]
	if hinted.CatalogBossID == nil || *hinted.CatalogBossID != tokenBossCreature(t, token.BossOptions) {
		t.Fatalf("the killed creature must pick the boss: %+v", hinted)
	}
	// Unknown or missing recipients are suggestions only: staff pick the winner.
	if row := preview.Rows[2]; !row.IsImportable || row.CharacterID != nil || len(row.Errors) != 0 || !strings.Contains(strings.Join(row.Warnings, " "), "nicht in der Aufstellung") {
		t.Fatalf("unknown recipient: %+v", row)
	}
	if row := preview.Rows[3]; !row.IsImportable || row.CharacterID != nil || len(row.Warnings) != 1 {
		t.Fatalf("unassigned drop: %+v", row)
	}
	if row := preview.Rows[4]; row.IsImportable || len(row.Errors) != 1 {
		t.Fatalf("broken line: %+v", row)
	}
	if row := preview.Rows[5]; row.IsImportable || len(row.Errors) != 1 {
		t.Fatalf("item outside the catalog: %+v", row)
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatalf("preview wrote loot: %v %v", awards, err)
	}
}

// RS2 (RaidLedger) adds the roll: the spec and an enhanced range pick the
// default method, a held item suggests its roll winner, and parked or deleted
// lines are shown but never imported.
func TestAddonLootPreviewReadsRS2RollsAndStates(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	f.seedPoints(f.first, 30)
	first, second := f.characterName(f.firstChar), f.characterName(f.secondChar)
	payload := strings.Join([]string{
		fmt.Sprintf("RS2;LOOT;%d;%s;1760000000;1760009000", f.raidID, second),
		fmt.Sprintf("1;1760000100;-;%s;%s;given;87;ms;1-100;%s", f.ordinaryItem, first, first),
		fmt.Sprintf("2;1760000200;-;%s;%s;held;131;ms;31-130;%s", f.ordinaryItem, second, first),
		fmt.Sprintf("3;1760000300;-;%s;%s;given;64;os;1-99;%s", f.ordinaryItem, second, second),
		fmt.Sprintf("4;1760000400;-;%s;%s;parked;-;-;-;-", f.ordinaryItem, second),
		fmt.Sprintf("5;1760000500;-;%s;%s;deleted;12;ms;1-100;%s", f.ordinaryItem, first, first),
		fmt.Sprintf("6;1760000600;-;%s;%s;given;99;ms;21-120;%s", f.ordinaryItem, first, first),
		fmt.Sprintf("7;1760000700;-;%s;%s;disenchanted;-;-;-;-", f.ordinaryItem, second),
		fmt.Sprintf("8;1760000800;-;%s;%s;bank;-;-;-;-", f.ordinaryItem, second),
	}, "\n")
	rows := f.addonPreview(payload).Rows
	if len(rows) != 8 {
		t.Fatalf("rows: %+v", rows)
	}
	if r := rows[0]; !r.IsImportable || r.AwardMethod != "MAIN" || r.Roll == nil || *r.Roll != 87 || r.Spec != "ms" || r.RollRange != "1-100" || r.EnhancedPoints != 0 {
		t.Fatalf("main-spec row: %+v", r)
	}
	if r := rows[1]; !r.IsImportable || r.AwardMethod != "ENHANCED" || r.EnhancedPoints != 30 || r.CharacterID == nil || *r.CharacterID != f.firstChar || !strings.Contains(strings.Join(r.Warnings, " "), "gewonnen von "+first) || strings.Contains(strings.Join(r.Warnings, " "), "laut Konto") {
		t.Fatalf("held row must suggest the winner and ENHANCED: %+v", r)
	}
	if r := rows[2]; !r.IsImportable || r.AwardMethod != "SECOND" || *r.CharacterID != f.secondChar {
		t.Fatalf("off-spec row: %+v", r)
	}
	for _, r := range rows[3:5] {
		if r.IsImportable || r.IsUnclaimed || r.CharacterID != nil || len(r.Errors) != 0 || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "nicht übernommen") {
			t.Fatalf("parked and deleted rows are shown only: %+v", r)
		}
	}
	// Disenchanted and banked items went to nobody: imported as unclaimed loot (ADR 089).
	for _, r := range rows[6:8] {
		if !r.IsImportable || !r.IsUnclaimed || r.CharacterID != nil || len(r.Errors) != 0 || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "Beute ohne Empfänger") {
			t.Fatalf("disenchanted and banked rows are unclaimed loot: %+v", r)
		}
	}
	if r := rows[5]; !r.IsImportable || r.EnhancedPoints != 20 || !strings.Contains(strings.Join(r.Warnings, " "), "laut Konto sind es 30") {
		t.Fatalf("an enhanced roll that disagrees with the balance must warn: %+v", r)
	}

	results := f.addonImport(f.token, payload, []map[string]any{
		{"difficulty": "NORMAL", "line_key": rows[1].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": rows[1].AwardMethod},
		{"difficulty": "NORMAL", "line_key": rows[6].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"},
		{"difficulty": "NORMAL", "line_key": rows[7].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"},
	})
	if len(results) != 3 || results[0].Status != "created" ||
		results[1].Status != "created" || results[1].UnclaimedID == nil || results[1].AwardID != nil ||
		results[2].Status != "created" || results[2].UnclaimedID == nil || results[2].AwardID != nil {
		t.Fatalf("results: %+v", results)
	}
	var unclaimed struct {
		Data []struct {
			Outcome       string  `json:"outcome"`
			HolderName    string  `json:"holder_name"`
			ItemName      string  `json:"item_name"`
			CatalogBossID *string `json:"catalog_boss_id"`
			Source        string  `json:"source"`
		}
	}
	if err := json.Unmarshal(f.request(http.MethodGet, "/unclaimed-loot", nil, http.StatusOK), &unclaimed); err != nil {
		t.Fatal(err)
	}
	if len(unclaimed.Data) != 2 || unclaimed.Data[0].Outcome != "DISENCHANTED" || unclaimed.Data[1].Outcome != "BANK" ||
		unclaimed.Data[0].HolderName != second || unclaimed.Data[0].ItemName == "" || unclaimed.Data[0].Source != "addon" ||
		unclaimed.Data[0].CatalogBossID == nil || *unclaimed.Data[0].CatalogBossID != f.ordinaryBoss {
		t.Fatalf("unclaimed loot: %+v", unclaimed.Data)
	}
	again := f.addonImport(f.token, payload, []map[string]any{{"difficulty": "NORMAL", "line_key": rows[6].LineKey, "catalog_boss_id": f.ordinaryBoss}})
	if len(again) != 1 || again[0].Status != "already_imported" || again[0].UnclaimedID == nil || *again[0].UnclaimedID != *results[1].UnclaimedID {
		t.Fatalf("a second import must not record the line again: %+v", again)
	}
	if r := f.addonPreview(payload).Rows[6]; r.IsImportable || !strings.Contains(strings.Join(r.Warnings, " "), "Bereits als Beute ohne Empfänger") {
		t.Fatalf("an imported unclaimed line is marked: %+v", r)
	}
	f.points(f.first, 0)
	if r := f.addonPreview(strings.Replace(payload, ";given;87;", ";deleted;87;", 1)).Rows; r[1].IsImportable || !strings.Contains(strings.Join(r[0].Warnings, " "), "Im Addon gelöscht") {
		t.Fatalf("rows after import: %+v", r)
	}
}

func TestAddonLootImportIsIdempotentAcrossStaff(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	first := f.characterName(f.firstChar)
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1|1760000100|-|%s|%s|given\n2|1760000200|-|%s|%s|given", f.raidID, f.ordinaryItem, first, f.tokenItem, first)
	preview := f.addonPreview(payload)
	selections := []map[string]any{
		{"difficulty": "NORMAL", "line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN"},
		{"difficulty": "NORMAL", "line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.tokenBoss, "character_id": f.firstChar, "award_method": "SECOND"},
	}
	results := f.addonImport(f.token, payload, selections)
	if len(results) != 2 || results[0].Status != "created" || results[1].Status != "created" || results[0].CharacterName != first {
		t.Fatalf("first import: %+v", results)
	}

	seedAccount(t, f.db, "addon-assistant", "password", validation.RoleRaidassistent)
	assistant := login(t, f.srv.Client(), f.srv, "addon-assistant", "password")
	for _, token := range []string{f.token, assistant} {
		again := f.addonImport(token, payload, selections)
		if len(again) != 2 || again[0].Status != "already_imported" || again[1].Status != "already_imported" || *again[0].AwardID != *results[0].AwardID {
			t.Fatalf("repeat import must be a no-op: %+v", again)
		}
	}
	rows := f.addonPreview(payload).Rows
	if !rows[0].IsAlreadyImported || rows[0].IsImportable {
		t.Fatalf("preview must show imported lines: %+v", rows[0])
	}
	if rows[1].CatalogBossID == nil || *rows[1].CatalogBossID != f.tokenBoss {
		t.Fatalf("an imported line must show the boss chosen at import: %+v", rows[1])
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 2 {
		t.Fatalf("awards: %v %v", awards, err)
	}
	for _, award := range awards {
		if award.AwardedByPlayerID != f.actor || award.AwardedToCharacterID == nil || *award.AwardedToCharacterID != f.firstChar {
			t.Fatalf("award: %+v", award)
		}
	}
}

func TestAddonLootImportReportsFailuresPerLine(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	first := f.characterName(f.firstChar)
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1|1760000100|-|%s|%s|given\n2|1760000200|-|%s|%s|given", f.raidID, f.ordinaryItem, first, f.ordinaryItem, first)
	preview := f.addonPreview(payload)
	results := f.addonImportStatus(f.token, payload, []map[string]any{
		{"difficulty": "NORMAL", "line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"},
		{"difficulty": "NORMAL", "line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "ENHANCED"},
		{"difficulty": "NORMAL", "line_key": "raidsync-unknown", "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN"},
	}, http.StatusUnprocessableEntity)
	if len(results) != 3 || results[0].Code != "recipient_not_in_roster" || results[1].Code != "insufficient_balance" || results[2].Code != "unknown_line" {
		t.Fatalf("results: %+v", results)
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatalf("failed lines wrote loot: %v %v", awards, err)
	}
}

func TestAddonLootImportAppliesItemLock(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.second, RaidID: f.raidID, ItemID: f.ordinaryItem, Status: validation.LockStatusActive, LockedAt: now, ActorID: f.second, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1|1760000100|-|%s|%s|given", f.raidID, f.ordinaryItem, f.characterName(f.firstChar))
	preview := f.addonPreview(payload)
	if !strings.Contains(strings.Join(preview.Rows[0].Warnings, " "), "Item-Lock") {
		t.Fatalf("lock warning missing: %+v", preview.Rows[0])
	}
	results := f.addonImport(f.token, payload, []map[string]any{
		{"difficulty": "NORMAL", "line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "SECOND"},
	})
	if len(results) != 1 || results[0].Status != "created" || results[0].CharacterName != f.characterName(f.secondChar) {
		t.Fatalf("lock owner must receive the item: %+v", results)
	}
	award, err := loot.NewRepo(f.db).Get(context.Background(), *results[0].AwardID)
	if err != nil || award.AwardedToPlayerID != f.second || award.AwardMethod != loot.Main {
		t.Fatalf("award: %+v %v", award, err)
	}
}

func TestAddonLootImportPermissionsAndRaidState(t *testing.T) {
	f := newCorrectionFixture(t)
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000", f.raidID)
	for _, suffix := range []string{"/addon-loot-import/preview", "/addon-loot-import"} {
		status, body := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix), f.memberToken, map[string]any{"payload": payload})
		if status != http.StatusForbidden {
			t.Fatalf("member %s: %d %s", suffix, status, body)
		}
	}
	result := f.request(http.MethodPost, "/addon-loot-import/preview", map[string]any{"payload": "Shanna:BD-AAAA"}, http.StatusBadRequest)
	if !bytes.Contains(result, []byte(`"code":"invalid_addon_export"`)) {
		t.Fatalf("wrong code: %s", result)
	}
	f.archiveHistory()
	result = f.request(http.MethodPost, "/addon-loot-import/preview", map[string]any{"payload": payload}, http.StatusConflict)
	if !bytes.Contains(result, []byte(`"code":"raid_state"`)) {
		t.Fatalf("wrong code: %s", result)
	}
}

func TestAddonLootReimportAfterTradeKeepsTheAward(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	first, second := f.characterName(f.firstChar), f.characterName(f.secondChar)
	held := fmt.Sprintf("RS1|LOOT|%d|%s|1760000000\n1|1760000100|-|%s|%s|held", f.raidID, first, f.ordinaryItem, first)
	preview := f.addonPreview(held)
	row := preview.Rows[0]
	if row.State != "held" || !row.IsImportable || !strings.Contains(strings.Join(row.Warnings, " "), "Noch beim Plündermeister") {
		t.Fatalf("held drop must warn: %+v", row)
	}
	if len(preview.RecipientOptions) != 2 {
		t.Fatalf("recipient options: %+v", preview.RecipientOptions)
	}
	results := f.addonImport(f.token, held, []map[string]any{{"difficulty": "NORMAL", "line_key": row.LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN"}})
	if len(results) != 1 || results[0].Status != "created" {
		t.Fatalf("first import: %+v", results)
	}

	// The loot master traded the item on after the first import.
	traded := fmt.Sprintf("RS1|LOOT|%d|%s|1760000000\n1|1760000100|-|%s|%s|given", f.raidID, first, f.ordinaryItem, second)
	again := f.addonPreview(traded).Rows[0]
	if again.LineKey != row.LineKey || !again.IsAlreadyImported || again.IsImportable || len(again.Warnings) != 2 || !strings.Contains(again.Warnings[1], "Bereits importiert an "+first) {
		t.Fatalf("re-import after a trade must be recognised and warn only about the recipient: %+v", again)
	}
	if same := f.addonPreview(held).Rows[0]; len(same.Warnings) != 1 {
		t.Fatalf("an unchanged imported line only says it was imported: %+v", same)
	}
	repeat := f.addonImport(f.token, traded, []map[string]any{{"difficulty": "NORMAL", "line_key": again.LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"}})
	if len(repeat) != 1 || repeat[0].Status != "already_imported" {
		t.Fatalf("repeat must not create a second award: %+v", repeat)
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 1 || *awards[0].AwardedToCharacterID != f.firstChar {
		t.Fatalf("awards: %+v %v", awards, err)
	}
}

func TestAddonLootImportAcceptsAChosenRecipient(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	// A unique item went to a helper first, who traded it to the winner.
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1|1760000100|-|%s|Helfer|given\n2|1760000200|-|%s|-|-", f.raidID, f.ordinaryItem, f.ordinaryItem)
	preview := f.addonPreview(payload)
	results := f.addonImportStatus(f.token, payload, []map[string]any{
		{"difficulty": "NORMAL", "line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"},
		{"difficulty": "NORMAL", "line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": 0, "award_method": "MAIN"},
	}, http.StatusUnprocessableEntity)
	if len(results) != 2 || results[0].Code != "import_rolled_back" || results[0].CharacterName != f.characterName(f.secondChar) || results[1].Code != "recipient_not_in_roster" {
		t.Fatalf("results: %+v", results)
	}
}

type rosterPreviewRow struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	CharacterID     *int64 `json:"character_id"`
	CharacterName   string `json:"character_name"`
	SuggestedRole   string `json:"suggested_role"`
	SuggestLateJoin bool   `json:"suggest_late_join"`
	Errors          []string
}

type rosterPreview struct {
	HeaderRaidID int64              `json:"header_raid_id"`
	Rows         []rosterPreviewRow `json:"rows"`
	Missing      []string           `json:"missing"`
	Warnings     []string           `json:"warnings"`
}

type rosterImportResult struct {
	CharacterID  int64  `json:"character_id"`
	Status       string `json:"status"`
	AttendanceID *int64 `json:"attendance_id"`
	Code         string `json:"code"`
}

func (f *correctionFixture) rosterPreview(payload string) rosterPreview {
	f.t.Helper()
	body := f.request(http.MethodPost, "/addon-roster-import/preview", map[string]any{"payload": payload}, http.StatusOK)
	var out struct{ Data rosterPreview }
	if err := json.Unmarshal(body, &out); err != nil {
		f.t.Fatal(err)
	}
	return out.Data
}

func (f *correctionFixture) rosterImport(payload string, selections []map[string]any) []rosterImportResult {
	f.t.Helper()
	body := f.request(http.MethodPost, "/addon-roster-import", map[string]any{"payload": payload, "selections": selections}, http.StatusOK)
	var out struct{ Data []rosterImportResult }
	if err := json.Unmarshal(body, &out); err != nil {
		f.t.Fatal(err)
	}
	return out.Data
}

func TestAddonRosterPreviewMatchesRosterKnownAndUnknown(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1760000000, 0).UTC()
	rd.ScheduledStart = &start
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	// The second character healed last time in another raid.
	now := time.Now().UTC()
	earlier := &raid.Raid{Name: "Earlier raid", RaidLeaderID: f.actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(f.db).Create(ctx, earlier); err != nil {
		t.Fatal(err)
	}
	if err := attendance.NewRepo(f.db).Create(ctx, &attendance.Attendance{RaidID: earlier.ID, PlayerID: f.second, CharacterID: f.secondChar, Role: validation.RoleHeal, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	umlaut := &character.Character{PlayerID: f.second, Name: "Rüdiger", Class: "Warrior", MainSpec: "Protection", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(f.db).Create(ctx, umlaut); err != nil {
		t.Fatal(err)
	}
	first, second := f.characterName(f.firstChar), f.characterName(f.secondChar)
	payload := strings.Join([]string{
		fmt.Sprintf("RS1|ROSTER|%d|%s|1760007200", f.raidID, first),
		fmt.Sprintf("%s|MAGE|1760000060", strings.ToUpper(first)),
		fmt.Sprintf("%s|MAGE|1760003600", strings.ToUpper(second)),
		"Weltchat|PALADIN|1760003600",
		"kaputt",
		"RÜDIGER|WARRIOR|1760000100",
	}, "\n")
	preview := f.rosterPreview(payload)
	if len(preview.Rows) != 5 || len(preview.Missing) != 0 || len(preview.Warnings) != 0 {
		t.Fatalf("preview: %+v", preview)
	}
	if row := preview.Rows[0]; row.Status != "in_roster" || row.CharacterID == nil || *row.CharacterID != f.firstChar {
		t.Fatalf("roster member: %+v", row)
	}
	if row := preview.Rows[1]; row.Status != "known" || *row.CharacterID != f.secondChar || row.SuggestedRole != "HEAL" || !row.SuggestLateJoin {
		t.Fatalf("known character: %+v", row)
	}
	if row := preview.Rows[2]; row.Status != "unknown" || row.CharacterID != nil {
		t.Fatalf("unknown character: %+v", row)
	}
	if row := preview.Rows[3]; row.Status != "invalid" || len(row.Errors) != 1 {
		t.Fatalf("broken line: %+v", row)
	}
	if row := preview.Rows[4]; row.Status != "known" || row.CharacterID == nil || *row.CharacterID != umlaut.ID || row.CharacterName != "Rüdiger" {
		t.Fatalf("known character must match regardless of case, umlauts included: %+v", row)
	}
	missing := f.rosterPreview(fmt.Sprintf("RS1|ROSTER|%d|x|1\n%s|-|1760000000", f.raidID+1, second))
	if len(missing.Missing) != 1 || missing.Missing[0] != first || len(missing.Warnings) != 1 || missing.Rows[0].SuggestedRole != "HEAL" || missing.Rows[0].SuggestLateJoin {
		t.Fatalf("roster members not seen in game must be listed: %+v", missing)
	}
	if rows, err := attendance.NewRepo(f.db).ListByRaid(ctx, f.raidID); err != nil || len(rows) != 1 {
		t.Fatalf("preview wrote attendance: %v %v", rows, err)
	}
}

func TestAddonRosterImportAddsKnownCharactersOnce(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	second := f.characterName(f.secondChar)
	payload := fmt.Sprintf("RS2;ROSTER;%d;x;1760007200\n%s;MAGE;1760003600", f.raidID, second)
	results := f.rosterImport(payload, []map[string]any{
		{"character_id": f.secondChar, "role": "HEAL", "late_join": true},
		{"character_id": f.firstChar, "role": "DD"},
		{"character_id": 999999, "role": "DD"},
	})
	if len(results) != 3 || results[0].Status != "added" || results[0].AttendanceID == nil || results[1].Code != "not_in_export" || results[2].Code != "unknown_character" {
		t.Fatalf("results: %+v", results)
	}
	row, err := attendance.NewRepo(f.db).Get(context.Background(), *results[0].AttendanceID)
	if err != nil || row.PlayerID != f.second || row.Role != validation.RoleHeal || !row.LateJoin {
		t.Fatalf("attendance: %+v %v", row, err)
	}
	again := f.rosterImport(payload, []map[string]any{{"character_id": f.secondChar, "role": "DD"}})
	if len(again) != 1 || again[0].Status != "already_in_roster" {
		t.Fatalf("repeat must not add twice: %+v", again)
	}
	if rows, err := attendance.NewRepo(f.db).ListByRaid(context.Background(), f.raidID); err != nil || len(rows) != 2 {
		t.Fatalf("attendance rows: %v %v", rows, err)
	}
	if bad := f.rosterImport(payload, []map[string]any{{"character_id": f.secondChar, "role": "BOSS"}}); bad[0].Status != "already_in_roster" {
		t.Fatalf("existing row is reported before the role is checked: %+v", bad)
	}
}

func TestAddonRosterImportPermissionsAndRaidState(t *testing.T) {
	f := newCorrectionFixture(t)
	payload := fmt.Sprintf("RS1|ROSTER|%d|x|1", f.raidID)
	for _, suffix := range []string{"/addon-roster-import/preview", "/addon-roster-import"} {
		status, body := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix), f.memberToken, map[string]any{"payload": payload})
		if status != http.StatusForbidden {
			t.Fatalf("member %s: %d %s", suffix, status, body)
		}
	}
	result := f.request(http.MethodPost, "/addon-roster-import/preview", map[string]any{"payload": fmt.Sprintf("RS1|LOOT|%d|x|1", f.raidID)}, http.StatusBadRequest)
	if !bytes.Contains(result, []byte(`"code":"invalid_addon_export"`)) {
		t.Fatalf("wrong code: %s", result)
	}
	f.archiveHistory()
	result = f.request(http.MethodPost, "/addon-roster-import", map[string]any{"payload": payload}, http.StatusConflict)
	if !bytes.Contains(result, []byte(`"code":"raid_state"`)) {
		t.Fatalf("wrong code: %s", result)
	}
}

// raidsyncAward is an addon line as importLine writes it.
func (f *correctionFixture) raidsyncAward(requestID string, actor int64) *loot.Award {
	now := time.Now().UTC()
	return &loot.Award{Difficulty: "NORMAL", RaidID: f.raidID, CatalogBossID: &f.ordinaryBoss, ItemID: f.ordinaryItem, AwardedToPlayerID: f.first, AwardedToCharacterID: &f.firstChar, AwardedByPlayerID: actor, Source: "manual", AwardMethod: loot.Main, RequestID: &requestID, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
}

func TestAddonLootRequestIsRaidScopedAcrossStaff(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	assistant := seedAccount(t, f.db, "race-assistant", "password", validation.RoleRaidassistent)
	corrections := raidcorrection.NewService(raidcorrection.NewRepo(f.db))
	ctx := context.Background()
	// Both staff members passed the preview's pre-read before either wrote.
	if err := corrections.SaveLoot(ctx, f.raidsyncAward("raidsync-race", f.actor), f.actor, ""); err != nil {
		t.Fatal(err)
	}
	if err := corrections.SaveLoot(ctx, f.raidsyncAward("raidsync-race", assistant), assistant, ""); !errors.Is(err, loot.ErrAlreadyRecorded) {
		t.Fatalf("second staff member must not record the line again: %v", err)
	}
	// Ordinary request IDs stay per staff member.
	if err := corrections.SaveLoot(ctx, f.raidsyncAward("manual-1", f.actor), f.actor, ""); err != nil {
		t.Fatal(err)
	}
	if err := corrections.SaveLoot(ctx, f.raidsyncAward("manual-1", assistant), assistant, ""); err != nil {
		t.Fatalf("ordinary request IDs are scoped to the actor: %v", err)
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(ctx, f.raidID)
	if err != nil || len(awards) != 3 {
		t.Fatalf("awards: %+v %v", awards, err)
	}
}

func TestLootIndexKeepsRaidsyncRequestsUniquePerRaid(t *testing.T) {
	f := newCorrectionFixture(t)
	assistant := seedAccount(t, f.db, "index-assistant", "password", validation.RoleRaidassistent)
	repo := loot.NewRepo(f.db)
	ctx := context.Background()
	if err := repo.CreateAward(ctx, f.raidsyncAward("raidsync-index", f.actor)); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAward(ctx, f.raidsyncAward("raidsync-index", assistant)); err == nil {
		t.Fatal("the database must reject a second award for the same raid and RaidSync line")
	}
	if err := repo.CreateAward(ctx, f.raidsyncAward("manual-index", f.actor)); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAward(ctx, f.raidsyncAward("manual-index", assistant)); err != nil {
		t.Fatalf("ordinary request IDs of different staff members must stay allowed: %v", err)
	}
}

func TestAddonLootImportConcurrentStaffCreateOneAward(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	seedAccount(t, f.db, "parallel-assistant", "password", validation.RoleRaidassistent)
	assistant := login(t, f.srv.Client(), f.srv, "parallel-assistant", "password")
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1|1760000100|-|%s|%s|given", f.raidID, f.ordinaryItem, f.characterName(f.firstChar))
	key := f.addonPreview(payload).Rows[0].LineKey
	selections := []map[string]any{{"difficulty": "NORMAL", "line_key": key, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN"}}
	start := make(chan struct{})
	results := make([][]addonImportResult, 2)
	var wg sync.WaitGroup
	for i, token := range []string{f.token, assistant} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, body := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/addon-loot-import", f.srv.URL, f.raidID), token, map[string]any{"payload": payload, "selections": selections})
			var out struct{ Data []addonImportResult }
			if status != http.StatusOK || json.Unmarshal(body, &out) != nil {
				t.Errorf("import %d: %d %s", i, status, body)
				return
			}
			results[i] = out.Data
		}()
	}
	close(start)
	wg.Wait()
	statuses := map[string]int{}
	for _, result := range results {
		if len(result) == 1 {
			statuses[result[0].Status]++
		}
	}
	if statuses["created"] != 1 || statuses["already_imported"] != 1 {
		t.Fatalf("one import must create, the other report already imported: %+v", results)
	}
	if awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID); err != nil || len(awards) != 1 {
		t.Fatalf("awards: %+v %v", awards, err)
	}
}

// uniqueCreatureDrop finds an ICC25 item that only one single-creature boss drops.
func uniqueCreatureDrop(t *testing.T, f *correctionFixture) (string, string) {
	t.Helper()
	bossesByItem, err := lootcatalog.NewService(lootcatalog.NewRepo(f.db)).BossesByItem(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := lootcatalog.NewService(lootcatalog.NewRepo(f.db)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range catalog.Drops {
		bosses := bossesByItem[drop.ItemID]
		if drop.RaidSize == 25 && len(bosses) == 1 && bosses[0].InstanceID == "631" && !strings.HasPrefix(bosses[0].ID, "encounter:") {
			return bosses[0].ID, drop.ItemID
		}
	}
	t.Fatal("no ICC25 item with a single creature boss")
	return "", ""
}

func tokenBossCreature(t *testing.T, options []lootcatalog.Boss) string {
	t.Helper()
	for _, boss := range options {
		if !strings.HasPrefix(boss.ID, "encounter:") {
			return boss.ID
		}
	}
	t.Fatal("token has no single creature boss")
	return ""
}
