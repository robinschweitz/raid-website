package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/auth"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/httpapi"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func setupServer(t *testing.T) (*httptest.Server, *bun.DB) {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "api.db") + "?_pragma=foreign_keys(1)"
	bdb, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = bdb.Close() })
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := httptest.NewServer(httpapi.New(bdb.DB, httpapi.Options{}))
	t.Cleanup(srv.Close)
	return srv, bdb.DB
}

func seedAccount(t *testing.T, bdb *bun.DB, username, password string, role validation.Role) int64 {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	now := time.Now().UTC()
	uname := strings.ToLower(username)
	p := &player.Player{
		Name:         username,
		Username:     &uname,
		PasswordHash: hash,
		Role:         role,
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := player.NewRepo(bdb).Create(context.Background(), p); err != nil {
		t.Fatalf("seed account %s: %v", username, err)
	}
	return p.ID
}

func login(t *testing.T, client *http.Client, srv *httptest.Server, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"username": username, "password": password})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: status %d", username, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName {
			return c.Value
		}
	}
	t.Fatalf("login %s: no session cookie", username)
	return ""
}

func do(t *testing.T, client *http.Client, method, url, token string, body any) (int, []byte) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Cookie", auth.CookieName+"="+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, buf.Bytes()
}

func createPlayer(t *testing.T, client *http.Client, srv *httptest.Server, token, name string) int64 {
	t.Helper()
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/players", token, map[string]any{"name": name})
	if status != http.StatusCreated {
		t.Fatalf("create player %s: status %d body %s", name, status, body)
	}
	var out struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode player: %v", err)
	}
	return out.Data.ID
}

func TestRaidLifecycleAndPoints(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()

	seedAccount(t, bdb, "admin", "admin-pw", validation.RoleAdmin)
	token := login(t, client, srv, "admin", "admin-pw")

	leader := createPlayer(t, client, srv, token, "Leader")
	plain := createPlayer(t, client, srv, token, "Plain")
	rewarded := createPlayer(t, client, srv, token, "Rewarded")

	now := time.Now().UTC()
	chars := character.NewRepo(bdb)
	plainChar := &character.Character{PlayerID: plain, Name: "Plainchar", Class: "Krieger", MainSpec: "Waffen", SecondSpec: "Furor", Active: true, CreatedAt: now, UpdatedAt: now}
	rewardedChar := &character.Character{PlayerID: rewarded, Name: "Rewardedchar", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	for _, c := range []*character.Character{plainChar, rewardedChar} {
		if err := chars.Create(ctx, c); err != nil {
			t.Fatalf("create character: %v", err)
		}
	}

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", token, map[string]any{
		"name":           "ICC",
		"raid_leader_id": leader,
	})
	if status != http.StatusCreated {
		t.Fatalf("create raid: status %d body %s", status, body)
	}
	var raidOut struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raidOut); err != nil {
		t.Fatalf("decode raid: %v", err)
	}
	raidID := raidOut.Data.ID

	for _, entry := range []struct{ player, char int64 }{{plain, plainChar.ID}, {rewarded, rewardedChar.ID}} {
		status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/attendance", srv.URL, raidID), token, map[string]any{
			"player_id":    entry.player,
			"character_id": entry.char,
			"role":         "DD",
		})
		if status != http.StatusCreated {
			t.Fatalf("create attendance: status %d body %s", status, body)
		}
	}

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/loot", srv.URL, raidID), token, map[string]any{
		"boss":                 "Saurfang",
		"item_id":              "50001",
		"item_name":            "Testitem",
		"awarded_to_player_id": rewarded,
		"is_main_spec":         true,
	})
	if status != http.StatusCreated {
		t.Fatalf("create loot: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/complete", srv.URL, raidID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("complete raid: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, plain), "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("get points unauthenticated: want 401 got %d", status)
	}
	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, plain), token, nil)
	if status != http.StatusOK {
		t.Fatalf("get points: status %d body %s", status, body)
	}
	var pointsOut struct {
		Data struct {
			Balance int `json:"balance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &pointsOut); err != nil {
		t.Fatalf("decode points: %v", err)
	}
	if pointsOut.Data.Balance != 10 {
		t.Fatalf("plain balance: want 10 got %d", pointsOut.Data.Balance)
	}

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, rewarded), token, nil)
	if err := json.Unmarshal(body, &pointsOut); err != nil {
		t.Fatalf("decode rewarded points: %v", err)
	}
	if pointsOut.Data.Balance != 0 {
		t.Fatalf("rewarded balance: want 0 got %d", pointsOut.Data.Balance)
	}

	// Token award derives a token transaction.
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/loot", srv.URL, raidID), token, map[string]any{
		"item_name":            "Weiheabzeichen des Eroberers",
		"awarded_to_player_id": plain,
	})
	if status != http.StatusCreated {
		t.Fatalf("create token loot: status %d body %s", status, body)
	}
	var lootOut struct {
		TokenAward bool `json:"token_award"`
	}
	if err := json.Unmarshal(body, &lootOut); err != nil {
		t.Fatalf("decode loot: %v", err)
	}
	if !lootOut.TokenAward {
		t.Fatalf("expected token_award true, body %s", body)
	}

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/tokens", srv.URL, plain), token, nil)
	if status != http.StatusOK {
		t.Fatalf("get tokens: status %d body %s", status, body)
	}
	var tokensOut struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tokensOut); err != nil {
		t.Fatalf("decode tokens: %v", err)
	}
	if tokensOut.Data.Total != 1 {
		t.Fatalf("token total: want 1 got %d", tokensOut.Data.Total)
	}

	// Roll consumes min(balance,50) = 10 and returns a roll in [1,110].
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/points/roll", srv.URL, plain), token, nil)
	if status != http.StatusOK {
		t.Fatalf("roll: status %d body %s", status, body)
	}
	var rollOut struct {
		Data struct {
			Cost         int `json:"cost"`
			Roll         int `json:"roll"`
			Min          int `json:"min"`
			Max          int `json:"max"`
			BalanceAfter int `json:"balance_after"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &rollOut); err != nil {
		t.Fatalf("decode roll: %v", err)
	}
	if rollOut.Data.Cost != 10 {
		t.Fatalf("roll cost: want 10 got %d", rollOut.Data.Cost)
	}
	if rollOut.Data.Roll < rollOut.Data.Min || rollOut.Data.Roll > rollOut.Data.Max {
		t.Fatalf("roll %d out of range [%d,%d]", rollOut.Data.Roll, rollOut.Data.Min, rollOut.Data.Max)
	}
	if rollOut.Data.BalanceAfter != 0 {
		t.Fatalf("balance after roll: want 0 got %d", rollOut.Data.BalanceAfter)
	}

	// Lock requires balance >= 150; now 0.
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/locks", srv.URL, plain), token, map[string]any{"raid_id": raidID})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("lock: want 422 got %d body %s", status, body)
	}
}

func TestMissingSessionRejected(t *testing.T) {
	srv, _ := setupServer(t)
	status, _ := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/players", "", map[string]any{"name": "X"})
	if status != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", status)
	}
}

func TestRoleEnforcement(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()

	adminID := seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	memberID := seedAccount(t, bdb, "member", "pw", validation.RoleMember)
	seedAccount(t, bdb, "guest", "pw", validation.RoleGuest)
	_ = adminID
	_ = memberID

	adminToken := login(t, client, srv, "admin", "pw")
	memberToken := login(t, client, srv, "member", "pw")
	guestToken := login(t, client, srv, "guest", "pw")

	// Config endpoints require Admin.
	status, _ := do(t, client, http.MethodPost, srv.URL+"/api/v1/guild-ranks", memberToken, map[string]any{"name": "X"})
	if status != http.StatusForbidden {
		t.Fatalf("member create guild rank: want 403 got %d", status)
	}
	// Raid operations require Raidassistent or higher; member is refused.
	status, _ = do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", memberToken, map[string]any{"name": "ICC", "raid_leader_id": 1})
	if status != http.StatusForbidden {
		t.Fatalf("member create raid: want 403 got %d", status)
	}
	// Guest cannot vote (Member+ required).
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", adminToken, map[string]any{"title": "t"})
	if status != http.StatusCreated {
		t.Fatalf("admin create poll: status %d body %s", status, body)
	}
	var pollOut struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &pollOut); err != nil {
		t.Fatalf("decode poll: %v", err)
	}
	status, _ = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, pollOut.Data.ID), guestToken, map[string]any{"choice": "yes"})
	if status != http.StatusForbidden {
		t.Fatalf("guest vote: want 403 got %d", status)
	}
	status, _ = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, pollOut.Data.ID), memberToken, map[string]any{"choice": "yes"})
	if status != http.StatusOK {
		t.Fatalf("member vote: want 200 got %d", status)
	}
	// Admin can manage config.
	status, _ = do(t, client, http.MethodPost, srv.URL+"/api/v1/guild-ranks", adminToken, map[string]any{"name": "Raidleader"})
	if status != http.StatusCreated {
		t.Fatalf("admin create guild rank: want 201 got %d", status)
	}
}
