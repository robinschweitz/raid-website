package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/tokens"
	"raid-website/internal/validation"
)

func integrationCatalog() lootcatalog.Dataset {
	return lootcatalog.Dataset{
		Source:    lootcatalog.Source{ID: "integration", Revision: "pinned", URL: "https://example.org/source", License: "test", Attribution: "integration fixture"},
		Notices:   []string{"Synthetic integration fixture, not production data"},
		Instances: []lootcatalog.Instance{{ID: "631", Name: "ICC"}, {ID: "603", Name: "Ulduar"}},
		Bosses:    []lootcatalog.Boss{{ID: "1", InstanceID: "631", Name: "First boss"}, {ID: "2", InstanceID: "631", Name: "Second boss"}, {ID: "3", InstanceID: "603", Name: "Other instance boss"}},
		Items:     []lootcatalog.Item{{ItemID: "11", Name: "Canonical item"}, {ItemID: "12", Name: "Other item"}},
		Drops:     []lootcatalog.Drop{{BossID: "1", ItemID: "11", RaidSize: 10, Difficulty: "source/base", LootMode: 1}, {BossID: "2", ItemID: "12", RaidSize: 25, Difficulty: "source/hard", LootMode: 2}, {BossID: "3", ItemID: "11", RaidSize: 10, Difficulty: "source/base", LootMode: 1}},
	}
}

func mergeIntegrationCatalog(t *testing.T, f *correctionFixture) {
	t.Helper()
	url := f.srv.URL + "/api/v1/loot-catalog"
	input := map[string]any{"catalog": integrationCatalog()}
	status, body := do(t, f.srv.Client(), http.MethodPost, url+"/merge", f.token, input)
	if status != http.StatusConflict || !bytes.Contains(body, []byte("catalog_preview_required")) {
		t.Fatalf("unpreviewed merge: %d %s", status, body)
	}
	status, body = do(t, f.srv.Client(), http.MethodPost, url+"/preview", f.token, input)
	if status != http.StatusOK {
		t.Fatalf("preview: %d %s", status, body)
	}
	var preview struct{ Data lootcatalog.Report }
	if err := json.Unmarshal(body, &preview); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, f.srv.Client(), http.MethodGet, url+"/instances", f.token, nil)
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"data":[]`)) {
		t.Fatalf("preview wrote domain records: %d %s", status, body)
	}
	input["raw_hash"] = preview.Data.RawHash
	for i := 0; i < 2; i++ {
		status, body = do(t, f.srv.Client(), http.MethodPost, url+"/merge", f.token, input)
		if status != http.StatusOK {
			t.Fatalf("merge: %d %s", status, body)
		}
	}
	changed := integrationCatalog()
	changed.Items[0].Name = "Overwrite attempt"
	status, body = do(t, f.srv.Client(), http.MethodPost, url+"/preview", f.token, map[string]any{"catalog": changed})
	if status != http.StatusConflict || !bytes.Contains(body, []byte("catalog_conflict")) {
		t.Fatalf("overwrite: %d %s", status, body)
	}
}

func TestCatalogRealRouterPermissionsAndEmptyArrays(t *testing.T) {
	f := newCorrectionFixture(t)
	seedAccount(t, f.db, "catalog-guest", "password", validation.RoleGuest)
	guestToken := login(t, f.srv.Client(), f.srv, "catalog-guest", "password")
	for _, path := range []string{"", "/instances", "/bosses", "/items"} {
		url := f.srv.URL + "/api/v1/loot-catalog" + path
		status, body := do(t, f.srv.Client(), http.MethodGet, url, "", nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("anonymous read: %d %s", status, body)
		}
		status, body = do(t, f.srv.Client(), http.MethodGet, url, guestToken, nil)
		if status != http.StatusOK || bytes.Contains(body, []byte("null")) {
			t.Fatalf("guest read: %d %s", status, body)
		}
	}
	for _, path := range []string{"/preview", "/merge"} {
		for _, token := range []string{"", guestToken, f.memberToken} {
			want := http.StatusForbidden
			if token == "" {
				want = http.StatusUnauthorized
			}
			status, body := do(t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/loot-catalog"+path, token, map[string]any{"catalog": integrationCatalog()})
			if status != want {
				t.Fatalf("catalog write permission: %d %s", status, body)
			}
		}
	}
	mergeIntegrationCatalog(t, f)
}

func TestCatalogAwardsValidateBeforeLedgersAndPreserveSnapshots(t *testing.T) {
	f := newCorrectionFixture(t)
	mergeIntegrationCatalog(t, f)
	f.attend(f.first, f.firstChar)
	f.seedPoints(f.first, 70)
	f.request(http.MethodPatch, "", map[string]any{"catalog_instance_id": "631", "raid_size": 10}, http.StatusOK)
	for _, association := range []map[string]any{{"catalog_instance_id": "999"}, {"raid_size": 15}} {
		f.request(http.MethodPatch, "", association, http.StatusBadRequest)
	}
	for _, selection := range []map[string]any{{}, {"catalog_boss_id": "999", "item_id": "11"}, {"catalog_boss_id": "3", "item_id": "11"}, {"catalog_boss_id": "1", "item_id": "12"}, {"catalog_boss_id": "2", "item_id": "12"}} {
		body := map[string]any{"award_method": "ENHANCED", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "request_id": "invalid-catalog-award"}
		for k, v := range selection {
			body[k] = v
		}
		result := f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
		if !bytes.Contains(result, []byte(`"code":"catalog_item_invalid"`)) {
			t.Fatalf("wrong catalog code: %s", result)
		}
	}
	f.points(f.first, 70)
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatalf("invalid award persisted: %v %v", awards, err)
	}
	var count int
	if err := f.db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Scan(context.Background(), &count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid award wrote audit: %d", count)
	}
	body := map[string]any{"catalog_boss_id": "1", "item_id": "11", "boss": "Untrusted boss", "item_name": "Untrusted item", "award_method": "ENHANCED", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "request_id": "catalog-award"}
	id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	if retry := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated)); retry != id {
		t.Fatal("catalog retry duplicated award")
	}
	a := f.award(id)
	if a.Boss != "First boss" || a.ItemName != "Canonical item" || a.CatalogBossID == nil || *a.CatalogBossID != "1" {
		t.Fatalf("canonical award: %+v", a)
	}
	f.points(f.first, 20)
	suffix := fmt.Sprintf("/loot/%d", id)
	f.request(http.MethodPatch, suffix, map[string]any{"catalog_boss_id": "2", "item_id": "12", "reason": "wrong size"}, http.StatusBadRequest)
	f.request(http.MethodPatch, suffix, map[string]any{"notes": "clarify", "boss": "Overwrite", "item_name": "Overwrite", "reason": "notes"}, http.StatusOK)
	a = f.award(id)
	if a.Boss != "First boss" || a.ItemName != "Canonical item" || a.EnhancedCost != 50 {
		t.Fatalf("snapshot changed: %+v", a)
	}
	f.points(f.first, 20)
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "loot_award", fmt.Sprint(id))
	if err != nil || len(logs) != 2 {
		t.Fatalf("unexpected audits: %v %v", logs, err)
	}
}

func TestCatalogRaidSeriesInheritanceAndPreplannedReuse(t *testing.T) {
	for _, preplanned := range []bool{false, true} {
		t.Run(fmt.Sprint(preplanned), func(t *testing.T) {
			f := newCorrectionFixture(t)
			mergeIntegrationCatalog(t, f)
			url := f.srv.URL + "/api/v1"
			request := func(method, path string, body any, want int) []byte {
				status, result := do(t, f.srv.Client(), method, url+path, f.token, body)
				if status != want {
					t.Fatalf("%s %s: %d %s", method, path, status, result)
				}
				return result
			}
			config := map[string]any{"name": "Explicit series", "raid_leader_id": f.actor, "weekdays": []int{1}, "interval_weeks": 1, "start_time": "20:00", "timezone": "UTC", "catalog_instance_id": "631", "raid_size": 10}
			seriesID := correctionID(t, request(http.MethodPost, "/raid-series", config, http.StatusCreated))
			request(http.MethodPatch, fmt.Sprintf("/raid-series/%d", seriesID), map[string]any{"raid_size": 15}, http.StatusBadRequest)
			request(http.MethodPatch, fmt.Sprintf("/raid-series/%d", seriesID), map[string]any{"name": "Renamed"}, http.StatusOK)
			start := time.Date(2090, 1, 2, 20, 0, 0, 0, time.UTC)
			f.raidID = correctionID(t, request(http.MethodPost, "/raids", map[string]any{"name": "Manual", "raid_leader_id": f.actor, "series_id": seriesID, "scheduled_start": start.Format(time.RFC3339)}, http.StatusCreated))
			rd, err := raid.NewRepo(f.db).Get(context.Background(), f.raidID)
			if err != nil || rd.CatalogInstanceID == nil || *rd.CatalogInstanceID != "631" || rd.RaidSize == nil || *rd.RaidSize != 10 {
				t.Fatalf("create inheritance: %+v %v", rd, err)
			}
			// Explicit selection wins over the series on manual creation.
			explicitID := correctionID(t, request(http.MethodPost, "/raids", map[string]any{"name": "Other", "raid_leader_id": f.actor, "series_id": seriesID, "catalog_instance_id": "603", "raid_size": 10}, http.StatusCreated))
			explicit, err := raid.NewRepo(f.db).Get(context.Background(), explicitID)
			if err != nil || explicit.CatalogInstanceID == nil || *explicit.CatalogInstanceID != "603" {
				t.Fatalf("explicit selection overwritten: %+v %v", explicit, err)
			}
			f.attend(f.first, f.firstChar)
			var nextID int64
			if preplanned {
				// Jan 2, 2090 is Monday; the next occurrence is one week later.
				nextID = correctionID(t, request(http.MethodPost, "/raids", map[string]any{"name": "Preplanned", "raid_leader_id": f.actor, "series_id": seriesID, "scheduled_start": start.AddDate(0, 0, 7).Format(time.RFC3339), "catalog_instance_id": "603", "raid_size": 10}, http.StatusCreated))
			}
			result := f.request(http.MethodPost, "/archive", nil, http.StatusOK)
			var archive struct {
				NextRaid raid.Raid `json:"next_raid"`
			}
			if err := json.Unmarshal(result, &archive); err != nil {
				t.Fatal(err)
			}
			wantInstance := "631"
			if preplanned {
				wantInstance = "603"
				if archive.NextRaid.ID != nextID {
					t.Fatalf("preplanned raid not reused: %+v", archive.NextRaid)
				}
			}
			if archive.NextRaid.CatalogInstanceID == nil || *archive.NextRaid.CatalogInstanceID != wantInstance || archive.NextRaid.RaidSize == nil || *archive.NextRaid.RaidSize != 10 {
				t.Fatalf("successor metadata: %+v", archive.NextRaid)
			}
			f.request(http.MethodPatch, "", map[string]any{"raid_size": 25}, http.StatusConflict)
			f.request(http.MethodPost, "/archive", nil, http.StatusOK)
			f.points(f.first, 10)
		})
	}
}

func TestCatalogAssociationKeepsHistoricalFreeTextAndImportOptional(t *testing.T) {
	f := newCorrectionFixture(t)
	mergeIntegrationCatalog(t, f)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	f.seedPoints(f.first, 91)
	id := correctionID(t, f.request(http.MethodPost, "/loot", map[string]any{"boss": "Historical boss", "item_name": "Historical item", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN"}, http.StatusCreated))
	f.request(http.MethodPatch, "", map[string]any{"catalog_instance_id": "631"}, http.StatusOK)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"notes": "historical clarification", "reason": "clarify"}, http.StatusOK)
	a := f.award(id)
	if a.CatalogBossID != nil || a.ItemID != "" || a.ItemName != "Historical item" || a.Boss != "Historical boss" {
		t.Fatalf("fabricated historical catalog: %+v", a)
	}
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "new recipient requires selection"}, http.StatusBadRequest)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"catalog_boss_id": "1", "item_id": "11", "reason": "explicit catalog identity"}, http.StatusOK)
	if a = f.award(id); a.CatalogBossID == nil || a.ItemName != "Canonical item" {
		t.Fatalf("explicit correction not resolved: %+v", a)
	}
	imported := &loot.Award{RaidID: f.raidID, Boss: "Imported boss", ItemName: "Imported item", AwardedToPlayerID: f.first, Source: "legacy_import", AwardedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), IsMainSpec: true}
	if err := raidcorrection.NewService(raidcorrection.NewRepo(f.db)).SaveImportedLoot(context.Background(), imported, f.actor, "historical import addition"); err != nil {
		t.Fatal(err)
	}
	if imported.CatalogBossID != nil || imported.AwardedToCharacterID != nil || imported.ItemName != "Imported item" {
		t.Fatalf("import fabricated identities: %+v", imported)
	}
	f.points(f.first, 91)
	status, body := do(t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/raids", f.token, map[string]any{"name": "Size without parent", "raid_leader_id": f.actor, "raid_size": 10})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"catalog_invalid"`)) {
		t.Fatalf("orphan raid size accepted: %d %s", status, body)
	}
	status, body = do(t, f.srv.Client(), http.MethodPatch, fmt.Sprintf("%s/api/v1/raids/%d", f.srv.URL, f.raidID), f.memberToken, map[string]any{"raid_size": 25})
	if status != http.StatusForbidden {
		t.Fatalf("member metadata mutation: %d %s", status, body)
	}
}

func TestCatalogHistoricalFreeTextIdentityCorrectionsRequireSelection(t *testing.T) {
	f := newCorrectionFixture(t)
	mergeIntegrationCatalog(t, f)
	f.attend(f.first, f.firstChar)
	f.seedPoints(f.first, 91)
	id := correctionID(t, f.request(http.MethodPost, "/loot", map[string]any{"boss": "Historical boss", "item_name": "Historical item", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN"}, http.StatusCreated))
	f.request(http.MethodPatch, "", map[string]any{"catalog_instance_id": "631", "raid_size": 10}, http.StatusOK)
	suffix := fmt.Sprintf("/loot/%d", id)
	f.request(http.MethodPatch, suffix, map[string]any{"notes": "clarification", "reason": "notes only"}, http.StatusOK)
	for _, method := range []string{"SECOND", "ENHANCED", "MAIN"} {
		f.request(http.MethodPatch, suffix, map[string]any{"award_method": method, "reason": "method only"}, http.StatusOK)
		a := f.award(id)
		if a.CatalogBossID != nil || a.ItemID != "" || a.Boss != "Historical boss" || a.ItemName != "Historical item" {
			t.Fatalf("unrelated correction changed historical identity: %+v", a)
		}
		wantBalance, wantCost := 91, 0
		if method == "ENHANCED" {
			wantBalance, wantCost = 41, 50
		}
		f.points(f.first, wantBalance)
		if a.EnhancedCost != wantCost {
			t.Fatalf("method %s cost: %d", method, a.EnhancedCost)
		}
	}
	ctx := context.Background()
	logs, err := audit.NewRepo(f.db).ListByEntity(ctx, "loot_award", fmt.Sprint(id))
	if err != nil {
		t.Fatal(err)
	}
	points, err := enhancepoints.NewRepo(f.db).ListByPlayer(ctx, f.first)
	if err != nil {
		t.Fatal(err)
	}
	tokenEntries, err := tokens.NewRepo(f.db).ListByPlayer(ctx, f.first)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(f.award(id))
	if err != nil {
		t.Fatal(err)
	}
	for _, patch := range []map[string]any{
		{"item_name": "Arbitrary replacement", "reason": "name only"},
		{"item_name": "Weiheabzeichen des Eroberers", "reason": "token name only"},
		{"boss": "Arbitrary boss", "reason": "boss only"},
	} {
		result := f.request(http.MethodPatch, suffix, patch, http.StatusBadRequest)
		if !bytes.Contains(result, []byte(`"code":"catalog_item_invalid"`)) {
			t.Fatalf("wrong identity rejection: %s", result)
		}
		after, err := json.Marshal(f.award(id))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("rejected correction changed award: %s", after)
		}
		afterLogs, err := audit.NewRepo(f.db).ListByEntity(ctx, "loot_award", fmt.Sprint(id))
		if err != nil || len(afterLogs) != len(logs) {
			t.Fatalf("rejected correction wrote audit: %v %v", afterLogs, err)
		}
		afterPoints, err := enhancepoints.NewRepo(f.db).ListByPlayer(ctx, f.first)
		if err != nil || len(afterPoints) != len(points) {
			t.Fatalf("rejected correction wrote EP: %v %v", afterPoints, err)
		}
		afterTokens, err := tokens.NewRepo(f.db).ListByPlayer(ctx, f.first)
		if err != nil || len(afterTokens) != len(tokenEntries) {
			t.Fatalf("rejected correction wrote tokens: %v %v", afterTokens, err)
		}
		f.points(f.first, 91)
		f.tokenBalance(f.first, 0)
	}
	f.request(http.MethodPatch, suffix, map[string]any{"catalog_boss_id": "1", "item_id": "11", "boss": "Untrusted boss", "item_name": "Weiheabzeichen des Eroberers", "reason": "explicit catalog replacement"}, http.StatusOK)
	a := f.award(id)
	if a.CatalogBossID == nil || *a.CatalogBossID != "1" || a.ItemID != "11" || a.Boss != "First boss" || a.ItemName != "Canonical item" {
		t.Fatalf("catalog replacement not canonical: %+v", a)
	}
	afterLogs, err := audit.NewRepo(f.db).ListByEntity(ctx, "loot_award", fmt.Sprint(id))
	if err != nil || len(afterLogs) != len(logs)+1 {
		t.Fatalf("catalog replacement audit: %v %v", afterLogs, err)
	}
	f.points(f.first, 91)
	f.tokenBalance(f.first, 0)
}
