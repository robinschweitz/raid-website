package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"raid-website/internal/audit"
	"raid-website/internal/loot"
)

type shortfallAnswer struct {
	Error struct{ Code string }
	Data  struct {
		Shortfalls []struct {
			PlayerID   int64  `json:"player_id"`
			PlayerName string `json:"player_name"`
			NotTaken   int    `json:"points_not_taken"`
			Taken      int    `json:"points_taken"`
		}
	}
}

// completedFixtureWithBalance5 builds a completed raid in which the first player stands in the roster and holds 5 EP.
func completedFixtureWithBalance5(t *testing.T) *correctionFixture {
	t.Helper()
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.legacyCompleted(f.first)
	f.seedPoints(f.first, -5)
	f.points(f.first, 5)
	return f
}

// REQ-RAID-012 AC-01: the correction is refused with a warning that names the player and the points not taken.
func TestReqRaid012_AC01(t *testing.T) {
	f := completedFixtureWithBalance5(t)
	body := f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN"}, false)
	var out shortfallAnswer
	if err := json.Unmarshal(f.request(http.MethodPost, "/loot", body, http.StatusConflict), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error.Code != "ep_shortfall" || len(out.Data.Shortfalls) != 1 {
		t.Fatalf("answer: %+v", out)
	}
	if s := out.Data.Shortfalls[0]; s.PlayerID != f.first || s.PlayerName != "correction-first" || s.NotTaken != 5 || s.Taken != 5 {
		t.Fatalf("shortfall: %+v", s)
	}
	f.points(f.first, 5)
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatalf("the refused correction stored an award: %v %v", awards, err)
	}
}

// REQ-RAID-012 AC-02: the confirmed correction leaves 0 and the audit entry records the points not taken.
func TestReqRaid012_AC02(t *testing.T) {
	f := completedFixtureWithBalance5(t)
	body := f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN", "confirm_ep_shortfall": true}, false)
	f.request(http.MethodPost, "/loot", body, http.StatusCreated)
	f.points(f.first, 0)
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "ep_correction", fmt.Sprint(f.first))
	if err != nil || len(logs) != 1 {
		t.Fatalf("audit: %v %v", logs, err)
	}
	var after struct {
		NotTaken int `json:"points_not_taken"`
	}
	if err := json.Unmarshal([]byte(logs[0].AfterJSON), &after); err != nil || after.NotTaken != 5 {
		t.Fatalf("audit after: %s %v", logs[0].AfterJSON, err)
	}
}

// REQ-RAID-012 AC-03: the points that were not taken are not taken later.
func TestReqRaid012_AC03(t *testing.T) {
	f := completedFixtureWithBalance5(t)
	body := f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN", "confirm_ep_shortfall": true}, false)
	award := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	f.points(f.first, 0)
	f.request(http.MethodPost, fmt.Sprintf("/loot/%d/withdraw", award), map[string]any{}, http.StatusOK)
	f.points(f.first, 10)
	f.request(http.MethodPost, "/loot", body, http.StatusCreated)
	f.points(f.first, 0)
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "ep_correction", fmt.Sprint(f.first))
	if err != nil || len(logs) != 1 {
		t.Fatalf("the second deduction was whole, so only the first is audited: %v %v", logs, err)
	}
}

// A balance of 0 takes nothing and writes no ledger row; withdrawal and roster paths follow the same floor.
func TestReqRaid012_ZeroBalanceTakesNothing(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.legacyCompleted()
	body := f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "SECOND"}, false)
	award := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", award), map[string]any{"award_method": "MAIN"}, http.StatusConflict)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", award), map[string]any{"award_method": "MAIN", "confirm_ep_shortfall": true}, http.StatusOK)
	f.points(f.first, 0)
}

// REQ-RAID-012 permission boundary: a member cannot make the correction, with or without the confirmation.
func TestReqRaid012_MemberForbidden(t *testing.T) {
	f := completedFixtureWithBalance5(t)
	body := f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN", "confirm_ep_shortfall": true}, false)
	status, out := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/loot", f.srv.URL, f.raidID), f.memberToken, body)
	if status != http.StatusForbidden {
		t.Fatalf("member: %d %s", status, out)
	}
	f.points(f.first, 5)
}
