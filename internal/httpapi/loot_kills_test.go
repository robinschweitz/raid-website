package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"raid-website/internal/bosskill"
	"raid-website/internal/loot"
)

func TestReqRaid016_AC01(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	body := f.selection(map[string]any{"difficulty": "HEROIC", "award_method": "MAIN", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar}, false)
	id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	if got := f.award(id); got.Difficulty != bosskill.Heroic {
		t.Fatalf("loot difficulty=%s", got.Difficulty)
	}
	kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(kills) != 1 || kills[0].Difficulty != bosskill.Heroic {
		t.Fatalf("kills=%+v %v", kills, err)
	}
}
func TestReqRaid016_AC02(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	body := f.lootBody("MAIN")
	delete(body, "difficulty")
	result := f.request(http.MethodPost, "/loot", body, http.StatusUnprocessableEntity)
	if errorCode(t, result) != "difficulty_required" {
		t.Fatalf("missing difficulty=%s", result)
	}
	kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(kills) != 0 {
		t.Fatal("missing difficulty wrote a kill")
	}
}
func TestReqRaid016_AC03(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.recordKill("NORMAL", http.StatusCreated)
	body := f.selection(map[string]any{"difficulty": "HEROIC", "award_method": "MAIN", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar}, false)
	result := f.request(http.MethodPost, "/loot", body, http.StatusConflict)
	if errorCode(t, result) != "kill_difficulty_conflict" {
		t.Fatalf("stale difficulty=%s", result)
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(awards) != 0 {
		t.Fatal("conflict wrote loot")
	}
	delete(body, "difficulty")
	id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	if f.award(id).Difficulty != bosskill.Normal {
		t.Fatal("existing kill difficulty not inherited")
	}
}
func TestReqRaid016_AC04(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	if _, err := f.db.ExecContext(context.Background(), "CREATE TRIGGER reject_loot BEFORE INSERT ON loot_awards BEGIN SELECT RAISE(ABORT,'failure'); END"); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, "/loot", f.lootBody("MAIN"), http.StatusInternalServerError)
	kills, err := bosskill.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil || len(kills) != 0 {
		t.Fatal("failed loot left a kill")
	}
}
