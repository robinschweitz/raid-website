package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/auth"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestSupportViewBoundedAndAudited(t *testing.T) {
	srv, db := setupServer(t)
	c := srv.Client()
	admin := seedAccount(t, db, "support", "pw", validation.RoleAdmin)
	target := seedAccount(t, db, "target", "pw", validation.RoleMember)
	adminToken := login(t, c, srv, "support", "pw")
	memberToken := login(t, c, srv, "target", "pw")
	ctx := context.Background()
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	rd := &raid.Raid{Name: "Test", RaidLeaderID: admin, Status: validation.RaidStatusPlanned, ScheduledStart: &future, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(db).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	var view string
	request := func(method, path, token, body string, want int) string {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+"/api/v1/"+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Player-View", view)
		addCSRF(req)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, resp.StatusCode, want, data)
		}
		return string(data)
	}
	start := fmt.Sprintf(`{"player_id":%d}`, target)
	request("POST", "support/start", memberToken, start, 403)
	raw := request("POST", "support/start", adminToken, start, 201)
	var result struct {
		Data struct {
			ID string `json:"id"`
		}
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil || result.Data.ID == "" {
		t.Fatal(raw)
	}
	view = result.Data.ID
	if !strings.Contains(request("GET", "auth/me", adminToken, "", 200), `"name":"target"`) {
		t.Fatal("wrong effective player")
	}
	otherSession := login(t, c, srv, "support", "pw")
	request("GET", "auth/me", otherSession, "", 409)
	absence := fmt.Sprintf("raids/%d/absences/%d", rd.ID, target)
	body := `{"reason":"private note"}`
	request("PUT", absence, adminToken, body, 403)
	request("POST", "support/write", adminToken, "", 204)
	request("POST", "polls/1/vote", adminToken, "{}", 403)
	request("POST", "auth/password", adminToken, "{}", 403)
	request("POST", fmt.Sprintf("raids/%d/archive", rd.ID), adminToken, "{}", 403)
	request("PUT", fmt.Sprintf("raids/%d/absences/%d", rd.ID, admin), adminToken, body, 403)
	if strings.Contains(request("PUT", absence, adminToken, body, 200), "private note") {
		t.Fatal("returned private note to represented member")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit_log WHERE entity_type='raid_signup' AND actor_id=? AND represented_player_id=?", admin, target).Scan(&count); err != nil || count != 1 {
		t.Fatalf("missing attribution: %d %v", count, err)
	}
	request("POST", "support/end", adminToken, "", 204)
	request("PUT", absence, adminToken, body, 409)
	view = ""
	if !strings.Contains(request("GET", "auth/me", adminToken, "", 200), `"name":"support"`) {
		t.Fatal("admin not restored")
	}
}
