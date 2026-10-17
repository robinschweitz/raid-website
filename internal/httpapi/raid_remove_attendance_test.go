package httpapi_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestRemoveAttendanceFromOpenRaid(t *testing.T) {
	f := newCorrectionFixture(t)
	keep := f.attend(f.first, f.firstChar)
	leave := f.attend(f.second, f.secondChar)
	suffix := fmt.Sprintf("/attendance/%d", leave)

	status, _ := do(t, f.srv.Client(), http.MethodDelete, fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix), f.memberToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member removal status %d", status)
	}

	f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar}, false), http.StatusCreated)
	f.request(http.MethodDelete, fmt.Sprintf("/attendance/%d", keep), nil, http.StatusConflict)

	f.request(http.MethodDelete, suffix, nil, http.StatusNoContent)
	f.request(http.MethodDelete, suffix, nil, http.StatusNotFound)
	// Removed characters can be added again.
	f.attend(f.second, f.secondChar)
}
