package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"raid-website/internal/audit"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

type killResponse struct {
	Data struct {
		ID            int64  `json:"id"`
		RaidID        int64  `json:"raid_id"`
		CatalogBossID string `json:"catalog_boss_id"`
		Boss          string `json:"boss"`
		Difficulty    string `json:"difficulty"`
	} `json:"data"`
}

// killModes returns the difficulties the bundled catalog has for the fixture's
// ordinary boss at 25 players, and one it lacks.
func killModes(t *testing.T, f *correctionFixture) (offered []string, missing string) {
	t.Helper()
	_, modes, err := lootcatalog.NewService(lootcatalog.NewRepo(f.db)).BossModes(context.Background(), "631", f.ordinaryBoss, 25)
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, mode := range modes {
		offered = append(offered, string(mode))
		has[string(mode)] = true
	}
	for _, mode := range []string{"NORMAL", "HEROIC", "HARDMODE"} {
		if !has[mode] {
			missing = mode
		}
	}
	if !has["HEROIC"] || missing == "" {
		t.Fatalf("fixture boss needs HEROIC and a missing difficulty, got %v", offered)
	}
	return offered, missing
}

func (f *correctionFixture) recordKill(difficulty string, want int) killResponse {
	f.t.Helper()
	var out killResponse
	body := f.request(http.MethodPost, "/kills", map[string]any{"catalog_boss_id": f.ordinaryBoss, "difficulty": difficulty}, want)
	if want == http.StatusCreated {
		if err := json.Unmarshal(body, &out); err != nil {
			f.t.Fatal(err)
		}
	}
	return out
}

func killAudit(t *testing.T, f *correctionFixture, killID int64) []audit.Log {
	t.Helper()
	entries, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "raid_boss_kill", fmt.Sprint(killID))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestReqRaid015_AC01(t *testing.T) {
	f := newCorrectionFixture(t)
	killModes(t, f)
	created := f.recordKill("HEROIC", http.StatusCreated).Data
	if created.ID == 0 || created.RaidID != f.raidID || created.CatalogBossID != f.ordinaryBoss || created.Boss == "" || created.Difficulty != "HEROIC" {
		t.Fatalf("stored kill: %+v", created)
	}
	status, body := do(t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/kills", f.srv.URL, f.raidID), f.memberToken, nil)
	var list struct {
		Data []struct {
			ID         int64  `json:"id"`
			Difficulty string `json:"difficulty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil || status != http.StatusOK || len(list.Data) != 1 || list.Data[0].ID != created.ID || list.Data[0].Difficulty != "HEROIC" {
		t.Fatalf("members read the kill list: %d %s", status, body)
	}
	entries := killAudit(t, f, created.ID)
	if len(entries) != 1 || entries[0].Action != "create" || entries[0].ActorID == nil || *entries[0].ActorID != f.actor || !strings.Contains(entries[0].AfterJSON, `"HEROIC"`) {
		t.Fatalf("create is audited: %+v", entries)
	}
}

func TestReqRaid015_AC02(t *testing.T) {
	f := newCorrectionFixture(t)
	killModes(t, f)
	first := f.recordKill("HEROIC", http.StatusCreated).Data
	body := f.request(http.MethodPost, "/kills", map[string]any{"catalog_boss_id": f.ordinaryBoss, "difficulty": "NORMAL"}, http.StatusConflict)
	if code := errorCode(t, body); code != "kill_exists" {
		t.Fatalf("second kill: %s", body)
	}
	// A removed kill no longer blocks the boss; history keeps the old row.
	f.request(http.MethodPost, fmt.Sprintf("/kills/%d/remove", first.ID), map[string]any{}, http.StatusNoContent)
	second := f.recordKill("NORMAL", http.StatusCreated).Data
	if second.ID == first.ID {
		t.Fatalf("re-recording creates a new row: %+v", second)
	}
	status, listBody := do(t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/kills", f.srv.URL, f.raidID), f.token, nil)
	var list struct{ Data []struct{ ID int64 } }
	if err := json.Unmarshal(listBody, &list); err != nil || status != http.StatusOK || len(list.Data) != 1 || list.Data[0].ID != second.ID {
		t.Fatalf("only the active kill is listed: %d %s", status, listBody)
	}
}

func TestReqRaid015_AC03(t *testing.T) {
	f := newCorrectionFixture(t)
	_, missing := killModes(t, f)
	for _, difficulty := range []string{missing, "LEGENDARY", ""} {
		body := f.request(http.MethodPost, "/kills", map[string]any{"catalog_boss_id": f.ordinaryBoss, "difficulty": difficulty}, http.StatusUnprocessableEntity)
		if code := errorCode(t, body); code != "invalid_difficulty" {
			t.Fatalf("difficulty %q: %s", difficulty, body)
		}
	}
	body := f.request(http.MethodPost, "/kills", map[string]any{"catalog_boss_id": "encounter:603:ignis", "difficulty": "NORMAL"}, http.StatusUnprocessableEntity)
	if code := errorCode(t, body); code != "invalid_boss" {
		t.Fatalf("boss of another instance: %s", body)
	}
	kill := f.recordKill("HEROIC", http.StatusCreated).Data
	body = f.request(http.MethodPatch, fmt.Sprintf("/kills/%d", kill.ID), map[string]any{"difficulty": missing}, http.StatusUnprocessableEntity)
	if code := errorCode(t, body); code != "invalid_difficulty" {
		t.Fatalf("change to a missing difficulty: %s", body)
	}
}

func TestReqRaid015_AC04(t *testing.T) {
	f := newCorrectionFixture(t)
	killModes(t, f)
	kill := f.recordKill("HEROIC", http.StatusCreated).Data
	f.attend(f.first, f.firstChar)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN"}, false), http.StatusCreated))

	body := f.request(http.MethodPatch, fmt.Sprintf("/kills/%d", kill.ID), map[string]any{"difficulty": "NORMAL"}, http.StatusConflict)
	if code := errorCode(t, body); code != "kill_has_loot" {
		t.Fatalf("change with loot: %s", body)
	}
	body = f.request(http.MethodPost, fmt.Sprintf("/kills/%d/remove", kill.ID), map[string]any{}, http.StatusConflict)
	if code := errorCode(t, body); code != "kill_has_loot" {
		t.Fatalf("remove with loot: %s", body)
	}

	// Withdrawn loot no longer blocks the kill.
	f.request(http.MethodPost, fmt.Sprintf("/loot/%d/withdraw", awardID), map[string]any{"reason": "falsch eingetragen"}, http.StatusOK)
	body = f.request(http.MethodPatch, fmt.Sprintf("/kills/%d", kill.ID), map[string]any{"difficulty": "NORMAL"}, http.StatusOK)
	var changed killResponse
	if err := json.Unmarshal(body, &changed); err != nil || changed.Data.Difficulty != "NORMAL" {
		t.Fatalf("change: %s", body)
	}
	f.request(http.MethodPost, fmt.Sprintf("/kills/%d/remove", kill.ID), map[string]any{}, http.StatusNoContent)

	entries := killAudit(t, f, kill.ID)
	actions := []string{}
	for _, entry := range entries {
		actions = append(actions, entry.Action)
	}
	if strings.Join(actions, ",") != "create,update,remove" {
		t.Fatalf("audit actions: %v", actions)
	}
	update := entries[1]
	if !strings.Contains(update.BeforeJSON, `"difficulty":"HEROIC"`) || !strings.Contains(update.AfterJSON, `"difficulty":"NORMAL"`) {
		t.Fatalf("update audit keeps old and new difficulty: %+v", update)
	}
	if !strings.Contains(entries[2].AfterJSON, `"removed_at"`) {
		t.Fatalf("remove audit records the removal: %+v", entries[2])
	}
}

func TestReqRaid015_AC05(t *testing.T) {
	f := newCorrectionFixture(t)
	killModes(t, f)
	kill := f.recordKill("HEROIC", http.StatusCreated).Data
	base := fmt.Sprintf("%s/api/v1/raids/%d/kills", f.srv.URL, f.raidID)
	writes := []struct {
		method, url string
		body        map[string]any
	}{
		{http.MethodPost, base, map[string]any{"catalog_boss_id": f.ordinaryBoss, "difficulty": "NORMAL"}},
		{http.MethodPatch, fmt.Sprintf("%s/%d", base, kill.ID), map[string]any{"difficulty": "NORMAL"}},
		{http.MethodPost, fmt.Sprintf("%s/%d/remove", base, kill.ID), map[string]any{}},
	}
	for _, write := range writes {
		if status, body := do(t, f.srv.Client(), write.method, write.url, f.memberToken, write.body); status != http.StatusForbidden {
			t.Fatalf("member %s %s: %d %s", write.method, write.url, status, body)
		}
		if status, body := do(t, f.srv.Client(), write.method, write.url, "", write.body); status != http.StatusUnauthorized {
			t.Fatalf("visitor %s %s: %d %s", write.method, write.url, status, body)
		}
	}
	if status, body := do(t, f.srv.Client(), http.MethodGet, base, "", nil); status != http.StatusUnauthorized {
		t.Fatalf("visitor read: %d %s", status, body)
	}
	if entries := killAudit(t, f, kill.ID); len(entries) != 1 {
		t.Fatalf("rejected writes change nothing: %+v", entries)
	}
}

func TestReqRaid015KillRaidStateAndReason(t *testing.T) {
	f := newCorrectionFixture(t)
	killModes(t, f)
	ctx := context.Background()
	raids := raid.NewRepo(f.db)

	// Archived completed raids accept corrections only with a reason (FR-26.5).
	f.archiveHistory()
	body := f.request(http.MethodPost, "/kills", map[string]any{"catalog_boss_id": f.ordinaryBoss, "difficulty": "HEROIC"}, http.StatusBadRequest)
	if code := errorCode(t, body); code != "reason_required" {
		t.Fatalf("archived without reason: %s", body)
	}
	body = f.request(http.MethodPost, "/kills", map[string]any{"catalog_boss_id": f.ordinaryBoss, "difficulty": "HEROIC", "reason": "nachgetragen"}, http.StatusCreated)
	var created killResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if entries := killAudit(t, f, created.Data.ID); len(entries) != 1 || entries[0].Reason != "nachgetragen" {
		t.Fatalf("archived audit keeps the reason: %+v", entries)
	}
	f.request(http.MethodPost, fmt.Sprintf("/kills/%d/remove", created.Data.ID), map[string]any{}, http.StatusBadRequest)

	// Cancelled raids reject kill writes (FR-13.11).
	rd, err := raids.Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.Status, rd.ArchivedAt, rd.CompletedAt = validation.RaidStatusCancelled, nil, nil
	if err := raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	body = f.request(http.MethodPatch, fmt.Sprintf("/kills/%d", created.Data.ID), map[string]any{"difficulty": "NORMAL", "reason": "x"}, http.StatusConflict)
	if code := errorCode(t, body); code != "raid_state" {
		t.Fatalf("cancelled raid: %s", body)
	}

	// A raid without instance and size cannot validate a difficulty.
	rd.Status, rd.CatalogInstanceID, rd.RaidSize = validation.RaidStatusOpen, nil, nil
	if err := raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	body = f.request(http.MethodPost, "/kills", map[string]any{"catalog_boss_id": f.ordinaryBoss, "difficulty": "HEROIC"}, http.StatusConflict)
	if code := errorCode(t, body); code != "catalog_context_required" {
		t.Fatalf("raid without catalog context: %s", body)
	}
}
