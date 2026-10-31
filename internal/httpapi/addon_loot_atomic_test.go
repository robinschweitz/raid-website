package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"raid-website/internal/bosskill"
	"raid-website/internal/loot"
)

func TestReqRaid017_AC02(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1\n1|2|-|%s|%s|given\n2|3|-|%s|%s|given", f.raidID, f.ordinaryItem, f.characterName(f.firstChar), f.tokenItem, f.characterName(f.firstChar))
	preview := f.addonPreview(payload)
	selections := []map[string]any{
		{"line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN", "difficulty": "NORMAL"},
		{"line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.tokenBoss, "character_id": f.firstChar, "award_method": "SECOND"},
	}
	status, body := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/addon-loot-import", f.srv.URL, f.raidID), f.token, map[string]any{"payload": payload, "selections": selections})
	if status != 422 || errorCode(t, body) != "difficulty_required" {
		t.Fatalf("missing difficulty=%d %s", status, body)
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatal("partial loot stored")
	}
	kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(kills) != 0 {
		t.Fatal("partial kills stored")
	}
}
func TestReqRaid017_AC03(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	payload := fmt.Sprintf("RS1|LOOT|%d|Recorder|1\n1|2|-|%s|%s|given\n2|3|-|%s|%s|given", f.raidID, f.ordinaryItem, f.characterName(f.firstChar), f.tokenItem, f.characterName(f.firstChar))
	preview := f.addonPreview(payload)
	selections := []map[string]any{
		{"line_key": preview.Rows[0].LineKey, "catalog_boss_id": f.ordinaryBoss, "character_id": f.firstChar, "award_method": "MAIN", "difficulty": "NORMAL"},
		{"line_key": preview.Rows[1].LineKey, "catalog_boss_id": f.tokenBoss, "character_id": f.firstChar, "award_method": "SECOND", "difficulty": "NORMAL"},
	}
	if _, err := f.db.ExecContext(context.Background(), "CREATE TRIGGER reject_last_import BEFORE INSERT ON loot_awards WHEN (SELECT COUNT(*) FROM loot_awards) = 1 BEGIN SELECT RAISE(ABORT,'last line failed'); END"); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/addon-loot-import", f.srv.URL, f.raidID), f.token, map[string]any{"payload": payload, "selections": selections})
	if status != 500 {
		t.Fatalf("last-line failure=%d %s", status, body)
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatal("partial loot stored")
	}
	kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(kills) != 0 {
		t.Fatal("partial kills stored")
	}
	// The same untouched payload succeeds after the storage failure is removed.
	if _, err := f.db.ExecContext(context.Background(), "DROP TRIGGER reject_last_import"); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/addon-loot-import", f.srv.URL, f.raidID), f.token, map[string]any{"payload": payload, "selections": selections})
	if status != 200 {
		t.Fatalf("retry=%d %s", status, body)
	}
	var out struct{ Data []addonImportResult }
	if err := json.Unmarshal(body, &out); err != nil || len(out.Data) != 2 {
		t.Fatalf("retry results=%s %v", body, err)
	}
	kills, err = bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(kills) != 2 {
		t.Fatalf("retry kills=%d %v", len(kills), err)
	}
}
