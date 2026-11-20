package httpapi_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestRaidSubstituteScopeAndEligibility(t *testing.T) {
	s, db := setupServer(t)
	c := s.Client()
	seedAccount(t, db, "admin", "pw", validation.RoleAdmin)
	leader := seedAccount(t, db, "leader", "pw", validation.RoleRaidleader)
	assistant := seedAccount(t, db, "assistant", "pw", validation.RoleRaidassistent)
	member := seedAccount(t, db, "member", "pw", validation.RoleMember)
	guest := seedAccount(t, db, "guest", "pw", validation.RoleGuest)
	inactive := seedAccount(t, db, "inactive", "pw", validation.RoleRaidleader)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "UPDATE players SET active=0 WHERE id=?", inactive); err != nil {
		t.Fatal(err)
	}
	admin, lead, sub := login(t, c, s, "admin", "pw"), login(t, c, s, "leader", "pw"), login(t, c, s, "assistant", "pw")
	now := time.Now().UTC()
	start := now.Add(time.Hour)
	makeRaid := func(name string, owner int64) string {
		r := &raid.Raid{Name: name, RaidLeaderID: owner, Status: validation.RaidStatusPlanned, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
		if err := raid.NewRepo(db).Create(ctx, r); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("/api/v1/raids/%d", r.ID)
	}
	own, other := makeRaid("Own", leader), makeRaid("Other", member)
	check := func(method, path, token string, body any, want int) string {
		t.Helper()
		status, data := do(t, c, method, s.URL+path, token, body)
		if status != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, status, want, data)
		}
		return string(data)
	}
	set := func(id int64, bits int) {
		check("PUT", fmt.Sprintf("/api/v1/players/%d/raid-permissions", id), admin, map[string]any{"assigned_only": true, "allowed": bits}, 200)
	}
	set(leader, 3)
	set(assistant, 15)
	note := fmt.Sprintf("/absences/%d", member)
	check("PUT", own+note, admin, map[string]any{"reason": "privat"}, 200)
	if strings.Contains(check("GET", own+"/signups", sub, nil, 200), "privat") {
		t.Fatal("private note exposed before grant")
	}
	path := func(base string, id int64) string { return fmt.Sprintf("%s/substitutes/%d", base, id) }
	grant := func(bits int) any { return map[string]int{"allowed": bits} }
	check("PUT", path(own, member), lead, grant(1), 400)
	check("PUT", path(own, guest), lead, grant(1), 400)
	check("PUT", path(own, inactive), lead, grant(1), 404)
	check("PUT", path(other, assistant), lead, grant(1), 403)
	check("PUT", path(own, assistant), lead, grant(4), 403)
	check("PUT", path(own, assistant), lead, grant(3), 204)
	if !strings.Contains(check("GET", own+"/signups", sub, nil, 200), "privat") {
		t.Fatal("delegated reader did not receive note")
	}
	check("GET", own+"/permissions", lead, nil, 200)
	check("PUT", own+note, sub, map[string]any{"status": "TENTATIVE"}, 200)
	check("PUT", other+note, sub, map[string]any{}, 403)
	check("POST", own+"/loot", sub, map[string]any{}, 403)
	check("POST", own+"/addon-loot-import", sub, map[string]any{}, 403)
	check("POST", own+"/cancel", sub, map[string]any{}, 403)
	set(leader, 2)
	check("PUT", own+note, sub, map[string]any{}, 403)
	check("PUT", path(own, assistant), lead, grant(0), 204)
	if body := check("GET", own+"/permissions", sub, nil, 200); !strings.Contains(body, "outside_scope") {
		t.Fatalf("revoked grant still effective: %s", body)
	}
}
