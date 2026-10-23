package httpapi_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"raid-website/internal/audit"
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
	path := filepath.Join(t.TempDir(), "api.db")
	copyMigratedDatabase(t, path)
	bdb, err := db.Open(ctx, "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = bdb.Close() })
	handler, err := httpapi.New(bdb.DB, opts)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, bdb.DB
}

// migratedTemplate is a database file with every migration applied, built once per
// test binary. Each test gets its own copy, which is much faster than migrating
// a fresh database for every test.
var migratedTemplate struct {
	once sync.Once
	data []byte
	err  error
}

func copyMigratedDatabase(t *testing.T, dst string) {
	t.Helper()
	migratedTemplate.once.Do(func() {
		dir, err := os.MkdirTemp("", "httpapi-template-")
		if err != nil {
			migratedTemplate.err = err
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "template.db")
		ctx := context.Background()
		bdb, err := db.Open(ctx, "file:"+path+"?_pragma=foreign_keys(1)")
		if err != nil {
			migratedTemplate.err = fmt.Errorf("open template: %w", err)
			return
		}
		m, err := migrations.NewMigrator(bdb.DB)
		if err == nil {
			_, err = m.Migrate(ctx)
		}
		if closeErr := bdb.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			migratedTemplate.err = fmt.Errorf("migrate template: %w", err)
			return
		}
		migratedTemplate.data, migratedTemplate.err = os.ReadFile(path)
	})
	if migratedTemplate.err != nil {
		t.Fatalf("migrated template: %v", migratedTemplate.err)
	}
	if err := os.WriteFile(dst, migratedTemplate.data, 0o600); err != nil {
		t.Fatalf("copy template: %v", err)
	}
}

type fakeRegistrationFeed struct {
	characters map[string]registration.ExternalCharacter
	searchErr  error
	loadErr    error
}

func (f fakeRegistrationFeed) SearchCharacters(ctx context.Context, query string) ([]registration.ExternalCharacter, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	var rows []registration.ExternalCharacter
	for _, row := range f.characters {
		if strings.Contains(strings.ToLower(row.Name), strings.ToLower(query)) {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (f fakeRegistrationFeed) LoadCharacter(ctx context.Context, profileID string) (registration.ExternalCharacter, string, error) {
	if f.loadErr != nil {
		return registration.ExternalCharacter{}, "", f.loadErr
	}
	row, ok := f.characters[profileID]
	if !ok {
		return registration.ExternalCharacter{}, "", registration.ErrExternalNotFound
	}
	return row, `{"fixture":true}`, nil
}

func seedAccount(t *testing.T, bdb *bun.DB, username, password string, role validation.Role) int64 {
	t.Helper()
	// Minimum bcrypt cost keeps test logins fast; production hashing is tested in internal/auth.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	now := time.Now().UTC()
	uname := strings.ToLower(username)
	p := &player.Player{
		Name:         username,
		Username:     &uname,
		PasswordHash: string(hash),
		Role:         role,
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := player.NewRepo(bdb).Create(context.Background(), p); err != nil {
		t.Fatalf("seed account %s: %v", username, err)
	}
	if role == validation.RoleAdmin {
		enableTestTOTP(t, bdb, p.ID)
	}
	return p.ID
}

// testTOTPSecret is the authenticator secret of every seeded ADMIN: Admin-only
// routes require a second factor (SC-040), so test Admins log in with a TOTP code.
const testTOTPSecret = "JBSWY3DPEHPK3PXP"

func enableTestTOTP(t *testing.T, bdb *bun.DB, playerID int64) {
	t.Helper()
	now := time.Now().UTC()
	if err := auth.NewRepo(bdb).UpsertMFASettings(context.Background(), &auth.MFASettings{PlayerID: playerID, TOTPSecret: testTOTPSecret, TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("enable test TOTP: %v", err)
	}
}

// testTOTPCode computes the current RFC 6238 code for secret, as an authenticator app does.
func testTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatalf("decode TOTP secret: %v", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", bin%1000000)
}

func login(t *testing.T, client *http.Client, srv *httptest.Server, username, password string) string {
	t.Helper()
	resp := postJSON(t, client, srv.URL+"/api/v1/auth/login", map[string]any{"username": username, "password": password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: status %d", username, resp.StatusCode)
	}
	var started struct {
		Data struct {
			MFARequired bool   `json:"mfa_required"`
			Challenge   string `json:"challenge"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &started)
	if started.Data.MFARequired {
		verified := postJSON(t, client, srv.URL+"/api/v1/auth/mfa/verify", map[string]any{"challenge": started.Data.Challenge, "code": testTOTPCode(t, testTOTPSecret)})
		defer verified.Body.Close()
		if verified.StatusCode != http.StatusOK {
			t.Fatalf("login %s: MFA status %d", username, verified.StatusCode)
		}
		resp = verified
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName {
			return c.Value
		}
	}
	t.Fatalf("login %s: no session cookie", username)
	return ""
}

func postJSON(t *testing.T, client *http.Client, url string, payload any) *http.Response {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
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

	status, _ = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, plain), "", nil)
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

	_, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, rewarded), token, nil)
	if err := json.Unmarshal(body, &pointsOut); err != nil {
		t.Fatalf("decode rewarded points: %v", err)
	}
	if pointsOut.Data.Balance != 0 {
		t.Fatalf("rewarded balance: want 0 got %d", pointsOut.Data.Balance)
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

func TestAuditLogIsVisibleOnlyToAdmins(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin", "admin-pw", validation.RoleAdmin)
	seedAccount(t, bdb, "member", "member-pw", validation.RoleMember)
	adminToken := login(t, client, srv, "admin", "admin-pw")
	memberToken := login(t, client, srv, "member", "member-pw")

	now := time.Now().UTC()
	if err := audit.NewRepo(bdb).Insert(t.Context(), &audit.Log{
		ActorID:    &adminID,
		EntityType: "attendance",
		EntityID:   "42",
		Action:     "correction",
		BeforeJSON: `{"role":"DD"}`,
		AfterJSON:  `{"role":"HEAL"}`,
		Reason:     "Korrektur der Rolle",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("insert audit log: %v", err)
	}

	for _, caller := range []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "member", token: memberToken, wantStatus: http.StatusForbidden},
	} {
		t.Run(caller.name, func(t *testing.T) {
			status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/audit-log", caller.token, nil)
			if status != caller.wantStatus {
				t.Fatalf("list audit log: want %d got %d body %s", caller.wantStatus, status, body)
			}
		})
	}

	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/audit-log", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("admin list audit log: status %d body %s", status, body)
	}
	var out struct {
		Data []audit.Entry `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode audit log: %v", err)
	}
	if len(out.Data) != 1 || out.Data[0].ActorName == nil || *out.Data[0].ActorName != "admin" || out.Data[0].BeforeJSON != `{"role":"DD"}` || out.Data[0].AfterJSON != `{"role":"HEAL"}` {
		t.Fatalf("unexpected audit log response: %s", body)
	}
}

func TestPublicDashboardReads(t *testing.T) {
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
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", token, map[string]any{"title": "Termin"})
	if status != http.StatusCreated {
		t.Fatalf("create poll: status %d body %s", status, body)
	}

	for _, path := range []string{
		"/api/v1/raids",
		"/api/v1/players",
		"/api/v1/polls",
		fmt.Sprintf("/api/v1/raids/%d/attendance", raidOut.Data.ID),
	} {
		status, body = do(t, client, http.MethodGet, srv.URL+path, "", nil)
		if status != http.StatusOK {
			t.Fatalf("anonymous GET %s: want 200 got %d body %s", path, status, body)
		}
	}

	status, _ = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/points", srv.URL, adminID), "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous points read: want 401 got %d", status)
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
