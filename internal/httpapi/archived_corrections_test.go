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
	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestArchivedCorrectionsStaffPermissionsReasonsAndFinalization(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	attendanceID := f.attend(f.first, f.firstChar)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar}, false), http.StatusCreated))
	series := &raidseries.Series{Name: "Correction series", RaidLeaderID: f.actor, Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "UTC"}
	if err := raidseries.NewService(raidseries.NewRepo(f.db)).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPatch, "", map[string]any{"series_id": series.ID, "scheduled_start": time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339)}, http.StatusOK)
	f.request(http.MethodPost, "/archive", nil, http.StatusOK)
	before, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	var raidList struct{ Data []raid.Raid }
	status, body := do(t, f.srv.Client(), http.MethodGet, f.srv.URL+"/api/v1/raids", f.token, nil)
	if status != http.StatusOK {
		t.Fatal(status)
	}
	if err := json.Unmarshal(body, &raidList); err != nil {
		t.Fatal(err)
	}
	if len(raidList.Data) != 2 {
		t.Fatalf("successor missing: %s", body)
	}
	attendanceSuffix, lootSuffix := fmt.Sprintf("/attendance/%d", attendanceID), fmt.Sprintf("/loot/%d", awardID)
	for _, role := range []validation.Role{validation.RoleGuest, validation.RoleMember, validation.RoleRaidassistent, validation.RoleRaidleader, validation.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			username := "archived-" + string(role)
			actor := seedAccount(t, f.db, username, "password", role)
			token := login(t, f.srv.Client(), f.srv, username, "password")
			for _, suffix := range []string{attendanceSuffix, lootSuffix} {
				url := fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix)
				for _, reason := range []string{"", " \t "} {
					want := http.StatusForbidden
					if role.AtLeast(validation.RoleRaidassistent) {
						want = http.StatusBadRequest
					}
					status, body := do(t, f.srv.Client(), http.MethodPatch, url, token, map[string]any{"notes": "must not persist", "reason": reason})
					if status != want {
						t.Fatalf("reason: %d %s", status, body)
					}
					if want == http.StatusBadRequest {
						var out struct{ Error struct{ Code string } }
						if err := json.Unmarshal(body, &out); err != nil {
							t.Fatal(err)
						}
						if out.Error.Code != "reason_required" {
							t.Fatalf("wrong error: %s", body)
						}
					}
				}
				want := http.StatusForbidden
				if role.AtLeast(validation.RoleRaidassistent) {
					want = http.StatusOK
				}
				status, body := do(t, f.srv.Client(), http.MethodPatch, url, token, map[string]any{"notes": username, "reason": "  archived clarification  "})
				if status != want {
					t.Fatalf("correction: %d %s", status, body)
				}
				if want == http.StatusOK {
					entity, id := "attendance", attendanceID
					if suffix == lootSuffix {
						entity, id = "loot_award", awardID
					}
					logs, err := audit.NewRepo(f.db).ListByEntity(ctx, entity, fmt.Sprint(id))
					if err != nil {
						t.Fatal(err)
					}
					last := logs[len(logs)-1]
					if last.ActorID == nil || *last.ActorID != actor || last.Reason != "archived clarification" || last.BeforeJSON == last.AfterJSON {
						t.Fatalf("audit: %+v", last)
					}
				}
			}
			for _, suffix := range []string{"/attendance", "/loot"} {
				var payload map[string]any
				if suffix == "/attendance" {
					payload = map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "DD"}
				} else {
					payload = f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "SECOND"}, false)
				}
				want := http.StatusForbidden
				if role.AtLeast(validation.RoleRaidassistent) {
					want = http.StatusBadRequest
				}
				status, body := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix), token, payload)
				if status != want {
					t.Fatalf("addition reason: %d %s", status, body)
				}
			}
		})
	}
	f.points(f.first, 10)
	after, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("finalization changed: %+v %+v %v", before, after, err)
	}
	f.request(http.MethodPost, "/archive", nil, http.StatusOK)
	f.points(f.first, 10)
	status, body = do(t, f.srv.Client(), http.MethodGet, f.srv.URL+"/api/v1/raids", f.token, nil)
	var afterList struct{ Data []raid.Raid }
	if err := json.Unmarshal(body, &afterList); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || !reflect.DeepEqual(raidList, afterList) {
		t.Fatalf("successor or history changed: %s", body)
	}
}

func TestUnarchivedCorrectionsDoNotRequireReasonButRemainAudited(t *testing.T) {
	for _, state := range []validation.RaidStatus{validation.RaidStatusPlanned, validation.RaidStatusOpen, validation.RaidStatusCompleted} {
		t.Run(string(state), func(t *testing.T) {
			f := newCorrectionFixture(t)
			ctx := context.Background()
			id := f.attend(f.first, f.firstChar)
			award := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "SECOND"}, false), http.StatusCreated))
			rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
			if err != nil {
				t.Fatal(err)
			}
			rd.Status = state
			if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
				t.Fatal(err)
			}
			f.request(http.MethodPatch, fmt.Sprintf("/attendance/%d", id), map[string]any{"notes": "inline edit"}, http.StatusOK)
			f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", award), map[string]any{"notes": "inline edit"}, http.StatusOK)
			a, err := attendance.NewRepo(f.db).Get(ctx, id)
			if err != nil || a.Notes != "inline edit" {
				t.Fatalf("attendance: %+v %v", a, err)
			}
			l, err := loot.NewRepo(f.db).Get(ctx, award)
			if err != nil || l.Notes != "inline edit" {
				t.Fatalf("loot: %+v %v", l, err)
			}
			for entity, id := range map[string]int64{"attendance": id, "loot_award": award} {
				logs, err := audit.NewRepo(f.db).ListByEntity(ctx, entity, fmt.Sprint(id))
				if err != nil || len(logs) == 0 {
					t.Fatalf("missing audit: %v", err)
				}
				last := logs[len(logs)-1]
				if last.ActorID == nil || *last.ActorID != f.actor || last.Reason != "" || last.BeforeJSON == last.AfterJSON {
					t.Fatalf("audit: %+v", last)
				}
			}
			f.points(f.first, 0)
		})
	}
}
