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
	"raid-website/internal/validation"
)

type addonPreviewRow struct {
	LineKey           string             `json:"line_key"`
	ItemID            string             `json:"item_id"`
	ItemName          string             `json:"item_name"`
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
	HeaderRaidID int64             `json:"header_raid_id"`
	Rows         []addonPreviewRow `json:"rows"`
	Warnings     []string          `json:"warnings"`
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
		fmt.Sprintf("1760000100|%s|%s|%s", creatureBoss, creatureItem, strings.ToUpper(first)),
		fmt.Sprintf("1760000200|-|%s|%s", f.tokenItem, first),
		fmt.Sprintf("1760000300|-|%s|Unbekannt", f.ordinaryItem),
		fmt.Sprintf("1760000400|-|%s|-", f.ordinaryItem),
		"kaputt",
		"1760000500|-|1|" + first,
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
	hinted := f.addonPreview(fmt.Sprintf("RS1|LOOT|%d|Recorder|1\n2|%s|%s|%s", f.raidID, tokenBossCreature(t, token.BossOptions), f.tokenItem, first)).Rows[0]
	if hinted.CatalogBossID == nil || *hinted.CatalogBossID != tokenBossCreature(t, token.BossOptions) {
		t.Fatalf("the killed creature must pick the boss: %+v", hinted)
	}
	if row := preview.Rows[2]; row.IsImportable || len(row.Errors) != 1 {
		t.Fatalf("unknown recipient: %+v", row)
	}
	if row := preview.Rows[3]; row.IsImportable || len(row.Warnings) != 1 {
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
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1760000100|-|%s|%s\n1760000200|-|%s|%s", f.raidID, f.ordinaryItem, first, f.tokenItem, first)
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
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1760000100|-|%s|%s\n1760000200|-|%s|%s", f.raidID, f.ordinaryItem, first, f.ordinaryItem, first)
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
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1760000000\n1760000100|-|%s|%s", f.raidID, f.ordinaryItem, f.characterName(f.firstChar))
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
