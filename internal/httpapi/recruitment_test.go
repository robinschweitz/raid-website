package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestRecruitmentTemplateIsSharedPerSeriesAndRestrictedToStaff(t *testing.T) {
	srv, database := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	seedAccount(t, database, "member", "pw", validation.RoleMember)
	staff := seedAccount(t, database, "assistant", "pw", validation.RoleRaidassistent)
	seedAccount(t, database, "leader", "pw", validation.RoleRaidleader)
	memberToken := login(t, client, srv, "member", "pw")
	staffToken := login(t, client, srv, "assistant", "pw")
	leaderToken := login(t, client, srv, "leader", "pw")
	now := time.Now().UTC()
	series := &raidseries.Series{Name: "Serie", RaidLeaderID: staff, Weekdays: []int{0}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
	if err := raidseries.NewService(raidseries.NewRepo(database)).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, seriesID *int64) int64 {
		rd := &raid.Raid{Name: name, RaidLeaderID: staff, SeriesID: seriesID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
		if err := raid.NewRepo(database).Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
		return rd.ID
	}
	weekOne, weekTwo, single := mk("W1", &series.ID), mk("W2", &series.ID), mk("Einzeln", nil)
	url := func(id int64) string { return fmt.Sprintf("%s/api/v1/raids/%d/recruitment-template", srv.URL, id) }
	check := func(method, u, token string, body any, want int) string {
		t.Helper()
		status, b := do(t, client, method, u, token, body)
		if status != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, u, status, want, b)
		}
		return string(b)
	}
	setup := make([][]string, 5)
	for i := range setup {
		setup[i] = []string{"Maintank", "Offtank", "Leer", "Irgendein MDD", "Irgendein RDD"}
	}
	good := map[string]any{"text": "Wir suchen {{TANK}}", "setup": setup}
	check(http.MethodGet, url(weekOne), "", nil, 401)
	check(http.MethodGet, url(weekOne), memberToken, nil, 403)
	check(http.MethodPut, url(weekOne), memberToken, good, 403)
	if body := check(http.MethodGet, url(weekOne), staffToken, nil, 200); !strings.Contains(body, `"data":null`) {
		t.Fatalf("expected no template yet: %s", body)
	}
	check(http.MethodPut, url(weekOne), staffToken, map[string]any{"text": " ", "setup": setup}, 400)
	check(http.MethodPut, url(weekOne), staffToken, map[string]any{"text": "x", "setup": setup[:4]}, 400)
	check(http.MethodPut, url(weekOne), staffToken, map[string]any{"text": strings.Repeat("ä", 2001), "setup": setup}, 400)
	check(http.MethodPut, url(weekOne), staffToken, good, 200)
	// Another raid leader sees it for the next week's raid of the same series.
	if body := check(http.MethodGet, url(weekTwo), leaderToken, nil, 200); !strings.Contains(body, "Wir suchen {{TANK}}") {
		t.Fatalf("series template missing: %s", body)
	}
	// A raid without series has its own template; updating replaces, not duplicates.
	if body := check(http.MethodGet, url(single), staffToken, nil, 200); !strings.Contains(body, `"data":null`) {
		t.Fatalf("single raid must not inherit: %s", body)
	}
	good["text"] = "Neu {{HEAL}}"
	check(http.MethodPut, url(weekTwo), leaderToken, good, 200)
	if body := check(http.MethodGet, url(weekOne), staffToken, nil, 200); !strings.Contains(body, "Neu {{HEAL}}") {
		t.Fatalf("update not shared: %s", body)
	}
	check(http.MethodGet, url(99999), staffToken, nil, 404)
	var count int
	if err := database.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ?", "raid_recruitment_template").Scan(ctx, &count); err != nil || count != 2 {
		t.Fatalf("audit entries: %d %v", count, err)
	}
}
