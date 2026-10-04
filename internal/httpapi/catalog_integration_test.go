package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/httpapi"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestCatalogRealRouterAutomaticReadsAndRemovedMutations(t *testing.T) {
	f := newCorrectionFixture(t)
	roles := []validation.Role{validation.RoleGuest, validation.RoleMember, validation.RoleRaidassistent, validation.RoleRaidleader, validation.RoleAdmin}
	for _, role := range roles {
		name := "catalog-" + string(role)
		seedAccount(t, f.db, name, "password", role)
		token := login(t, f.srv.Client(), f.srv, name, "password")
		for _, path := range []string{"", "/instances", "/bosses", "/items?boss_id=36612&raid_size=10&mode=NORMAL"} {
			url := f.srv.URL + "/api/v1/loot-catalog" + path
			status, body := do(t, f.srv.Client(), http.MethodGet, url, "", nil)
			if status != http.StatusUnauthorized {
				t.Fatalf("anonymous read: %d %s", status, body)
			}
			status, body = do(t, f.srv.Client(), http.MethodGet, url, token, nil)
			if status != http.StatusOK || bytes.Contains(body, []byte(`"data":[]`)) || bytes.Contains(body, []byte(`"data":null`)) {
				t.Fatalf("%s automatic read: %d %s", role, status, body)
			}
		}
		for _, path := range []string{"/preview", "/merge"} {
			status, body := do(t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/loot-catalog"+path, token, map[string]any{})
			if status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
				t.Fatalf("removed mutation: %d %s", status, body)
			}
		}
	}
	for _, query := range []string{"mode=UNKNOWN", "mode=heroic", "raid_size=0", "raid_size=15", "raid_size=abc"} {
		status, body := do(t, f.srv.Client(), http.MethodGet, f.srv.URL+"/api/v1/loot-catalog/items?"+query, f.token, nil)
		if status != http.StatusUnprocessableEntity || !bytes.Contains(body, []byte(`"code":"catalog_invalid"`)) {
			t.Fatalf("invalid filter: %d %s", status, body)
		}
	}
	status, body := do(t, f.srv.Client(), http.MethodGet, f.srv.URL+"/api/v1/loot-catalog/items?boss_id=not-present", f.token, nil)
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"data":[]`)) {
		t.Fatalf("empty array: %d %s", status, body)
	}
	for _, tc := range []struct {
		size  int
		mode  string
		count int
	}{{10, "NORMAL", 13}, {10, "HEROIC", 14}, {25, "NORMAL", 18}, {25, "HEROIC", 18}} {
		status, body := do(t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/loot-catalog/items?boss_id=36612&raid_size=%d&mode=%s", f.srv.URL, tc.size, tc.mode), f.token, nil)
		var envelope struct{ Data []lootcatalog.ItemDrop }
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatal(err)
		}
		if status != http.StatusOK || len(envelope.Data) != tc.count {
			t.Fatalf("Marrowgar %d %s: %d %s", tc.size, tc.mode, status, body)
		}
	}
}

func TestCatalogRouterRestartAndMirrorFailure(t *testing.T) {
	f := newCorrectionFixture(t)
	var count int
	if err := f.db.NewRaw("SELECT COUNT(*) FROM loot_catalog_initializations").Scan(context.Background(), &count); err != nil || count != 1 {
		t.Fatalf("cold report: %d %v", count, err)
	}
	if _, err := httpapi.New(f.db, httpapi.Options{AdminUsername: "admin", AdminPassword: "password"}); err != nil {
		t.Fatal(err)
	}
	var reportJSON string
	if err := f.db.NewRaw("SELECT report FROM loot_catalog_initializations ORDER BY id DESC LIMIT 1").Scan(context.Background(), &reportJSON); err != nil {
		t.Fatal(err)
	}
	var report lootcatalog.InitializationReport
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		t.Fatal(err)
	}
	if report.Inserted != 0 || report.Updated != 0 {
		t.Fatal("restart rewrote mirror")
	}
	if _, err := f.db.ExecContext(context.Background(), "DROP TABLE loot_catalog_initializations"); err != nil {
		t.Fatal(err)
	}
	if _, err := httpapi.New(f.db, httpapi.Options{AdminUsername: "admin", AdminPassword: "password"}); err == nil {
		t.Fatal("router accepted failed catalog initialization")
	}
}

func TestCatalogBundledAwardsValidateAndPreserveSnapshots(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.seedPoints(f.first, 70)
	f.request(http.MethodPatch, "", map[string]any{"catalog_instance_id": "631", "raid_size": 10}, http.StatusOK)
	// 39291 is a real bundled drop of another boss, so a bundled item id alone must
	// never be enough: the boss, item, instance, and raid size must match.
	for _, selection := range []map[string]any{{}, {"catalog_boss_id": "999", "item_id": "50759"}, {"catalog_boss_id": "16028", "item_id": "50759"}, {"catalog_boss_id": "36612", "item_id": "39291"}, {"catalog_boss_id": "36612", "item_id": "999999"}} {
		body := map[string]any{"award_method": "ENHANCED", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "request_id": "invalid-catalog-award"}
		for key, value := range selection {
			body[key] = value
		}
		result := f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
		if !bytes.Contains(result, []byte(`"code":"catalog_item_invalid"`)) {
			t.Fatalf("wrong code: %s", result)
		}
	}
	f.points(f.first, 70)
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatalf("invalid award persisted: %v %v", awards, err)
	}
	body := map[string]any{"catalog_boss_id": "36612", "item_id": "50759", "boss": "Untrusted", "item_name": "Untrusted", "award_method": "ENHANCED", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "request_id": "catalog-award"}
	id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	if retry := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated)); retry != id {
		t.Fatal("duplicate catalog award")
	}
	a := f.award(id)
	boss, item, err := lootcatalog.NewService(lootcatalog.NewRepo(f.db)).ValidateDrop(context.Background(), "631", nil, a.CatalogBossID, "50759")
	if err != nil || a.Boss != boss.Name || a.ItemName != item.Name {
		t.Fatalf("canonical snapshot: %+v %v", a, err)
	}
	f.points(f.first, 20)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"notes": "clarify", "reason": "notes"}, http.StatusOK)
	if corrected := f.award(id); corrected.Boss != a.Boss || corrected.ItemName != a.ItemName || corrected.EnhancedCost != 50 {
		t.Fatalf("snapshot changed: %+v", corrected)
	}
}

func TestChestEncounterAwardsAndPermissionBoundary(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.request(http.MethodPatch, "", map[string]any{"catalog_instance_id":"631", "raid_size":10}, http.StatusOK)
	seedAccount(t, f.db, "chest-member", "password", validation.RoleMember)
	member := login(t, f.srv.Client(), f.srv, "chest-member", "password")
	for _, bossID := range []string{"encounter:631:gunship", "36789"} {
		items, err := lootcatalog.NewService(lootcatalog.NewRepo(f.db)).Items(context.Background(), lootcatalog.Filter{BossID:bossID,RaidSize:10,Mode:lootcatalog.ModeNormal})
		if err != nil || len(items)==0 { t.Fatal("missing encounter",bossID,err) }
		body := map[string]any{"catalog_boss_id":bossID,"item_id":items[0].ItemID,"award_method":"SECOND","awarded_to_player_id":f.first,"awarded_to_character_id":f.firstChar,"request_id":"chest-"+bossID}
		status, response := do(t,f.srv.Client(),http.MethodPost,fmt.Sprintf("%s/api/v1/raids/%d/loot",f.srv.URL,f.raidID),member,body)
		if status != http.StatusForbidden { t.Fatalf("member chest award: %d %s",status,response) }
		id := correctionID(t,f.request(http.MethodPost,"/loot",body,http.StatusCreated))
		award := f.award(id)
		if award.CatalogBossID==nil || *award.CatalogBossID!=bossID || award.ItemName!=items[0].Name { t.Fatalf("chest award identity: %+v",award) }
		f.request(http.MethodPatch,fmt.Sprintf("/loot/%d",id),map[string]any{"notes":"Truhenbeute","reason":"Quelle ergänzt"},http.StatusOK)
		if corrected := f.award(id); corrected.Boss!=award.Boss || corrected.ItemID!=award.ItemID { t.Fatal("correction changed encounter snapshot") }
	}
}

func TestCatalogRaidSeriesInheritanceAndPreplannedReuse(t *testing.T) {
	for _, preplanned := range []bool{false, true} {
		t.Run(fmt.Sprint(preplanned), func(t *testing.T) {
			f := newCorrectionFixture(t)
			request := func(method, path string, body any, want int) []byte {
				status, result := do(t, f.srv.Client(), method, f.srv.URL+"/api/v1"+path, f.token, body)
				if status != want {
					t.Fatalf("%s %s: %d %s", method, path, status, result)
				}
				return result
			}
			config := map[string]any{"name": "Explicit series", "raid_leader_id": f.actor, "weekdays": []int{1}, "interval_weeks": 1, "start_time": "20:00", "timezone": "UTC", "catalog_instance_id": "631", "raid_size": 10}
			seriesID := correctionID(t, request(http.MethodPost, "/raid-series", config, http.StatusCreated))
			start := time.Date(2090, 1, 2, 20, 0, 0, 0, time.UTC)
			f.raidID = correctionID(t, request(http.MethodPost, "/raids", map[string]any{"name": "Manual", "raid_leader_id": f.actor, "series_id": seriesID, "scheduled_start": start.Format(time.RFC3339)}, http.StatusCreated))
			rd, err := raid.NewRepo(f.db).Get(context.Background(), f.raidID)
			if err != nil || rd.CatalogInstanceID == nil || *rd.CatalogInstanceID != "631" || rd.RaidSize == nil || *rd.RaidSize != 10 {
				t.Fatalf("inheritance: %+v %v", rd, err)
			}
			f.attend(f.first, f.firstChar)
			var nextID int64
			if preplanned {
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
					t.Fatal("preplanned raid not reused")
				}
			}
			if archive.NextRaid.CatalogInstanceID == nil || *archive.NextRaid.CatalogInstanceID != wantInstance || archive.NextRaid.RaidSize == nil || *archive.NextRaid.RaidSize != 10 {
				t.Fatalf("successor: %+v", archive.NextRaid)
			}
			f.request(http.MethodPost, "/archive", nil, http.StatusOK)
			f.points(f.first, 10)
		})
	}
}
