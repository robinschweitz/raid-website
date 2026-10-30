package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestArchiveAPIRosterIncompleteAndHistoricalCompatibility(t *testing.T) {
	srv, database := setupServer(t)
	actor := seedAccount(t, database, "roster-finalizer", "password", validation.RoleRaidassistent)
	token := login(t, srv.Client(), srv, "roster-finalizer", "password")
	ctx := context.Background()
	now := time.Now().UTC()
	char := &character.Character{PlayerID: actor, Name: "LiveCharacter", Class: "Krieger", MainSpec: "Waffen", SecondSpec: "Furor", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(database).Create(ctx, char); err != nil {
		t.Fatal(err)
	}
	series := &raidseries.Series{Name: "Series", RaidLeaderID: actor, Weekdays: []int{1, 3}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
	if err := raidseries.NewService(raidseries.NewRepo(database)).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	start := now.AddDate(0, 0, -7)
	rd := &raid.Raid{Name: "Incomplete actual roster", RaidLeaderID: actor, SeriesID: &series.ID, ScheduledStart: &start, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	raids := raid.NewRepo(database)
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	row := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: char.ID, CharacterName: "PlayedCharacter", Class: "Priester", Spec: "", Role: validation.RoleHeal, CreatedAt: now, UpdatedAt: now}
	att := attendance.NewRepo(database)
	if err := att.Create(ctx, row); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/raids/%d/archive", srv.URL, rd.ID)
	status, body := do(t, srv.Client(), http.MethodPost, url, token, nil)
	assertArchiveCode(t, status, body, "roster_incomplete")
	if !strings.Contains(string(body), "Die Teilnahme ist unvollständig.") {
		t.Fatalf("missing fixed incomplete attendance message: %s", body)
	}
	stored, err := raids.Get(ctx, rd.ID)
	if err != nil || stored.CompletedAt != nil || stored.ArchivedAt != nil {
		t.Fatalf("failed finalization mutated raid: %+v %v", stored, err)
	}
	storedRow, err := att.Get(ctx, row.ID)
	if err != nil || storedRow.Spec != "" || storedRow.Class != "Priester" || storedRow.CharacterName != "PlayedCharacter" {
		t.Fatalf("snapshot inferred from live character: %+v %v", storedRow, err)
	}
	balance, err := enhancepoints.NewRepo(database).Balance(ctx, actor)
	if err != nil || balance != 0 {
		t.Fatalf("failed archive awarded EP: %d %v", balance, err)
	}
	row.Spec = "Actual raid spec"
	if err := att.Update(ctx, row); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, srv.Client(), http.MethodPost, url, token, nil)
	if status != http.StatusOK {
		t.Fatalf("complete roster archive: %d %s", status, body)
	}
	// Imported completed history may lack every snapshot and must not be reconstructed.
	historic := &raid.Raid{Name: "Imported completed", RaidLeaderID: actor, Status: validation.RaidStatusCompleted, CompletedAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, historic); err != nil {
		t.Fatal(err)
	}
	unknown := &attendance.Attendance{RaidID: historic.ID, PlayerID: actor, CharacterID: char.ID, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
	if err := att.Create(ctx, unknown); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/archive", srv.URL, historic.ID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("historical compatibility: %d %s", status, body)
	}
	storedUnknown, err := att.Get(ctx, unknown.ID)
	if err != nil || storedUnknown.CharacterName != "" || storedUnknown.Class != "" || storedUnknown.Spec != "" {
		t.Fatalf("historical unknown snapshots guessed: %+v %v", storedUnknown, err)
	}
	balance, err = enhancepoints.NewRepo(database).Balance(ctx, actor)
	if err != nil || balance != 10 {
		t.Fatalf("historical EP regenerated: %d %v", balance, err)
	}
}
