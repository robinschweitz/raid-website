package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestCreateRaidDerivesNameFromSeriesOrRequiresExplicitName(t *testing.T) {
	srv, database := setupServer(t)
	actor := seedAccount(t, database, "naming-assistant", "password", validation.RoleRaidassistent)
	token := login(t, srv.Client(), srv, "naming-assistant", "password")
	ctx := context.Background()

	numbered := &raidseries.Series{Name: "ICC Montagsraid", NameTemplate: "ICC25-{number}", NextRaidNumber: 35, RaidLeaderID: actor, Weekdays: []int{1}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
	plain := &raidseries.Series{Name: "Freitagsraid", NextRaidNumber: 1, RaidLeaderID: actor, Weekdays: []int{5}, IntervalWeeks: 1, StartTime: "21:00", Timezone: "Europe/Berlin"}
	for _, s := range []*raidseries.Series{numbered, plain} {
		if err := raidseries.NewRepo(database).Save(ctx, s); err != nil {
			t.Fatalf("create series: %v", err)
		}
	}

	type result struct {
		status int
		name   string
		code   string
	}
	create := func(fields map[string]any) result {
		t.Helper()
		status, body := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/raids", token, fields)
		var response struct {
			Data  struct{ Name string } `json:"data"`
			Error struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatalf("decode response: %v %s", err, body)
		}
		return result{status: status, name: response.Data.Name, code: response.Error.Code}
	}
	start := time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)
	derived := func(offset int) map[string]any {
		return map[string]any{"raid_leader_id": actor, "series_id": numbered.ID, "scheduled_start": start.AddDate(0, 0, 7*offset).Format(time.RFC3339)}
	}

	first := create(derived(0))
	if first.status != http.StatusCreated || first.name != "ICC25-35" {
		t.Fatalf("first derived raid: %+v", first)
	}
	second := create(derived(1))
	if second.status != http.StatusCreated || second.name != "ICC25-36" {
		t.Fatalf("second derived raid: %+v", second)
	}
	explicit := create(map[string]any{"name": "Weihnachtsraid", "raid_leader_id": actor, "series_id": numbered.ID, "scheduled_start": start.AddDate(0, 0, 14).Format(time.RFC3339)})
	if explicit.status != http.StatusCreated || explicit.name != "ICC25-37" {
		t.Fatalf("explicit name raid must be derived from template: %+v", explicit)
	}
	stored, err := raidseries.NewRepo(database).Get(ctx, numbered.ID)
	if err != nil {
		t.Fatalf("reload series: %v", err)
	}
	if stored.NextRaidNumber != 38 {
		t.Fatalf("explicit name must not override template; counter consumed: next %d", stored.NextRaidNumber)
	}

	fallback := create(map[string]any{"raid_leader_id": actor, "series_id": plain.ID, "scheduled_start": start.Format(time.RFC3339)})
	if fallback.status != http.StatusCreated || fallback.name != "Freitagsraid" {
		t.Fatalf("template fallback raid: %+v", fallback)
	}

	withoutSeries := create(map[string]any{"name": "Einzelner Raid", "raid_leader_id": actor, "scheduled_start": start.Format(time.RFC3339)})
	if withoutSeries.status != http.StatusCreated || withoutSeries.name != "Einzelner Raid" {
		t.Fatalf("raid without series: %+v", withoutSeries)
	}
	missingName := create(map[string]any{"raid_leader_id": actor, "scheduled_start": start.Format(time.RFC3339)})
	if missingName.status != http.StatusBadRequest || missingName.code != "name_required" {
		t.Fatalf("missing name without series: %+v", missingName)
	}
}
