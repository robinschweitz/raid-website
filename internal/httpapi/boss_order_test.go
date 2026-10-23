package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestAdminSetsTheBossOrderOfAnInstance(t *testing.T) {
	srv, db := setupServer(t)
	adminID := seedAccount(t, db, "order-admin", "password", validation.RoleAdmin)
	seedAccount(t, db, "order-member", "password", validation.RoleMember)
	admin := login(t, srv.Client(), srv, "order-admin", "password")
	member := login(t, srv.Client(), srv, "order-member", "password")

	bosses := func() []string {
		status, body := do(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/loot-catalog/bosses?instance_id=631", member, nil)
		if status != http.StatusOK {
			t.Fatalf("bosses: %d %s", status, body)
		}
		var list struct{ Data []struct{ ID string } }
		if err := json.Unmarshal(body, &list); err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(list.Data))
		for i, boss := range list.Data {
			ids[i] = boss.ID
		}
		return ids
	}
	before := bosses()
	if len(before) < 3 {
		t.Fatalf("Icecrown needs several bosses, got %v", before)
	}
	reversed := make([]string, len(before))
	for i, id := range before {
		reversed[len(before)-1-i] = id
	}
	url := srv.URL + "/api/v1/loot-catalog/boss-order"

	status, body := do(t, srv.Client(), http.MethodPut, url, member, map[string]any{"instance_id": "631", "boss_ids": reversed})
	if status != http.StatusForbidden {
		t.Fatalf("member: %d %s", status, body)
	}
	for _, bad := range [][]string{reversed[1:], append(append([]string{}, reversed[1:]...), reversed[1]), append([]string{"encounter:631:nobody"}, reversed[1:]...)} {
		status, body = do(t, srv.Client(), http.MethodPut, url, admin, map[string]any{"instance_id": "631", "boss_ids": bad})
		if status != http.StatusUnprocessableEntity || !json.Valid(body) {
			t.Fatalf("incomplete order %v: %d %s", bad, status, body)
		}
	}
	status, body = do(t, srv.Client(), http.MethodPut, url, admin, map[string]any{"instance_id": "631", "boss_ids": reversed})
	if status != http.StatusOK {
		t.Fatalf("admin: %d %s", status, body)
	}
	after := bosses()
	for i := range reversed {
		if after[i] != reversed[i] {
			t.Fatalf("want %v, got %v", reversed, after)
		}
	}
	entries, err := audit.NewRepo(db).ListByEntity(context.Background(), "boss_order", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Action != "update" {
		t.Fatalf("want one audit entry, got %+v", entries)
	}

	// A raid's loot arrives in boss order, so the page's boss groups follow it.
	ctx := context.Background()
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "ICC order", RaidLeaderID: adminID, Status: validation.RaidStatusCompleted, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(db).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	for i, bossID := range []string{before[0], before[len(before)-1], ""} {
		award := &loot.Award{RaidID: rd.ID, Boss: "Boss " + bossID, ItemID: "1", ItemName: "Item", AwardedToPlayerID: adminID,
			AwardedByPlayerID: adminID, Source: "legacy_import", AwardMethod: loot.Main, AwardedAt: now.Add(time.Duration(i) * time.Minute), CreatedAt: now, UpdatedAt: now}
		if bossID != "" {
			id := bossID
			award.CatalogBossID = &id
		}
		if err := loot.NewRepo(db).CreateAward(ctx, award); err != nil {
			t.Fatal(err)
		}
	}
	status, body = do(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/raids/"+strconv.FormatInt(rd.ID, 10)+"/loot", member, nil)
	var awards struct {
		Data []struct {
			CatalogBossID *string `json:"catalog_boss_id"`
		}
	}
	if err := json.Unmarshal(body, &awards); err != nil || status != http.StatusOK || len(awards.Data) != 3 {
		t.Fatalf("loot: %d %s", status, body)
	}
	if *awards.Data[0].CatalogBossID != before[len(before)-1] || *awards.Data[1].CatalogBossID != before[0] || awards.Data[2].CatalogBossID != nil {
		t.Fatalf("loot not in boss order: %s", body)
	}
}
