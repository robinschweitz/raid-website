package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/raid"
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
	f.t.Helper()
	status, body := do(f.t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/addon-loot-import", f.srv.URL, f.raidID), token, map[string]any{"payload": payload, "selections": selections})
	if status != http.StatusOK {
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
		{"", fmt.Sprintf("RS1|INV|%d|Correction raid|25\nUmbenannt|DD|Magier|0", f.raidID)},
		{"?bench=1", fmt.Sprintf("RS1|INV|%d|Correction raid|25\nUmbenannt|DD|Magier|0\nCorrection1|DD|Magier|1", f.raidID)},
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

func TestAddonLootImportIsIdempotentAcrossStaff(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	first := f.characterName(f.firstChar)
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1|1760000100|-|%s|%s|given\n2|1760000200|-|%s|%s|given", f.raidID, f.ordinaryItem, first, f.tokenItem, first)
	preview := f.addonPreview(payload)
	selections := []map[string]any{
		{"line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN"},
		{"line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.tokenBoss, "character_id": f.firstChar, "award_method": "SECOND"},
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
	results := f.addonImport(f.token, payload, []map[string]any{
		{"line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"},
		{"line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "ENHANCED"},
		{"line_key": "raidsync-unknown", "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN"},
	})
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
		{"line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "SECOND"},
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
	results := f.addonImport(f.token, held, []map[string]any{{"line_key": row.LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN"}})
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
	repeat := f.addonImport(f.token, traded, []map[string]any{{"line_key": again.LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"}})
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
	results := f.addonImport(f.token, payload, []map[string]any{
		{"line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.secondChar, "award_method": "MAIN"},
		{"line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": 0, "award_method": "MAIN"},
	})
	if len(results) != 2 || results[0].Status != "created" || results[0].CharacterName != f.characterName(f.secondChar) || results[1].Code != "recipient_not_in_roster" {
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
	first, second := f.characterName(f.firstChar), f.characterName(f.secondChar)
	payload := strings.Join([]string{
		fmt.Sprintf("RS1|ROSTER|%d|%s|1760007200", f.raidID, first),
		fmt.Sprintf("%s|MAGE|1760000060", strings.ToUpper(first)),
		fmt.Sprintf("%s|MAGE|1760003600", second),
		"Weltchat|PALADIN|1760003600",
		"kaputt",
	}, "\n")
	preview := f.rosterPreview(payload)
	if len(preview.Rows) != 4 || len(preview.Missing) != 0 || len(preview.Warnings) != 0 {
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
	payload := fmt.Sprintf("RS1|ROSTER|%d|x|1760007200\n%s|MAGE|1760003600", f.raidID, second)
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
