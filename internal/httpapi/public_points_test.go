package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"raid-website/internal/validation"
)

// FR-16.9, ADR 110: visitors see each active player's EP balance, never the transactions.
func TestPublicPointsShowBalancesOnly(t *testing.T) {
	srv, bdb := setupServer(t)
	ctx := context.Background()
	highID := seedAccount(t, bdb, "high", "pw", validation.RoleMember)
	tieBID := seedAccount(t, bdb, "tieb", "pw", validation.RoleMember)
	tieAID := seedAccount(t, bdb, "tiea", "pw", validation.RoleMember)
	goneID := seedAccount(t, bdb, "gone", "pw", validation.RoleMember)
	for _, row := range []struct {
		playerID int64
		amount   int
	}{{highID, 60}, {highID, 30}, {tieBID, 20}, {tieAID, 20}, {goneID, 500}} {
		if _, err := bdb.ExecContext(ctx, "INSERT INTO enhance_point_transactions (player_id, amount, type, reason, ref_type, ref_id, actor_id, created_at) VALUES (?, ?, 'manual', 'secret note', 'manual', '1', ?, CURRENT_TIMESTAMP)", row.playerID, row.amount, row.playerID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := bdb.ExecContext(ctx, "UPDATE players SET active = FALSE WHERE id = ?", goneID); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(srv.URL + "/api/v1/points")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("public points: status %d body %s", res.StatusCode, body)
	}
	var out struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	var order []float64
	for _, row := range out.Data {
		if len(row) != 3 || row["player_id"] == nil || row["player_name"] == nil || row["balance"] == nil {
			t.Fatalf("public row must hold exactly player_id, player_name, balance: %v", row)
		}
		order = append(order, row["player_id"].(float64))
	}
	// Seeded names are the usernames: "tiea" sorts before "tieb" at the same balance.
	want := []int64{highID, tieAID, tieBID}
	found := 0
	for _, id := range order {
		if id == float64(goneID) {
			t.Fatalf("inactive player listed: %s", body)
		}
		if found < len(want) && id == float64(want[found]) {
			found++
		}
	}
	if found != len(want) || out.Data[0]["balance"].(float64) != 90 {
		t.Fatalf("order or balance wrong: %s", body)
	}

	res, err = http.Get(fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, highID))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("transactions without session: want 401 got %d", res.StatusCode)
	}
}

// ADR 110: everyone sees all raid leaders.
func TestRaidLeadersArePublic(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	leaderID := seedAccount(t, bdb, "leader", "pw", validation.RoleRaidleader)
	token := login(t, client, srv, "leader", "pw")
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", token, map[string]any{"name": "ICC", "raid_leader_id": leaderID})
	if status != http.StatusCreated {
		t.Fatalf("create raid: %d %s", status, body)
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(fmt.Sprintf("%s/api/v1/raids/%d/leaders", srv.URL, created.Data.ID))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("leaders without session: want 200 got %d", res.StatusCode)
	}
}
