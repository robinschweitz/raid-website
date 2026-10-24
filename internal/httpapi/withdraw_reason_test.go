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

// REQ-RAID-011: withdrawing a manual loot award needs a reason only for archived raids (FR-14.9, FR-26.5).
func TestReqRaid011_AC01(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	f.attend(f.first, f.firstChar)
	for _, reason := range []any{nil, "", "  \t "} {
		award := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "SECOND"}, false), http.StatusCreated))
		body := map[string]any{}
		if reason != nil {
			body["reason"] = reason
		}
		f.request(http.MethodPost, fmt.Sprintf("/loot/%d/withdraw", award), body, http.StatusOK)
		stored, err := loot.NewRepo(f.db).Get(ctx, award)
		if err != nil {
			t.Fatal(err)
		}
		if stored.WithdrawnAt == nil || stored.WithdrawnReason != nil {
			t.Fatalf("award not withdrawn without reason: %+v", stored)
		}
		logs, err := audit.NewRepo(f.db).ListByEntity(ctx, "loot_award", fmt.Sprint(award))
		if err != nil {
			t.Fatal(err)
		}
		if last := logs[len(logs)-1]; last.Reason != "" || last.BeforeJSON == last.AfterJSON {
			t.Fatalf("audit: %+v", last)
		}
	}
}

func TestReqRaid011_AC02(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	f.attend(f.first, f.firstChar)
	award := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "SECOND"}, false), http.StatusCreated))
	f.archiveHistory()
	for _, reason := range []string{"", " \t "} {
		status, body := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/loot/%d/withdraw", f.srv.URL, f.raidID, award), f.token, map[string]any{"reason": reason})
		if status != http.StatusBadRequest {
			t.Fatalf("archived withdrawal without reason: %d %s", status, body)
		}
		var out struct{ Error struct{ Code string } }
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if out.Error.Code != "reason_required" {
			t.Fatalf("wrong error: %s", body)
		}
		stored, err := loot.NewRepo(f.db).Get(ctx, award)
		if err != nil || stored.WithdrawnAt != nil {
			t.Fatalf("rejected withdrawal changed the award: %+v %v", stored, err)
		}
	}
	f.request(http.MethodPost, fmt.Sprintf("/loot/%d/withdraw", award), map[string]any{"reason": "  wrong recipient  "}, http.StatusOK)
	stored, err := loot.NewRepo(f.db).Get(ctx, award)
	if err != nil || stored.WithdrawnAt == nil || stored.WithdrawnReason == nil || *stored.WithdrawnReason != "wrong recipient" {
		t.Fatalf("archived withdrawal with reason: %+v %v", stored, err)
	}
}
