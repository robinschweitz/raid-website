package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/raid"
	"raid-website/internal/shadowmourne"
	"raid-website/internal/validation"
)

type shadowmourneFixture struct {
	t          *testing.T
	client     *http.Client
	base       string
	member     string
	staff      string
	characters []*character.Character
	audits     func(entityType string) int
}

func (f shadowmourneFixture) request(method, path, token string, body any, want int) []byte {
	f.t.Helper()
	status, response := do(f.t, f.client, method, f.base+path, token, body)
	if status != want {
		f.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, response)
	}
	return response
}

func (f shadowmourneFixture) create(body map[string]any) shadowmourne.Entry {
	f.t.Helper()
	var response struct {
		Data shadowmourne.Entry `json:"data"`
	}
	if err := json.Unmarshal(f.request(http.MethodPost, "/api/v1/shadowmourne", f.staff, body, http.StatusCreated), &response); err != nil {
		f.t.Fatal(err)
	}
	return response.Data
}

func (f shadowmourneFixture) list() []shadowmourne.Entry {
	f.t.Helper()
	var response struct {
		Data []shadowmourne.Entry `json:"data"`
	}
	if err := json.Unmarshal(f.request(http.MethodGet, "/api/v1/shadowmourne", f.member, nil, http.StatusOK), &response); err != nil {
		f.t.Fatal(err)
	}
	return response.Data
}

func listedIDs(entries []shadowmourne.Entry) []int64 {
	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func setupShadowmourne(t *testing.T, classes ...string) (shadowmourneFixture, int64) {
	t.Helper()
	srv, database := setupServer(t)
	ctx := context.Background()
	member := seedAccount(t, database, "member", "pw", validation.RoleMember)
	seedAccount(t, database, "assistant", "pw", validation.RoleRaidassistent)
	now := time.Now().UTC()
	f := shadowmourneFixture{t: t, client: srv.Client(), base: srv.URL, member: login(t, srv.Client(), srv, "member", "pw"), staff: login(t, srv.Client(), srv, "assistant", "pw")}
	f.audits = func(entityType string) int {
		var count int
		if err := database.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ?", entityType).Scan(ctx, &count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	for i, class := range classes {
		c := &character.Character{PlayerID: member, Name: fmt.Sprintf("Sammler %d", i+1), Class: class, MainSpec: "Waffen", SecondSpec: "", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := character.NewRepo(database).Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		f.characters = append(f.characters, c)
	}
	rd := &raid.Raid{Name: "ICC 25", RaidLeaderID: member, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	// Every character except the last one is in the roster.
	for _, c := range f.characters[:len(f.characters)-1] {
		row := &attendance.Attendance{RaidID: rd.ID, PlayerID: member, CharacterID: c.ID, CharacterName: c.Name, Class: c.Class, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
		if err := attendance.NewRepo(database).Create(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	return f, rd.ID
}

func TestShadowmourneListPermissionsAndValidation(t *testing.T) {
	f, _ := setupShadowmourne(t, "Krieger", "Magier", "Paladin")
	f.request(http.MethodGet, "/api/v1/shadowmourne", "", nil, http.StatusUnauthorized)
	f.request(http.MethodPost, "/api/v1/shadowmourne", f.member, map[string]any{"character_id": f.characters[0].ID}, http.StatusForbidden)
	f.request(http.MethodPost, "/api/v1/shadowmourne", f.staff, map[string]any{"character_id": f.characters[1].ID}, http.StatusBadRequest)

	entry := f.create(map[string]any{"character_id": f.characters[0].ID})
	if entry.Stage != shadowmourne.StagePrequest || entry.ShardTarget != shadowmourne.DefaultShardTarget || entry.Priority != 1 || entry.Status != shadowmourne.StatusActive {
		t.Fatalf("defaults not applied: %#v", entry)
	}
	f.request(http.MethodPost, "/api/v1/shadowmourne", f.staff, map[string]any{"character_id": f.characters[0].ID}, http.StatusConflict)
	f.request(http.MethodPatch, fmt.Sprintf("/api/v1/shadowmourne/%d", entry.ID), f.staff, map[string]any{"stage": "SHARDS", "shards_collected": 51, "shard_target": 50}, http.StatusBadRequest)
	f.request(http.MethodPatch, fmt.Sprintf("/api/v1/shadowmourne/%d", entry.ID), f.staff, map[string]any{"stage": "NOPE", "shards_collected": 1, "shard_target": 50}, http.StatusBadRequest)
	f.request(http.MethodPatch, fmt.Sprintf("/api/v1/shadowmourne/%d", entry.ID), f.member, map[string]any{"stage": "SHARDS", "shards_collected": 1, "shard_target": 50}, http.StatusForbidden)
}

func TestShadowmourneListOrderCompletionAndRemoval(t *testing.T) {
	f, _ := setupShadowmourne(t, "Krieger", "Paladin", "Todesritter", "Krieger")
	first := f.create(map[string]any{"character_id": f.characters[0].ID, "stage": "SHARDS", "shards_collected": 3, "shard_target": 4})
	second := f.create(map[string]any{"character_id": f.characters[1].ID})
	third := f.create(map[string]any{"character_id": f.characters[2].ID, "position": 1})
	if got := listedIDs(f.list()); fmt.Sprint(got) != fmt.Sprint([]int64{third.ID, first.ID, second.ID}) {
		t.Fatalf("insert at position 1: %v", got)
	}
	f.request(http.MethodPost, fmt.Sprintf("/api/v1/shadowmourne/%d/move", second.ID), f.staff, map[string]any{"position": 1}, http.StatusOK)
	f.request(http.MethodPost, fmt.Sprintf("/api/v1/shadowmourne/%d/move", second.ID), f.staff, map[string]any{"position": 4}, http.StatusBadRequest)
	if got := listedIDs(f.list()); fmt.Sprint(got) != fmt.Sprint([]int64{second.ID, third.ID, first.ID}) {
		t.Fatalf("move to position 1: %v", got)
	}

	f.request(http.MethodPost, fmt.Sprintf("/api/v1/shadowmourne/%d/complete", first.ID), f.staff, map[string]any{}, http.StatusConflict)
	f.request(http.MethodPatch, fmt.Sprintf("/api/v1/shadowmourne/%d", first.ID), f.staff, map[string]any{"stage": "FINAL", "shards_collected": 4, "shard_target": 4}, http.StatusOK)
	f.request(http.MethodPost, fmt.Sprintf("/api/v1/shadowmourne/%d/complete", first.ID), f.staff, map[string]any{}, http.StatusOK)
	f.request(http.MethodPost, fmt.Sprintf("/api/v1/shadowmourne/%d/remove", third.ID), f.member, nil, http.StatusForbidden)
	f.request(http.MethodPost, fmt.Sprintf("/api/v1/shadowmourne/%d/remove", third.ID), f.staff, nil, http.StatusNoContent)
	f.request(http.MethodPost, fmt.Sprintf("/api/v1/shadowmourne/%d/remove", third.ID), f.staff, nil, http.StatusNotFound)

	entries := f.list()
	if len(entries) != 2 || entries[0].ID != second.ID || entries[0].Priority != 1 || entries[1].ID != first.ID || entries[1].Status != shadowmourne.StatusCompleted || entries[1].CompletedAt == nil {
		t.Fatalf("unexpected list after completion and removal: %#v", entries)
	}
	// 3 creates, 1 move, 1 progress, 1 complete, 1 remove.
	if got := f.audits("shadowmourne_entry"); got != 7 {
		t.Fatalf("entry audit records: %d", got)
	}
	// A removed character can be listed again.
	f.create(map[string]any{"character_id": f.characters[2].ID})
}

func TestShadowmourneRaidTasksSuggestAndAssign(t *testing.T) {
	f, raidID := setupShadowmourne(t, "Krieger", "Paladin", "Todesritter", "Krieger")
	blood := f.create(map[string]any{"character_id": f.characters[0].ID, "stage": "BLOOD"})
	alsoBlood := f.create(map[string]any{"character_id": f.characters[1].ID, "stage": "BLOOD"})
	shards := f.create(map[string]any{"character_id": f.characters[2].ID, "stage": "SHARDS", "shards_collected": 10})
	absent := f.create(map[string]any{"character_id": f.characters[3].ID, "stage": "FROST"})
	path := fmt.Sprintf("/api/v1/raids/%d/shadowmourne", raidID)
	plan := func() shadowmourne.RaidPlan {
		t.Helper()
		var response struct {
			Data shadowmourne.RaidPlan `json:"data"`
		}
		if err := json.Unmarshal(f.request(http.MethodGet, path, f.member, nil, http.StatusOK), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	task := func(p shadowmourne.RaidPlan, stage shadowmourne.Stage) shadowmourne.RaidTask {
		for _, item := range p.Tasks {
			if item.Task == stage {
				return item
			}
		}
		t.Fatalf("task %s missing", stage)
		return shadowmourne.RaidTask{}
	}

	current := plan()
	if !current.Editable || len(current.Candidates) != 3 || len(current.Tasks) != 4 {
		t.Fatalf("unexpected plan: %#v", current)
	}
	if got := task(current, shadowmourne.StageBlood).Suggested; got == nil || got.ID != blood.ID {
		t.Fatalf("blood suggestion should follow list priority: %#v", got)
	}
	if got := task(current, shadowmourne.StageShards).Suggested; got == nil || got.ID != shards.ID {
		t.Fatalf("shard suggestion: %#v", got)
	}
	if got := task(current, shadowmourne.StageFrost).Suggested; got != nil {
		t.Fatalf("a character outside the roster must not be suggested: %#v", got)
	}

	f.request(http.MethodPut, path+"/BLOOD", f.member, map[string]any{"entry_id": alsoBlood.ID}, http.StatusForbidden)
	f.request(http.MethodPut, path+"/SOULS", f.staff, map[string]any{"entry_id": alsoBlood.ID}, http.StatusBadRequest)
	f.request(http.MethodPut, path+"/FROST", f.staff, map[string]any{"entry_id": absent.ID}, http.StatusConflict)
	f.request(http.MethodPut, path+"/BLOOD", f.staff, map[string]any{"entry_id": alsoBlood.ID}, http.StatusOK)
	if got := task(plan(), shadowmourne.StageBlood).Assigned; got == nil || got.ID != alsoBlood.ID {
		t.Fatalf("assignment not stored: %#v", got)
	}
	f.request(http.MethodPut, path+"/BLOOD", f.staff, map[string]any{"entry_id": nil}, http.StatusOK)
	if got := task(plan(), shadowmourne.StageBlood).Assigned; got != nil {
		t.Fatalf("assignment not reset: %#v", got)
	}
	if got := f.audits("shadowmourne_raid_task"); got != 2 {
		t.Fatalf("raid task audit records: %d", got)
	}
	f.request(http.MethodGet, "/api/v1/raids/999999/shadowmourne", f.member, nil, http.StatusNotFound)
}
