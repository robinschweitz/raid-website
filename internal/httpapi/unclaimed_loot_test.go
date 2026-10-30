package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/bosskill"
	"raid-website/internal/raid"
)

func TestStaffRecordLootThatWentToNobody(t *testing.T) {
	f := newCorrectionFixture(t)
	member := login(t, f.srv.Client(), f.srv, "correction-first", "password")
	url := fmt.Sprintf("%s/api/v1/raids/%d/unclaimed-loot", f.srv.URL, f.raidID)
	entry := map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": f.ordinaryItem, "difficulty": "NORMAL", "outcome": "NOBODY", "notes": "liegen gelassen"}

	status, body := do(t, f.srv.Client(), http.MethodPost, url, member, entry)
	if status != http.StatusForbidden {
		t.Fatalf("member: %d %s", status, body)
	}
	for _, bad := range []struct {
		entry map[string]any
		code  string
	}{
		{map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": f.ordinaryItem, "difficulty": "NORMAL", "outcome": "LOST"}, "unclaimed_invalid"},
		{map[string]any{"outcome": "NOBODY", "difficulty": "NORMAL"}, "catalog_item_invalid"},
		{map[string]any{"item_id": f.ordinaryItem, "difficulty": "NORMAL", "outcome": "NOBODY"}, "catalog_item_invalid"},
		{map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": "999999", "difficulty": "NORMAL", "outcome": "BANK"}, "catalog_item_invalid"},
		{map[string]any{"catalog_boss_id": "encounter:603:ignis", "item_id": f.ordinaryItem, "difficulty": "NORMAL", "outcome": "BANK"}, "catalog_item_invalid"},
		{map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": f.ordinaryItem, "outcome": "BANK"}, "difficulty_required"},
	} {
		status, body = do(t, f.srv.Client(), http.MethodPost, url, f.token, bad.entry)
		if status < 400 || status >= 500 || !strings.Contains(string(body), `"code":"`+bad.code+`"`) {
			t.Fatalf("invalid %v: %d %s", bad.entry, status, body)
		}
	}
	if kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID); err != nil || len(kills) != 0 {
		t.Fatalf("a rejected entry stores no kill: %+v %v", kills, err)
	}
	status, body = do(t, f.srv.Client(), http.MethodPost, url, f.token, entry)
	var response struct {
		Data struct {
			ID       int64
			ItemName string `json:"item_name"`
			Boss     string
			Outcome  string
			Source   string
		}
	}
	if err := json.Unmarshal(body, &response); err != nil || status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	created := response.Data
	kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(kills) != 1 || kills[0].CatalogBossID != f.ordinaryBoss || kills[0].Difficulty != "NORMAL" {
		t.Fatalf("the entry creates the missing kill: %+v %v", kills, err)
	}
	status, body = do(t, f.srv.Client(), http.MethodPost, url, f.token, map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": f.ordinaryItem, "difficulty": "HEROIC", "outcome": "BANK"})
	if status != http.StatusConflict || !strings.Contains(string(body), "kill_difficulty_conflict") {
		t.Fatalf("another difficulty conflicts with the kill: %d %s", status, body)
	}
	if created.ItemName == "" || created.Boss == "" || created.Outcome != "NOBODY" || created.Source != "manual" {
		t.Fatalf("the catalog fills item and boss names: %s", body)
	}
	status, body = do(t, f.srv.Client(), http.MethodGet, url, member, nil)
	var list struct{ Data []struct{ ID int64 } }
	if err := json.Unmarshal(body, &list); err != nil || status != http.StatusOK || len(list.Data) != 1 {
		t.Fatalf("members read the list: %d %s", status, body)
	}

	// Archived raids need a reason, for recording and for removing.
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	archived := time.Now().UTC()
	rd.ArchivedAt = &archived
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	remove := fmt.Sprintf("%s/api/v1/unclaimed-loot/%d/remove", f.srv.URL, created.ID)
	status, body = do(t, f.srv.Client(), http.MethodPost, remove, f.token, map[string]any{})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("remove without reason: %d %s", status, body)
	}
	status, body = do(t, f.srv.Client(), http.MethodPost, remove, f.token, map[string]any{"reason": "doch vergeben"})
	if status != http.StatusNoContent {
		t.Fatalf("remove: %d %s", status, body)
	}
	status, body = do(t, f.srv.Client(), http.MethodGet, url, member, nil)
	if err := json.Unmarshal(body, &list); err != nil || status != http.StatusOK || len(list.Data) != 0 {
		t.Fatalf("a removed entry leaves the list: %d %s", status, body)
	}
	entries, err := audit.NewRepo(f.db).ListByEntity(ctx, "unclaimed_loot", fmt.Sprint(created.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Action != "create" || entries[1].Action != "archive" {
		t.Fatalf("audit: %+v", entries)
	}
}

func TestUnclaimedLootNeedsARaidCatalogContext(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.CatalogInstanceID = nil
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/raids/%d/unclaimed-loot", f.srv.URL, f.raidID)
	status, body := do(t, f.srv.Client(), http.MethodPost, url, f.token, map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": f.ordinaryItem, "difficulty": "NORMAL", "outcome": "NOBODY"})
	if status != http.StatusConflict || !strings.Contains(string(body), "catalog_context_required") {
		t.Fatalf("a raid without instance rejects manual entries: %d %s", status, body)
	}
}

func TestDisenchantedLootNamesARosterCharacter(t *testing.T) {
	f := newCorrectionFixture(t)
	url := fmt.Sprintf("%s/api/v1/raids/%d/unclaimed-loot", f.srv.URL, f.raidID)
	entry := map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": f.ordinaryItem, "difficulty": "NORMAL", "outcome": "DISENCHANTED", "holder_name": "Ignoriert"}
	for name, character := range map[string]any{"missing": nil, "not in roster": f.firstChar} {
		entry["disenchanter_character_id"] = character
		status, body := do(t, f.srv.Client(), http.MethodPost, url, f.token, entry)
		if status != http.StatusBadRequest || !strings.Contains(string(body), "recipient_not_in_roster") {
			t.Fatalf("%s: %d %s", name, status, body)
		}
	}
	if kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID); err != nil || len(kills) != 0 {
		t.Fatalf("a rejected entry stores no kill: %+v %v", kills, err)
	}
	f.attend(f.first, f.firstChar)
	entry["disenchanter_character_id"] = f.firstChar
	status, body := do(t, f.srv.Client(), http.MethodPost, url, f.token, entry)
	var response struct {
		Data struct {
			HolderName string `json:"holder_name"`
		}
	}
	if err := json.Unmarshal(body, &response); err != nil || status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	if response.Data.HolderName != f.characterName(f.firstChar) {
		t.Fatalf("the disenchanter is the holder: %s", body)
	}
	status, body = do(t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/loot", f.srv.URL, f.raidID), f.token, nil)
	var awards struct{ Data []struct{ ID int64 } }
	if err := json.Unmarshal(body, &awards); err != nil || status != http.StatusOK || len(awards.Data) != 0 {
		t.Fatalf("disenchanting is no loot award: %d %s", status, body)
	}
}
