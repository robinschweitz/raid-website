package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/raid"
)

func TestStaffRecordLootThatWentToNobody(t *testing.T) {
	f := newCorrectionFixture(t)
	member := login(t, f.srv.Client(), f.srv, "correction-first", "password")
	url := fmt.Sprintf("%s/api/v1/raids/%d/unclaimed-loot", f.srv.URL, f.raidID)
	entry := map[string]any{"catalog_boss_id": f.ordinaryBoss, "item_id": f.ordinaryItem, "outcome": "NOBODY", "notes": "liegen gelassen"}

	status, body := do(t, f.srv.Client(), http.MethodPost, url, member, entry)
	if status != http.StatusForbidden {
		t.Fatalf("member: %d %s", status, body)
	}
	for _, bad := range []map[string]any{
		{"item_name": "Irgendwas", "outcome": "LOST"},
		{"outcome": "NOBODY"},
		{"item_name": "Irgendwas", "outcome": "BANK", "catalog_boss_id": "encounter:603:ignis"},
	} {
		status, body = do(t, f.srv.Client(), http.MethodPost, url, f.token, bad)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("invalid %v: %d %s", bad, status, body)
		}
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
