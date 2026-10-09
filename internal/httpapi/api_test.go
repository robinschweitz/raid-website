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
	"raid-website/internal/enhancepoints"
	"raid-website/internal/httpapi"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

func setupServer(t *testing.T) (*httptest.Server, *bun.DB) {
	return setupServerWithOptions(t, httpapi.Options{})
}

func setupServerWithOptions(t *testing.T, opts httpapi.Options) (*httptest.Server, *bun.DB) {
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
	handler, err := httpapi.New(bdb.DB, opts)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, bdb.DB
}

type fakeRegistrationFeed struct {
	characters map[string]registration.ExternalCharacter
}

func (f fakeRegistrationFeed) SearchCharacters(ctx context.Context, query string) ([]registration.ExternalCharacter, error) {
	var rows []registration.ExternalCharacter
	for _, row := range f.characters {
		if strings.Contains(strings.ToLower(row.Name), strings.ToLower(query)) {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (f fakeRegistrationFeed) LoadCharacter(ctx context.Context, profileID string) (registration.ExternalCharacter, string, error) {
	row, ok := f.characters[profileID]
	if !ok {
		return registration.ExternalCharacter{}, "", registration.ErrExternalNotFound
	}
	return row, `{"fixture":true}`, nil
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
	plain := seedAccount(t, bdb, "Plain", "plain-pw", validation.RoleMember)
	plainToken := login(t, client, srv, "Plain", "plain-pw")
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

	tokenBoss, tokenItem, ordinaryBoss, ordinaryItem := bundledICCDrops(t, bdb)
	instance, size := "631", 25
	series := &raidseries.Series{Name: "ICC", CatalogInstanceID: &instance, RaidSize: &size, RaidLeaderID: leader, Weekdays: []int{int(now.Weekday())}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "UTC"}
	if err := raidseries.NewService(raidseries.NewRepo(bdb)).Save(ctx, series); err != nil {
		t.Fatalf("create series: %v", err)
	}
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", token, map[string]any{
		"name":            "ICC",
		"raid_leader_id":  leader,
		"series_id":       series.ID,
		"scheduled_start": now.AddDate(0, 0, -7).Format(time.RFC3339),
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
		"catalog_boss_id":         ordinaryBoss,
		"item_id":                 ordinaryItem,
		"awarded_to_player_id":    rewarded,
		"award_method":            "MAIN",
		"awarded_to_character_id": rewardedChar.ID,
	})
	if status != http.StatusCreated {
		t.Fatalf("create loot: status %d body %s", status, body)
	}

	// Process all loot before final archiving. An off-spec token does not change EP eligibility.
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/loot", srv.URL, raidID), token, map[string]any{
		"catalog_boss_id":         tokenBoss,
		"item_id":                 tokenItem,
		"awarded_to_player_id":    plain,
		"awarded_to_character_id": plainChar.ID,
		"award_method":            "SECOND",
	})
	if status != http.StatusCreated || !bytes.Contains(body, []byte(`"token_award":true`)) {
		t.Fatalf("create token loot: status %d body %s", status, body)
	}
	balanceBefore, err := enhancepoints.NewRepo(bdb).Balance(ctx, plain)
	if err != nil || balanceBefore != 0 {
		t.Fatalf("EP must wait for archive: balance %d error %v", balanceBefore, err)
	}
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/archive", srv.URL, raidID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("archive raid: status %d body %s", status, body)
	}
	var archived struct {
		Data raid.Raid  `json:"data"`
		Next *raid.Raid `json:"next_raid"`
	}
	if err := json.Unmarshal(body, &archived); err != nil {
		t.Fatal(err)
	}
	if archived.Data.ArchivedAt == nil || archived.Next == nil || archived.Next.Status != validation.RaidStatusPlanned || archived.Next.ScheduledStart == nil || !archived.Next.ScheduledStart.After(now) {
		t.Fatalf("archive did not finalize and schedule future successor: %s", body)
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

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/loot", srv.URL, plain), token, nil)
	if status != http.StatusOK {
		t.Fatalf("get player loot: status %d body %s", status, body)
	}
	var lootOut struct {
		Data []struct {
			RaidID     int64  `json:"raid_id"`
			ItemID     string `json:"item_id"`
			TokenAward bool   `json:"token_award"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &lootOut); err != nil {
		t.Fatalf("decode player loot: %v", err)
	}
	if len(lootOut.Data) != 1 || lootOut.Data[0].RaidID != raidID || lootOut.Data[0].ItemID != tokenItem || !lootOut.Data[0].TokenAward {
		t.Fatalf("player loot token row mismatch: %+v", lootOut.Data)
	}

	// The removed roll endpoint must not debit the existing balance.
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/points/roll", srv.URL, plain), plainToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("removed roll endpoint: want 404 got %d %s", status, body)
	}
	balanceAfter, err := enhancepoints.NewRepo(bdb).Balance(ctx, plain)
	if err != nil || balanceAfter != 10 {
		t.Fatalf("removed roll changed balance: %d %v", balanceAfter, err)
	}

	// Lock still requires balance >= 150.
	upcoming := &raid.Raid{Name: "Upcoming ICC", RaidLeaderID: leader, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(bdb).Create(ctx, upcoming); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/locks", srv.URL, plain), plainToken, map[string]any{"raid_id": upcoming.ID, "item_name": "Test item"})
	if status != http.StatusUnprocessableEntity || !bytes.Contains(body, []byte(`"code":"lock_not_allowed"`)) {
		t.Fatalf("lock: want 422 got %d body %s", status, body)
	}
}

func TestPointsAndLocksSelfServiceOwnership(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	ownerID := seedAccount(t, bdb, "owner", "pw", validation.RoleMember)
	otherID := seedAccount(t, bdb, "other", "pw", validation.RoleMember)
	adminID := seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	guestID := seedAccount(t, bdb, "guest", "pw", validation.RoleGuest)
	ownerToken := login(t, client, srv, "owner", "pw")
	otherToken := login(t, client, srv, "other", "pw")
	adminToken := login(t, client, srv, "admin", "pw")
	guestToken := login(t, client, srv, "guest", "pw")
	now := time.Now().UTC()
	raidRecord := &raid.Raid{Name: "ICC", RaidLeaderID: adminID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(bdb).Create(ctx, raidRecord); err != nil {
		t.Fatal(err)
	}
	points := enhancepoints.NewRepo(bdb)
	for _, id := range []int64{ownerID, otherID, adminID, guestID} {
		if err := points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: id, Amount: 150, Type: "adjustment", Reason: "test balance", ActorID: adminID, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	lock := &enhancepoints.ItemLock{PlayerID: ownerID, RaidID: raidRecord.ID, Status: validation.LockStatusActive, LockedAt: now, ActorID: ownerID, CreatedAt: now, UpdatedAt: now}
	guestLock := &enhancepoints.ItemLock{PlayerID: guestID, RaidID: raidRecord.ID, Status: validation.LockStatusActive, LockedAt: now, ActorID: guestID, CreatedAt: now, UpdatedAt: now}
	for _, record := range []*enhancepoints.ItemLock{lock, guestLock} {
		if err := points.CreateLock(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	for _, caller := range []struct {
		name  string
		id    int64
		token string
	}{{"other member", otherID, otherToken}, {"admin", adminID, adminToken}, {"guest", guestID, guestToken}, {"unauthenticated", ownerID, ""}} {
		status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/points/roll", srv.URL, ownerID), caller.token, nil)
		if status != http.StatusNotFound {
			t.Fatalf("removed roll: %d %s", status, body)
		}
		for _, action := range []string{"locks", fmt.Sprintf("locks/%d/cancel", lock.ID)} {
			t.Run(caller.name+"/"+action, func(t *testing.T) {
				var requestBody any
				if action == "locks" {
					requestBody = map[string]any{"raid_id": raidRecord.ID}
				}
				wantStatus := http.StatusForbidden
				wantCode := "forbidden"
				if caller.token == "" {
					wantStatus = http.StatusUnauthorized
					wantCode = "unauthenticated"
				}
				status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/%s", srv.URL, ownerID, action), caller.token, requestBody)
				var out struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(body, &out); err != nil {
					t.Fatal(err)
				}
				if status != wantStatus || out.Error.Code != wantCode {
					t.Fatalf("want %d/%s, got %d %s", wantStatus, wantCode, status, body)
				}
			})
		}
		// The player's own URL must not allow cancellation of another player's lock.
		if caller.name == "other member" || caller.name == "admin" {
			status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/locks/%d/cancel", srv.URL, caller.id, lock.ID), caller.token, nil)
			if status != http.StatusNotFound {
				t.Fatalf("foreign lock under own URL: want 404, got %d %s", status, body)
			}
		}
	}
	for _, action := range []string{"locks", fmt.Sprintf("locks/%d/cancel", guestLock.ID)} {
		status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/%s", srv.URL, guestID, action), guestToken, map[string]any{"raid_id": raidRecord.ID})
		if status != http.StatusForbidden {
			t.Fatalf("guest own %s: want 403, got %d %s", action, status, body)
		}
	}
	balance, err := points.Balance(ctx, ownerID)
	if err != nil || balance != 150 {
		t.Fatalf("denied calls changed balance: %d, err %v", balance, err)
	}
	locks, err := points.ListLocksByPlayer(ctx, ownerID)
	if err != nil || len(locks) != 1 || locks[0].Status != validation.LockStatusActive {
		t.Fatalf("denied calls changed locks: %v, err %v", locks, err)
	}
	for i := 0; i < 2; i++ {
		status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/locks", srv.URL, ownerID), ownerToken, map[string]any{"raid_id": raidRecord.ID, "item_name": fmt.Sprintf("Test item %d", i)})
		if status != http.StatusCreated {
			t.Fatalf("create own lock: want 201, got %d %s", status, body)
		}
		var out struct {
			Data enhancepoints.ItemLock `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if out.Data.PlayerID != ownerID || out.Data.ActorID != ownerID || out.Data.Status != validation.LockStatusActive {
			t.Fatalf("invalid created lock: %+v", out.Data)
		}
	}
	balance, err = points.Balance(ctx, ownerID)
	if err != nil || balance != 150 {
		t.Fatalf("locks must not debit: %d, err %v", balance, err)
	}
	for _, expected := range []struct {
		status int
		code   string
	}{{http.StatusOK, ""}, {http.StatusConflict, "lock_not_active"}} {
		status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/locks/%d/cancel", srv.URL, ownerID, lock.ID), ownerToken, nil)
		if status != expected.status || expected.code != "" && !bytes.Contains(body, []byte(`"code":"`+expected.code+`"`)) {
			t.Fatalf("cancel own lock: want %d/%s, got %d %s", expected.status, expected.code, status, body)
		}
	}
	stored, err := points.GetLock(ctx, lock.ID)
	if err != nil || stored.Status != validation.LockStatusCancelled || stored.CancelledAt == nil {
		t.Fatalf("lock cancellation not saved: %+v, err %v", stored, err)
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

// TestListEndpointsReturnEmptyArrays locks in the API contract that list
// endpoints always encode an empty JSON array (never null) when there are no
// rows, so clients can iterate without null checks.
func TestListEndpointsReturnEmptyArrays(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()

	adminID := seedAccount(t, bdb, "admin", "admin-pw", validation.RoleAdmin)
	token := login(t, client, srv, "admin", "admin-pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", token, map[string]any{"name": "ICC", "raid_leader_id": adminID})
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

	assertEmptyArray := func(path string) {
		t.Helper()
		status, body := do(t, client, http.MethodGet, srv.URL+path, token, nil)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status %d body %s", path, status, body)
		}
		var out struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("GET %s: decode: %v", path, err)
		}
		if string(out.Data) != "[]" {
			t.Fatalf("GET %s: want [] got %s", path, out.Data)
		}
	}

	for _, path := range []string{
		"/api/v1/polls",
		"/api/v1/guild-ranks",
		"/api/v1/raid-groups",
		"/api/v1/imports",
		fmt.Sprintf("/api/v1/players/%d/characters", adminID),
		fmt.Sprintf("/api/v1/players/%d/locks", adminID),
		fmt.Sprintf("/api/v1/players/%d/loot", adminID),
		fmt.Sprintf("/api/v1/raids/%d/attendance", raidID),
		fmt.Sprintf("/api/v1/raids/%d/loot", raidID),
	} {
		assertEmptyArray(path)
	}

	for _, path := range []string{
		fmt.Sprintf("/api/v1/players/%d/points", adminID),
		fmt.Sprintf("/api/v1/players/%d/tokens", adminID),
	} {
		status, body := do(t, client, http.MethodGet, srv.URL+path, token, nil)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status %d body %s", path, status, body)
		}
		if !bytes.Contains(body, []byte(`"transactions":[]`)) {
			t.Fatalf("GET %s: want empty transactions array, got %s", path, body)
		}
	}
}
