package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/raid"
	"raid-website/internal/recruitment"
	"raid-website/internal/validation"
)

// slotEnv is a server with one raid without series, and a RAIDASSISTENT and a MEMBER session.
type slotEnv struct {
	srv    *httptest.Server
	db     *bun.DB
	raidID int64
	url    string
	staff  string
	member string
}

// newSlotEnv prepares the environment for the slot tests (FR-23.4, SC-020).
func newSlotEnv(t *testing.T) slotEnv {
	t.Helper()
	srv, database := setupServer(t)
	staff := seedAccount(t, database, "assistant", "pw", validation.RoleRaidassistent)
	seedAccount(t, database, "member", "pw", validation.RoleMember)
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Einzeln", RaidLeaderID: staff, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database).Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	return slotEnv{
		srv:    srv,
		db:     database,
		raidID: rd.ID,
		url:    fmt.Sprintf("%s/api/v1/raids/%d/recruitment-template", srv.URL, rd.ID),
		staff:  login(t, srv.Client(), srv, "assistant", "pw"),
		member: login(t, srv.Client(), srv, "member", "pw"),
	}
}

// slotSetup is a 5x5 setup of "Leer" with the given value in the first slot.
func slotSetup(first string) [][]string {
	setup := make([][]string, 5)
	for i := range setup {
		setup[i] = []string{"Leer", "Leer", "Leer", "Leer", "Leer"}
	}
	setup[0][0] = first
	return setup
}

func (e slotEnv) auditCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := e.db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ?", "raid_recruitment_template").Scan(context.Background(), &count); err != nil {
		t.Fatal(err)
	}
	return count
}

// TestReqRaid014_AC01: a known class-and-spec value is accepted.
// The issue names "Paladin:Holy"; the editor's spelling of that spec is "Paladin Heilig",
// and only values the editor offers are accepted (see validation.IsRecruitmentSlot).
func TestReqRaid014_AC01(t *testing.T) {
	e := newSlotEnv(t)
	status, body := do(t, e.srv.Client(), http.MethodPut, e.url, e.staff, map[string]any{"text": "Wir suchen", "setup": slotSetup("Paladin Heilig")})
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", status, body)
	}
}

// TestReqRaid014_AC02: markup in a slot is rejected with 422 invalid_slot and nothing is stored.
// A MEMBER gets 403 before the slot is looked at.
func TestReqRaid014_AC02(t *testing.T) {
	e := newSlotEnv(t)
	client := e.srv.Client()
	payload := map[string]any{"text": "Wir suchen", "setup": slotSetup("<b>x</b>")}
	if status, body := do(t, client, http.MethodPut, e.url, e.member, payload); status != http.StatusForbidden {
		t.Fatalf("member: status %d, want 403: %s", status, body)
	}
	status, body := do(t, client, http.MethodPut, e.url, e.staff, payload)
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), `"invalid_slot"`) {
		t.Fatalf("status %d, want 422 invalid_slot: %s", status, body)
	}
	if status, body := do(t, client, http.MethodGet, e.url, e.staff, nil); status != http.StatusOK || !strings.Contains(string(body), `"data":null`) {
		t.Fatalf("template must not be stored: %d %s", status, body)
	}
	if n := e.auditCount(t); n != 0 {
		t.Fatalf("audit entries after rejected save: %d, want 0", n)
	}
}

// TestReqRaid014_AC03: an "Irgendein <Rolle>" placeholder is accepted. The bare "Irgendein"
// is not an editor value (owner decision 2026-10-25), so it answers 422 invalid_slot.
func TestReqRaid014_AC03(t *testing.T) {
	e := newSlotEnv(t)
	client := e.srv.Client()
	status, body := do(t, client, http.MethodPut, e.url, e.staff, map[string]any{"text": "Wir suchen", "setup": slotSetup("Irgendein Heiler")})
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", status, body)
	}
	status, body = do(t, client, http.MethodPut, e.url, e.staff, map[string]any{"text": "Wir suchen", "setup": slotSetup("Irgendein")})
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), `"invalid_slot"`) {
		t.Fatalf("bare Irgendein: status %d, want 422 invalid_slot: %s", status, body)
	}
}

// TestReqRaid014_StoredSlotReport: a stored value that is no longer accepted is reported,
// and the report changes no stored data (FR-23.4).
func TestReqRaid014_StoredSlotReport(t *testing.T) {
	e := newSlotEnv(t)
	ctx := context.Background()
	setupJSON := `[["Maintank","<b>x</b>","Leer","Leer","Leer"]]`
	if _, err := e.db.ExecContext(ctx, "INSERT INTO raid_recruitment_templates (raid_id, text, setup_json, updated_by, updated_at) VALUES (?, ?, ?, ?, ?)", e.raidID, "Alt", setupJSON, 1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	invalid, err := recruitment.NewService(e.db).InvalidStoredSlots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := recruitment.InvalidStoredSlot{Scope: "raid_id", ID: e.raidID, Group: 1, Slot: 2, Value: "<b>x</b>"}
	if len(invalid) != 1 || invalid[0] != want {
		t.Fatalf("report: %+v, want [%+v]", invalid, want)
	}
	var stored string
	if err := e.db.NewSelect().TableExpr("raid_recruitment_templates").ColumnExpr("setup_json").Where("raid_id = ?", e.raidID).Scan(ctx, &stored); err != nil || stored != setupJSON {
		t.Fatalf("stored template changed: %q %v", stored, err)
	}
}
