package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func snapshotResponse(t *testing.T, body []byte) attendance.Attendance {
	t.Helper()
	var result struct {
		Data attendance.Attendance `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result.Data
}

func TestRosterSnapshotsPreserveHistoryAndAuditedRaidReality(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	row := snapshotResponse(t, f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.first, "character_id": f.firstChar, "role": "DD"}, http.StatusCreated))
	if row.CharacterName != "Correction0" || row.Class != "Magier" || row.Spec != "Fire" {
		t.Fatalf("defaults: %+v", row)
	}
	name, spec := "Renamed", "New live spec"
	if _, err := character.NewService(character.NewRepo(f.db)).Correct(ctx, f.actor, f.firstChar, character.Patch{Name: &name, MainSpec: &spec, Reason: "live rename and respec"}); err != nil {
		t.Fatal(err)
	}
	stored, err := attendance.NewRepo(f.db).Get(ctx, row.ID)
	if err != nil || stored.CharacterName != row.CharacterName || stored.Spec != row.Spec {
		t.Fatalf("history changed: %+v %v", stored, err)
	}
	suffix := fmt.Sprintf("/attendance/%d", row.ID)
	row = snapshotResponse(t, f.request(http.MethodPatch, suffix, map[string]any{"role": "HEAL", "class": "priester", "spec": "Raid-specific hybrid", "character_name": "Playedname", "reason": "actual raid roster"}, http.StatusOK))
	if row.CharacterName != "Playedname" || row.Class != "Priester" || row.Spec != "Raid-specific hybrid" || row.Role != validation.RoleHeal {
		t.Fatalf("edit: %+v", row)
	}
	row = snapshotResponse(t, f.request(http.MethodPatch, suffix, map[string]any{"notes": "late", "reason": "note correction"}, http.StatusOK))
	if row.Spec != "Raid-specific hybrid" || row.CharacterName != "Playedname" || row.Class != "Priester" {
		t.Fatalf("partial patch reset: %+v", row)
	}
	live, err := character.NewRepo(f.db).Get(ctx, f.firstChar)
	if err != nil || live.Name != name || live.MainSpec != spec || live.Class != "Mage" {
		t.Fatalf("live character changed: %+v %v", live, err)
	}
	row = snapshotResponse(t, f.request(http.MethodPatch, suffix, map[string]any{"player_id": f.second, "character_id": f.secondChar, "reason": "different participant"}, http.StatusOK))
	if row.CharacterName != "Correction1" || row.Class != "Magier" || row.Spec != "Fire" {
		t.Fatalf("reference defaults: %+v", row)
	}
	row = snapshotResponse(t, f.request(http.MethodPatch, suffix, map[string]any{"player_id": f.first, "character_id": f.firstChar, "spec": "Fire", "reason": "explicit played spec"}, http.StatusOK))
	if row.CharacterName != name || row.Spec != "Fire" {
		t.Fatalf("explicit spec lost: %+v", row)
	}
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.Status = validation.RaidStatusCompleted
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPatch, suffix, map[string]any{"spec": "Historical correction", "reason": "completed compatibility"}, http.StatusOK)
	f.points(f.first, 0)
	now := time.Now().UTC()
	rd.ArchivedAt = &now
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPatch, suffix, map[string]any{"spec": "Forbidden", "reason": "archived"}, http.StatusConflict)
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "DD"}, http.StatusConflict)
	status, body := do(t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/attendance", f.srv.URL, f.raidID), f.token, nil)
	var list struct {
		Data []attendance.Attendance `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || len(list.Data) != 1 || list.Data[0].Spec != "Historical correction" || list.Data[0].CharacterName != name {
		t.Fatalf("archived read: %d %s", status, body)
	}
	logs, err := audit.NewRepo(f.db).ListByEntity(ctx, "attendance", fmt.Sprint(row.ID))
	if err != nil || len(logs) != 5 {
		t.Fatalf("audit: %+v %v", logs, err)
	}
	var before, after attendance.Attendance
	if err := json.Unmarshal([]byte(logs[0].BeforeJSON), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(logs[0].AfterJSON), &after); err != nil {
		t.Fatal(err)
	}
	if before.CharacterName != "Correction0" || before.Spec != "Fire" || after.CharacterName != "Playedname" || after.Spec != "Raid-specific hybrid" {
		t.Fatalf("audit snapshots: %+v %+v", before, after)
	}
}

func TestRosterSnapshotValidationPermissionsAndRollback(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	base := map[string]any{"player_id": f.first, "character_id": f.firstChar, "role": "DD"}
	for _, field := range []string{"character_name", "class", "spec"} {
		body := map[string]any{"player_id": f.first, "character_id": f.firstChar, "role": "DD", field: " "}
		f.request(http.MethodPost, "/attendance", body, http.StatusBadRequest)
	}
	row := snapshotResponse(t, f.request(http.MethodPost, "/attendance", base, http.StatusCreated))
	suffix := fmt.Sprintf("/attendance/%d", row.ID)
	before, err := attendance.NewRepo(f.db).Get(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, patch := range []map[string]any{
		{"spec": "", "reason": "blank"}, {"class": "unknown", "reason": "invalid"},
		{"character_name": " ", "reason": "blank"}, {"spec": "Frost", "reason": " "},
		{"player_id": f.second, "reason": "wrong owner"},
	} {
		f.request(http.MethodPatch, suffix, patch, http.StatusBadRequest)
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		url := fmt.Sprintf("%s/api/v1/raids/%d/attendance", f.srv.URL, f.raidID)
		if method == http.MethodPatch {
			url += fmt.Sprintf("/%d", row.ID)
		}
		status, body := do(t, f.srv.Client(), method, url, f.memberToken, base)
		if status != http.StatusForbidden {
			t.Fatalf("permission: %d %s", status, body)
		}
	}
	if _, err := f.db.ExecContext(ctx, "CREATE TRIGGER reject_snapshot_audit BEFORE INSERT ON audit_log WHEN NEW.entity_type = 'attendance' BEGIN SELECT RAISE(ABORT, 'injected audit failure'); END"); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPatch, suffix, map[string]any{"spec": "Frost", "reason": "audit must succeed"}, http.StatusInternalServerError)
	after, err := attendance.NewRepo(f.db).Get(ctx, row.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("failed writes changed row: before %+v after %+v error %v", before, after, err)
	}
	f.points(f.first, 0)
}

func TestRosterUnknownHistoryIsNotBackfilled(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	row := &attendance.Attendance{RaidID: f.raidID, PlayerID: f.first, CharacterID: f.firstChar, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
	if err := attendance.NewRepo(f.db).Create(ctx, row); err != nil {
		t.Fatal(err)
	}
	patched := snapshotResponse(t, f.request(http.MethodPatch, fmt.Sprintf("/attendance/%d", row.ID), map[string]any{"notes": "old row", "reason": "preserve unknown history"}, http.StatusOK))
	if patched.CharacterName != "" || patched.Class != "" || patched.Spec != "" {
		t.Fatalf("fabricated history: %+v", patched)
	}
}

func TestRosterPlannedCreationSpecDefaultsOnlyWhenAbsent(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.Status = validation.RaidStatusPlanned
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	c, err := character.NewRepo(f.db).Get(ctx, f.firstChar)
	if err != nil {
		t.Fatal(err)
	}
	c.MainSpec = ""
	if err := character.NewRepo(f.db).Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.first, "character_id": f.firstChar, "role": "DD"}, http.StatusBadRequest)
	row := snapshotResponse(t, f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.first, "character_id": f.firstChar, "role": "DD", "spec": "Played hybrid"}, http.StatusCreated))
	if row.Spec != "Played hybrid" {
		t.Fatalf("explicit spec: %+v", row)
	}
	suffix := fmt.Sprintf("/attendance/%d", row.ID)
	f.request(http.MethodPatch, suffix, map[string]any{"spec": "Frost"}, http.StatusBadRequest)
	row = snapshotResponse(t, f.request(http.MethodPatch, suffix, map[string]any{"spec": "Frost", "reason": "planned roster edit"}, http.StatusOK))
	if row.Spec != "Frost" {
		t.Fatalf("planned edit: %+v", row)
	}
}
