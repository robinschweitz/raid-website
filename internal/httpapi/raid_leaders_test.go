package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"raid-website/internal/validation"
)

// FR-10.17: a raid lists one or more raid leaders; staff set them, members only read.
func TestRaidLeaders(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	leaderID := seedAccount(t, bdb, "leader", "pw", validation.RoleRaidleader)
	assistantID := seedAccount(t, bdb, "assistant", "pw", validation.RoleRaidassistent)
	memberID := seedAccount(t, bdb, "member", "pw", validation.RoleMember)
	staffToken := login(t, client, srv, "leader", "pw")
	memberToken := login(t, client, srv, "member", "pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", staffToken, map[string]any{"name": "ICC", "raid_leader_id": leaderID})
	if status != http.StatusCreated {
		t.Fatalf("create raid: status %d body %s", status, body)
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/raids/%d/leaders", srv.URL, created.Data.ID)
	leaders := func() []int64 {
		t.Helper()
		status, body := do(t, client, http.MethodGet, url, memberToken, nil)
		if status != http.StatusOK {
			t.Fatalf("read leaders: status %d body %s", status, body)
		}
		var out struct {
			Data []int64 `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out.Data
	}
	if got := leaders(); len(got) != 1 || got[0] != leaderID {
		t.Fatalf("initial leaders: %v", got)
	}
	if status, _ := do(t, client, http.MethodPut, url, memberToken, map[string]any{"player_ids": []int64{memberID}}); status != http.StatusForbidden {
		t.Fatalf("member sets leaders: want 403 got %d", status)
	}
	for _, bad := range [][]int64{{}, {leaderID, leaderID}, {leaderID, 99999}} {
		if status, _ := do(t, client, http.MethodPut, url, staffToken, map[string]any{"player_ids": bad}); status != http.StatusBadRequest {
			t.Fatalf("invalid leaders %v: want 400 got %d", bad, status)
		}
	}
	if status, body := do(t, client, http.MethodPut, url, staffToken, map[string]any{"player_ids": []int64{assistantID, leaderID, memberID}}); status != http.StatusOK {
		t.Fatalf("set leaders: status %d body %s", status, body)
	}
	if got := leaders(); len(got) != 3 || got[0] != assistantID || got[1] != leaderID || got[2] != memberID {
		t.Fatalf("leaders after set: %v", got)
	}
	var first int64
	if err := bdb.NewSelect().Table("raids").Column("raid_leader_id").Where("id = ?", created.Data.ID).Scan(context.Background(), &first); err != nil || first != assistantID {
		t.Fatalf("raid_leader_id stays the first leader: %d %v", first, err)
	}
	// Archived, completed raids can be corrected with a reason (FR-13.11, FR-26.5).
	if _, err := bdb.ExecContext(context.Background(), "UPDATE raids SET status = 'COMPLETED', completed_at = CURRENT_TIMESTAMP, archived_at = CURRENT_TIMESTAMP WHERE id = ?", created.Data.ID); err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, client, http.MethodPut, url, staffToken, map[string]any{"player_ids": []int64{leaderID}}); status != http.StatusBadRequest || !containsCode(body, "reason_required") {
		t.Fatalf("archived raid without reason: want 400 reason_required got %d %s", status, body)
	}
	if status, body := do(t, client, http.MethodPut, url, staffToken, map[string]any{"player_ids": []int64{leaderID, assistantID}, "reason": "Co-Leitung nachgetragen"}); status != http.StatusOK {
		t.Fatalf("archived raid with reason: status %d body %s", status, body)
	}
	if got := leaders(); len(got) != 2 || got[0] != leaderID || got[1] != assistantID {
		t.Fatalf("leaders of archived raid: %v", got)
	}
	var reason string
	if err := bdb.NewSelect().Table("audit_log").Column("reason").Where("entity_type = 'raid' AND action = 'set_leaders'").Order("id DESC").Limit(1).Scan(context.Background(), &reason); err != nil || reason != "Co-Leitung nachgetragen" {
		t.Fatalf("audit reason: %q %v", reason, err)
	}
	if _, err := bdb.ExecContext(context.Background(), "UPDATE raids SET status = 'PLANNED', completed_at = NULL, archived_at = NULL WHERE id = ?", created.Data.ID); err != nil {
		t.Fatal(err)
	}
	if status, _ := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/cancel", srv.URL, created.Data.ID), staffToken, nil); status != http.StatusOK {
		t.Fatalf("cancel raid: %d", status)
	}
	if status, _ := do(t, client, http.MethodPut, url, staffToken, map[string]any{"player_ids": []int64{leaderID}}); status != http.StatusConflict {
		t.Fatalf("cancelled raid leaders: want 409 got %d", status)
	}
}
