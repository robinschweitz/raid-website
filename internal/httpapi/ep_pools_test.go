package httpapi_test

import (
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
	"raid-website/internal/validation"
)

// FR-16.10: pools are public to read, Admin-only to write, unique by name.
func TestReqEP_PoolsReadPublicWriteAdmin(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "pooladmin", "password", validation.RoleAdmin)
	seedAccount(t, bdb, "poolleader", "password", validation.RoleRaidleader)
	admin := login(t, client, srv, "pooladmin", "password")
	leader := login(t, client, srv, "poolleader", "password")
	url := srv.URL + "/api/v1/ep-pools"

	status, body := do(t, client, http.MethodGet, url, "", nil)
	if status != http.StatusOK || string(body) != "{\"data\":[{\"id\":1,\"name\":\"ICC\"}]}\n" {
		t.Fatalf("public pool list: %d %s", status, body)
	}
	if status, _ = do(t, client, http.MethodPost, url, "", map[string]any{"name": "Ulduar"}); status != http.StatusUnauthorized {
		t.Fatalf("anonymous create: %d", status)
	}
	if status, _ = do(t, client, http.MethodPost, url, leader, map[string]any{"name": "Ulduar"}); status != http.StatusForbidden {
		t.Fatalf("raid leader create: %d", status)
	}
	if status, _ = do(t, client, http.MethodPatch, url+"/1", leader, map[string]any{"name": "X"}); status != http.StatusForbidden {
		t.Fatalf("raid leader rename: %d", status)
	}
	status, body = do(t, client, http.MethodPost, url, admin, map[string]any{"name": " Ulduar "})
	if status != http.StatusCreated || string(body) != "{\"data\":{\"id\":2,\"name\":\"Ulduar\"}}\n" {
		t.Fatalf("admin create: %d %s", status, body)
	}
	if status, body = do(t, client, http.MethodPost, url, admin, map[string]any{"name": "Ulduar"}); status != http.StatusConflict || errorCode(t, body) != "ep_pool_name_taken" {
		t.Fatalf("duplicate name: %d %s", status, body)
	}
	if status, body = do(t, client, http.MethodPost, url, admin, map[string]any{"name": "  "}); status != http.StatusUnprocessableEntity || errorCode(t, body) != "ep_pool_name_invalid" {
		t.Fatalf("blank name: %d %s", status, body)
	}
	if status, body = do(t, client, http.MethodPatch, url+"/2", admin, map[string]any{"name": "Ulduar & ToC"}); status != http.StatusOK {
		t.Fatalf("rename: %d %s", status, body)
	}
	if status, body = do(t, client, http.MethodPatch, url+"/99", admin, map[string]any{"name": "Nope"}); status != http.StatusNotFound || errorCode(t, body) != "ep_pool_not_found" {
		t.Fatalf("rename unknown: %d %s", status, body)
	}
	var audits int
	if err := bdb.NewRaw("SELECT COUNT(*) FROM audit_log WHERE entity_type = 'ep_pool'").Scan(context.Background(), &audits); err != nil || audits != 2 {
		t.Fatalf("pool audit entries: %d %v", audits, err)
	}
}

// FR-16.2, FR-16.11, FR-17.2: a raid copies pool and award from its series,
// archiving awards in that pool, and a raid without a pool takes no locks.
func TestReqEP_SeriesPoolAwardAndNoPoolRaids(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	adminID := seedAccount(t, bdb, "epadmin", "password", validation.RoleAdmin)
	memberID := seedAccount(t, bdb, "epmember", "password", validation.RoleMember)
	admin := login(t, client, srv, "epadmin", "password")
	member := login(t, client, srv, "epmember", "password")
	now := time.Now().UTC()
	char := &character.Character{PlayerID: memberID, Name: "Pooler", Class: "Krieger", MainSpec: "Waffen", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(bdb).Create(ctx, char); err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/ep-pools", admin, map[string]any{"name": "Ulduar"}); status != http.StatusCreated {
		t.Fatalf("create pool: %d %s", status, body)
	}
	series := func(name string, pool int64, award int) int64 {
		t.Helper()
		status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raid-series", admin, map[string]any{"name": name, "raid_leader_id": adminID, "weekdays": []int{1}, "interval_weeks": 1, "start_time": "20:00", "timezone": "Europe/Berlin", "ep_pool_id": pool, "ep_award": award})
		if status != http.StatusCreated {
			t.Fatalf("create series %s: %d %s", name, status, body)
		}
		var out struct {
			Data struct {
				ID       int64  `json:"id"`
				EPPoolID *int64 `json:"ep_pool_id"`
				EPAward  int    `json:"ep_award"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if out.Data.EPAward != award || (pool == 0) != (out.Data.EPPoolID == nil) {
			t.Fatalf("series %s EP not stored: %s", name, body)
		}
		return out.Data.ID
	}
	ulduar := series("Ulduar Mittwoch", 2, 7)
	fun := series("Spaßraid", 0, 10)
	if status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raid-series", admin, map[string]any{"name": "Bad", "raid_leader_id": adminID, "weekdays": []int{1}, "interval_weeks": 1, "start_time": "20:00", "timezone": "Europe/Berlin", "ep_award": 101}); status == http.StatusCreated {
		t.Fatalf("award above 100 accepted: %s", body)
	}
	if status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raid-series", admin, map[string]any{"name": "Bad", "raid_leader_id": adminID, "weekdays": []int{1}, "interval_weeks": 1, "start_time": "20:00", "timezone": "Europe/Berlin", "ep_pool_id": 99}); status != http.StatusNotFound || errorCode(t, body) != "ep_pool_not_found" {
		t.Fatalf("unknown pool: %d %s", status, body)
	}

	start := now.AddDate(0, 0, -1).Format(time.RFC3339)
	raidFor := func(seriesID int64) int64 {
		t.Helper()
		status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", admin, map[string]any{"raid_leader_id": adminID, "series_id": seriesID, "scheduled_start": start, "status": "OPEN"})
		if status != http.StatusCreated {
			t.Fatalf("create raid: %d %s", status, body)
		}
		var out struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		row := &attendance.Attendance{RaidID: out.Data.ID, PlayerID: memberID, CharacterID: char.ID, CharacterName: char.Name, Class: char.Class, Spec: "Waffen", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
		if err := attendance.NewRepo(bdb).Create(ctx, row); err != nil {
			t.Fatal(err)
		}
		return out.Data.ID
	}
	ulduarRaid := raidFor(ulduar)
	funRaid := raidFor(fun)

	// A raid without a pool takes no locks (FR-17.2).
	status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/locks", srv.URL, memberID), member, map[string]any{"raid_id": funRaid, "item_name": "Val'anyr"})
	if status != http.StatusConflict || errorCode(t, body) != "raid_has_no_ep_pool" {
		t.Fatalf("lock in raid without pool: %d %s", status, body)
	}

	for _, id := range []int64{ulduarRaid, funRaid} {
		if status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/archive", srv.URL, id), admin, nil); status != http.StatusOK {
			t.Fatalf("archive raid %d: %d %s", id, status, body)
		}
	}
	points := enhancepoints.NewRepo(bdb)
	for pool, want := range map[int64]int{1: 0, 2: 7} {
		if got, err := points.Balance(ctx, memberID, pool); err != nil || got != want {
			t.Fatalf("balance in pool %d: %d %v, want %d", pool, got, err, want)
		}
	}
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/points?pool_id=2", "", nil)
	if status != http.StatusOK || !jsonContains(body, fmt.Sprintf(`"player_id":%d,"player_name":"epmember","balance":7`, memberID)) {
		t.Fatalf("public points of pool 2: %d %s", status, body)
	}
	if status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/points?pool_id=99", "", nil); status != http.StatusNotFound {
		t.Fatalf("unknown pool points: %d %s", status, body)
	}
	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points?pool_id=2", srv.URL, memberID), member, nil)
	if status != http.StatusOK || !jsonContains(body, `"balance":7,`) || !jsonContains(body, `"pool_id":2,`) {
		t.Fatalf("own points in pool 2: %d %s", status, body)
	}
	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, memberID), member, nil)
	if status != http.StatusOK || !jsonContains(body, `"balance":0,`) || !jsonContains(body, `"pool_id":1,"transactions":[]`) {
		t.Fatalf("own points in default pool: %d %s", status, body)
	}
}

// FR-16.11: points already booked in a raid's pool fix it against a series change.
func TestReqEP_RaidPoolLockedByExistingPoints(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	adminID := seedAccount(t, bdb, "lockadmin", "password", validation.RoleAdmin)
	admin := login(t, client, srv, "lockadmin", "password")
	create := func(body map[string]any) int64 {
		t.Helper()
		body["raid_leader_id"], body["weekdays"], body["interval_weeks"], body["start_time"], body["timezone"] = adminID, []int{2}, 1, "20:00", "Europe/Berlin"
		status, raw := do(t, client, http.MethodPost, srv.URL+"/api/v1/raid-series", admin, body)
		if status != http.StatusCreated {
			t.Fatalf("series: %d %s", status, raw)
		}
		var out struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out.Data.ID
	}
	icc := create(map[string]any{"name": "ICC"})
	same := create(map[string]any{"name": "ICC zwei", "ep_pool_id": 1, "ep_award": 10})
	none := create(map[string]any{"name": "Ohne", "ep_pool_id": 0})
	start := time.Now().UTC().AddDate(0, 0, 3).Format(time.RFC3339)
	status, raw := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", admin, map[string]any{"raid_leader_id": adminID, "series_id": icc, "scheduled_start": start, "status": "OPEN"})
	if status != http.StatusCreated {
		t.Fatalf("raid: %d %s", status, raw)
	}
	var created struct {
		Data struct {
			ID       int64  `json:"id"`
			EPPoolID *int64 `json:"ep_pool_id"`
			EPAward  int    `json:"ep_award"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.Data.EPPoolID == nil || *created.Data.EPPoolID != 1 || created.Data.EPAward != 10 {
		t.Fatalf("raid did not copy the default pool: %s %v", raw, err)
	}
	if err := enhancepoints.NewRepo(bdb).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: adminID, PoolID: 1, RaidID: &created.Data.ID, Amount: -5, Type: "enhanced_loot", Reason: "test", ActorID: adminID, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/raids/%d", srv.URL, created.Data.ID)
	if status, raw = do(t, client, http.MethodPatch, url, admin, map[string]any{"series_id": none}); status != http.StatusConflict || errorCode(t, raw) != "raid_ep_locked" {
		t.Fatalf("pool change with points: %d %s", status, raw)
	}
	if status, raw = do(t, client, http.MethodPatch, url, admin, map[string]any{"series_id": same}); status != http.StatusOK {
		t.Fatalf("series change with the same pool and award: %d %s", status, raw)
	}
}

// jsonContains reports whether a compact JSON body contains a fragment.
func jsonContains(body []byte, fragment string) bool {
	return strings.Contains(string(body), fragment)
}
