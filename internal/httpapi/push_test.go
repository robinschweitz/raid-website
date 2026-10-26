package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/auth"
	"raid-website/internal/character"
	"raid-website/internal/httpapi"
	"raid-website/internal/push"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

// A valid P-256 point and auth secret (RFC 8291 Appendix A).
const (
	testP256dh = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	testAuth   = "BTBZMqHH6r4Tts7J_aSIgg"
)

type sentPush struct {
	endpoint string
	kind     push.Kind
	payload  string
}

// recordingSender stands in for the push services.
type recordingSender struct {
	mu     sync.Mutex
	sent   []sentPush
	status map[string]int
	fail   bool
}

func (s *recordingSender) Send(_ context.Context, sub push.Subscription, kind push.Kind, payload []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, sentPush{endpoint: sub.Endpoint, kind: kind, payload: string(payload)})
	if s.fail {
		return 0, errors.New("push service unreachable")
	}
	if status, ok := s.status[sub.Endpoint]; ok {
		return status, nil
	}
	return http.StatusCreated, nil
}

func (s *recordingSender) take() []sentPush {
	s.mu.Lock()
	defer s.mu.Unlock()
	sent := s.sent
	s.sent = nil
	return sent
}

type pushFixture struct {
	t                       *testing.T
	srv                     *httptest.Server
	db                      *bun.DB
	sender                  *recordingSender
	staff, member, other    int64
	staffToken, memberToken string
	otherToken, guestToken  string
	memberChar              int64
	raidID                  int64
}

func newPushFixture(t *testing.T) *pushFixture {
	t.Helper()
	privateKey, err := push.GenerateVAPIDKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := push.ParseVAPIDKey(privateKey, "mailto:raid@example.org")
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{status: map[string]int{}}
	srv, db := setupServerWithOptions(t, httpapi.Options{VAPIDKey: key, PushSender: sender})
	f := &pushFixture{t: t, srv: srv, db: db, sender: sender}
	f.staff = seedAccount(t, db, "push-staff", "password", validation.RoleRaidassistent)
	f.member = seedAccount(t, db, "push-member", "password", validation.RoleMember)
	f.other = seedAccount(t, db, "push-other", "password", validation.RoleMember)
	seedAccount(t, db, "push-guest", "password", validation.RoleGuest)
	client := srv.Client()
	f.staffToken = login(t, client, srv, "push-staff", "password")
	f.memberToken = login(t, client, srv, "push-member", "password")
	f.otherToken = login(t, client, srv, "push-other", "password")
	f.guestToken = login(t, client, srv, "push-guest", "password")
	now := time.Now().UTC()
	c := &character.Character{PlayerID: f.member, Name: "Pushmage", Class: "Mage", MainSpec: "Fire", Active: true, IsMain: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(db).Create(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	f.memberChar = c.ID
	rd := &raid.Raid{Name: "Push raid", RaidLeaderID: f.staff, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(db).Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	f.raidID = rd.ID
	return f
}

func endpointFor(name string) string {
	return "https://fcm.googleapis.com/fcm/send/" + name
}

func (f *pushFixture) subscribe(token, endpoint string, want int) []byte {
	f.t.Helper()
	status, body, _ := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions", token, deviceFor(token), map[string]any{"endpoint": endpoint, "p256dh": testP256dh, "auth": testAuth})
	if status != want {
		f.t.Fatalf("subscribe %s: status %d body %s", endpoint, status, body)
	}
	return body
}

// deviceFor gives each test session its own browser, so helpers that do not
// care about devices still subscribe from a stable one.
func deviceFor(token string) string {
	return "device-of-" + token
}

// deviceDo sends a request with the session cookie and, when device is not
// empty, the raid_device cookie of that browser. It returns the device cookie
// the server set, if any.
func (f *pushFixture) deviceDo(method, path, token, device string, body any) (int, []byte, string) {
	f.t.Helper()
	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, f.srv.URL+path, reader)
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	if device != "" {
		req.AddCookie(&http.Cookie{Name: "raid_device", Value: device})
	}
	addCSRF(req)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	setDevice := ""
	for _, c := range resp.Cookies() {
		if c.Name == "raid_device" {
			setDevice = c.Value
		}
	}
	return resp.StatusCode, raw, setDevice
}

// deviceLogin signs a player in from the browser device and returns the session token.
func (f *pushFixture) deviceLogin(username, device string) string {
	f.t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+"/api/v1/auth/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":"password"}`, username)))
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "raid_device", Value: device})
	addCSRF(req)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			return c.Value
		}
	}
	f.t.Fatalf("login %s: status %d, no session", username, resp.StatusCode)
	return ""
}

// pollSent creates a poll as staff and returns which endpoints got a message.
func (f *pushFixture) pollSent(title string) map[string][]push.Kind {
	f.t.Helper()
	if status, raw := do(f.t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/polls", f.staffToken, map[string]any{"title": title}); status != http.StatusCreated {
		f.t.Fatalf("create poll: %d %s", status, raw)
	}
	return kindsByEndpoint(f.sender.take())
}

func (f *pushFixture) subscriptionCount(endpoint string) int {
	f.t.Helper()
	count, err := f.db.NewSelect().Table("push_subscriptions").Where("endpoint = ?", endpoint).Count(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return int(count)
}

func kindsByEndpoint(sent []sentPush) map[string][]push.Kind {
	out := map[string][]push.Kind{}
	for _, s := range sent {
		out[s.endpoint] = append(out[s.endpoint], s.kind)
	}
	return out
}

// TestReqPush004_PermissionBoundary: subscribing needs MEMBER (403 for GUEST),
// every push endpoint needs a session (401), and CSRF applies (FR-32.1).
func TestReqPush004_PermissionBoundary(t *testing.T) {
	f := newPushFixture(t)
	client := f.srv.Client()
	body := map[string]any{"endpoint": endpointFor("x"), "p256dh": testP256dh, "auth": testAuth}
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/push/config"},
		{http.MethodPost, "/api/v1/push/subscriptions"},
		{http.MethodDelete, "/api/v1/push/subscriptions"},
	} {
		if status, raw := do(t, client, tc.method, f.srv.URL+tc.path, "", body); status != http.StatusUnauthorized {
			t.Errorf("%s %s without session: %d %s", tc.method, tc.path, status, raw)
		}
	}
	if status, raw := do(t, client, http.MethodPost, f.srv.URL+"/api/v1/push/subscriptions", f.guestToken, body); status != http.StatusForbidden {
		t.Fatalf("guest subscribe: %d %s", status, raw)
	}
	if status, raw := do(t, client, http.MethodGet, f.srv.URL+"/api/v1/push/config", f.guestToken, nil); status != http.StatusForbidden {
		t.Fatalf("guest config: %d %s", status, raw)
	}
	if f.subscriptionCount(endpointFor("x")) != 0 {
		t.Fatal("a rejected request stored a subscription")
	}
	// Without the CSRF header the write is refused before the handler runs.
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+"/api/v1/push/subscriptions", strings.NewReader(`{"endpoint":"`+endpointFor("x")+`","p256dh":"`+testP256dh+`","auth":"`+testAuth+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", auth.CookieName+"="+f.memberToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("subscribe without CSRF: %d", resp.StatusCode)
	}
}

// TestReqPush005_SubscribeValidatesAndAudits: a member stores a subscription
// for an allowed push service; the audit entry names the service host only.
func TestReqPush005_SubscribeValidatesAndAudits(t *testing.T) {
	f := newPushFixture(t)
	status, raw := do(t, f.srv.Client(), http.MethodGet, f.srv.URL+"/api/v1/push/config", f.memberToken, nil)
	var config struct {
		Data struct {
			Enabled   bool   `json:"enabled"`
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if status != http.StatusOK || json.Unmarshal(raw, &config) != nil || !config.Data.Enabled || len(config.Data.PublicKey) != 87 {
		t.Fatalf("config: %d %s", status, raw)
	}
	body := f.subscribe(f.memberToken, endpointFor("member-secret-path"), http.StatusCreated)
	if strings.Contains(string(body), "member-secret-path") || strings.Contains(string(body), testAuth) {
		t.Fatalf("response echoes the subscription: %s", body)
	}
	var out struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "push_subscription", fmt.Sprint(out.Data.ID))
	if err != nil || len(logs) != 1 || logs[0].Action != "create" {
		t.Fatalf("audit = %+v, %v", logs, err)
	}
	if !strings.Contains(logs[0].AfterJSON, `"push_service":"fcm.googleapis.com"`) || strings.Contains(logs[0].AfterJSON, "member-secret-path") || strings.Contains(logs[0].AfterJSON, testAuth) || strings.Contains(logs[0].AfterJSON, testP256dh) {
		t.Fatalf("audit snapshot = %s", logs[0].AfterJSON)
	}
	for name, payload := range map[string]map[string]any{
		"other host":   {"endpoint": "https://127.0.0.1/push", "p256dh": testP256dh, "auth": testAuth},
		"plain http":   {"endpoint": "http://fcm.googleapis.com/fcm/send/x", "p256dh": testP256dh, "auth": testAuth},
		"bad key":      {"endpoint": endpointFor("bad"), "p256dh": "AAAA", "auth": testAuth},
		"missing auth": {"endpoint": endpointFor("bad"), "p256dh": testP256dh},
	} {
		if status, raw := do(t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/push/subscriptions", f.memberToken, payload); status != http.StatusBadRequest || errorCode(t, raw) != "invalid_push_subscription" {
			t.Errorf("%s: %d %s", name, status, raw)
		}
	}
	// The same browser signed in as another member moves to that account.
	f.subscribe(f.otherToken, endpointFor("member-secret-path"), http.StatusCreated)
	var owner int64
	if err := f.db.NewSelect().Table("push_subscriptions").Column("player_id").Where("endpoint = ?", endpointFor("member-secret-path")).Scan(context.Background(), &owner); err != nil || owner != f.other {
		t.Fatalf("owner = %d, %v", owner, err)
	}
}

// TestReqPush006_Unsubscribe: a player removes only their own subscription.
func TestReqPush006_Unsubscribe(t *testing.T) {
	f := newPushFixture(t)
	f.subscribe(f.memberToken, endpointFor("mine"), http.StatusCreated)
	client := f.srv.Client()
	if status, raw := do(t, client, http.MethodDelete, f.srv.URL+"/api/v1/push/subscriptions", f.otherToken, map[string]any{"endpoint": endpointFor("mine")}); status != http.StatusNotFound {
		t.Fatalf("foreign delete: %d %s", status, raw)
	}
	if f.subscriptionCount(endpointFor("mine")) != 1 {
		t.Fatal("foreign delete removed the subscription")
	}
	if status, raw := do(t, client, http.MethodDelete, f.srv.URL+"/api/v1/push/subscriptions", f.memberToken, map[string]any{"endpoint": endpointFor("mine")}); status != http.StatusNoContent {
		t.Fatalf("own delete: %d %s", status, raw)
	}
	if f.subscriptionCount(endpointFor("mine")) != 0 {
		t.Fatal("own delete kept the subscription")
	}
}

// TestReqPush007_NewPollNotifiesMembers: creating a poll notifies every
// subscribed member except the author, with the kind only (FR-32.2).
func TestReqPush007_NewPollNotifiesMembers(t *testing.T) {
	f := newPushFixture(t)
	f.subscribe(f.memberToken, endpointFor("member"), http.StatusCreated)
	f.subscribe(f.staffToken, endpointFor("staff"), http.StatusCreated)
	f.subscribe(f.otherToken, endpointFor("gone"), http.StatusCreated)
	f.sender.status[endpointFor("gone")] = http.StatusGone
	// A player demoted to GUEST after subscribing gets nothing.
	if _, err := f.db.NewUpdate().Table("players").Set("role = ?", validation.RoleGuest).Where("id = ?", f.other).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, raw := do(t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/polls", f.staffToken, map[string]any{"title": "Geheimer Umfragetitel"})
	if status != http.StatusCreated {
		t.Fatalf("create poll: %d %s", status, raw)
	}
	sent := f.sender.take()
	if len(sent) != 1 || sent[0].endpoint != endpointFor("member") || sent[0].kind != push.KindPoll || sent[0].payload != `{"kind":"poll"}` {
		t.Fatalf("sent = %+v", sent)
	}
	// 410 from the push service deletes the subscription.
	if _, err := f.db.NewUpdate().Table("players").Set("role = ?", validation.RoleMember).Where("id = ?", f.other).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status, raw := do(t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/polls", f.staffToken, map[string]any{"title": "Zweite"}); status != http.StatusCreated {
		t.Fatalf("create poll: %d %s", status, raw)
	}
	if got := kindsByEndpoint(f.sender.take()); len(got[endpointFor("gone")]) != 1 || len(got[endpointFor("member")]) != 1 {
		t.Fatalf("sent = %v", got)
	}
	if f.subscriptionCount(endpointFor("gone")) != 0 {
		t.Fatal("subscription answered 410 was kept")
	}
}

// TestReqPush008_FailedDeliveryKeepsTheWrite: an unreachable push service never
// fails the poll write (FR-32.4).
func TestReqPush008_FailedDeliveryKeepsTheWrite(t *testing.T) {
	f := newPushFixture(t)
	f.subscribe(f.memberToken, endpointFor("member"), http.StatusCreated)
	f.sender.fail = true
	if status, raw := do(t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/polls", f.staffToken, map[string]any{"title": "Trotzdem"}); status != http.StatusCreated {
		t.Fatalf("create poll: %d %s", status, raw)
	}
	if len(f.sender.take()) != 1 {
		t.Fatal("no delivery attempted")
	}
	if f.subscriptionCount(endpointFor("member")) != 1 {
		t.Fatal("a network error removed the subscription")
	}
}

// TestReqPush009_NewRaidAndRosterNotify: a new raid notifies members; a roster
// row notifies only the player whose character was added (FR-32.2).
func TestReqPush009_NewRaidAndRosterNotify(t *testing.T) {
	f := newPushFixture(t)
	f.subscribe(f.memberToken, endpointFor("member"), http.StatusCreated)
	f.subscribe(f.otherToken, endpointFor("other"), http.StatusCreated)
	client := f.srv.Client()
	if status, raw := do(t, client, http.MethodPost, f.srv.URL+"/api/v1/raids", f.staffToken, map[string]any{"name": "Neuer Raid", "raid_leader_id": f.staff}); status != http.StatusCreated {
		t.Fatalf("create raid: %d %s", status, raw)
	}
	if got := kindsByEndpoint(f.sender.take()); len(got) != 2 || got[endpointFor("member")][0] != push.KindRaid || got[endpointFor("other")][0] != push.KindRaid {
		t.Fatalf("raid sent = %v", got)
	}
	attendanceURL := fmt.Sprintf("%s/api/v1/raids/%d/attendance", f.srv.URL, f.raidID)
	if status, raw := do(t, client, http.MethodPost, attendanceURL, f.staffToken, map[string]any{"player_id": f.member, "character_id": f.memberChar, "role": "DD"}); status != http.StatusCreated {
		t.Fatalf("add roster row: %d %s", status, raw)
	}
	sent := f.sender.take()
	if len(sent) != 1 || sent[0].endpoint != endpointFor("member") || sent[0].kind != push.KindRoster {
		t.Fatalf("roster sent = %+v", sent)
	}
	// A failed roster write sends nothing.
	if status, _ := do(t, client, http.MethodPost, attendanceURL, f.staffToken, map[string]any{"player_id": f.member, "character_id": f.memberChar, "role": "DD"}); status < 400 {
		t.Fatalf("duplicate roster row accepted: %d", status)
	}
	if sent := f.sender.take(); len(sent) != 0 {
		t.Fatalf("failed write sent %+v", sent)
	}
}

// TestReqPush010_DisabledWithoutKey: without a VAPID key the feature is off.
func TestReqPush010_DisabledWithoutKey(t *testing.T) {
	srv, db := setupServer(t)
	seedAccount(t, db, "push-off", "password", validation.RoleMember)
	token := login(t, srv.Client(), srv, "push-off", "password")
	status, raw := do(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/push/config", token, nil)
	if status != http.StatusOK || !strings.Contains(string(raw), `"enabled":false`) {
		t.Fatalf("config: %d %s", status, raw)
	}
	status, raw = do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/push/subscriptions", token, map[string]any{"endpoint": endpointFor("x"), "p256dh": testP256dh, "auth": testAuth})
	if status != http.StatusConflict || errorCode(t, raw) != "push_disabled" {
		t.Fatalf("subscribe: %d %s", status, raw)
	}
}

// TestReqPush011_OptInSurvivesSignInOnTheSameDevice: the opt-in belongs to the
// player on this device. After the session ends (expiry) and the same player
// signs in again there, messages keep coming without a new opt-in (FR-32.3).
func TestReqPush011_OptInSurvivesSignInOnTheSameDevice(t *testing.T) {
	f := newPushFixture(t)
	const phone = "member-phone"
	token := f.deviceLogin("push-member", phone)
	if status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions", token, phone, map[string]any{"endpoint": endpointFor("phone"), "p256dh": testP256dh, "auth": testAuth}); status != http.StatusCreated {
		t.Fatalf("subscribe: %d %s", status, raw)
	}
	if _, err := f.db.NewDelete().Table("sessions").Where("player_id = ?", f.member).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.deviceLogin("push-member", phone)
	if got := f.pollSent("Nach neuer Anmeldung"); len(got[endpointFor("phone")]) != 1 {
		t.Fatalf("sent = %v, want the phone after signing in again", got)
	}
}

// TestReqPush012_AnotherPlayerOnTheDeviceEndsTheOptIn: when another player
// signs in on that browser, the server removes the previous player's
// subscription in the login itself; it never moves to the new player.
func TestReqPush012_AnotherPlayerOnTheDeviceEndsTheOptIn(t *testing.T) {
	f := newPushFixture(t)
	const shared = "shared-browser"
	token := f.deviceLogin("push-member", shared)
	if status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions", token, shared, map[string]any{"endpoint": endpointFor("shared"), "p256dh": testP256dh, "auth": testAuth}); status != http.StatusCreated {
		t.Fatalf("subscribe: %d %s", status, raw)
	}
	f.deviceLogin("push-other", shared)
	if f.subscriptionCount(endpointFor("shared")) != 0 {
		t.Fatal("the previous player's subscription survived another player's sign-in")
	}
	if got := f.pollSent("Fremdes Gerät"); len(got) != 0 {
		t.Fatalf("sent = %v, want nothing", got)
	}
	var reason string
	if err := f.db.NewSelect().Table("audit_log").Column("reason").Where("entity_type = ? AND action = ?", "push_subscription", "remove").Scan(context.Background(), &reason); err != nil || reason == "" {
		t.Fatalf("audit reason = %q, %v", reason, err)
	}
}

// TestReqPush014_SyncReportsAndClears: the browser checks in after a sign-in.
// Its own subscription is active and gets this device attached (rows from
// before migration 081); another player's subscription is removed, also when
// the browser has no device cookie any more.
func TestReqPush014_SyncReportsAndClears(t *testing.T) {
	f := newPushFixture(t)
	legacy := &push.Subscription{PlayerID: f.member, Endpoint: endpointFor("legacy"), P256dh: testP256dh, Auth: testAuth, CreatedAt: time.Now().UTC()}
	if _, err := f.db.NewInsert().Model(legacy).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	sync := func(token, device, endpoint string) (bool, string) {
		t.Helper()
		status, raw, setDevice := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions/sync", token, device, map[string]any{"endpoint": endpoint})
		var out struct {
			Data struct {
				Active bool `json:"active"`
			} `json:"data"`
		}
		if status != http.StatusOK || json.Unmarshal(raw, &out) != nil {
			t.Fatalf("sync: %d %s", status, raw)
		}
		return out.Data.Active, setDevice
	}
	active, newDevice := sync(f.memberToken, "", endpointFor("legacy"))
	if !active || newDevice == "" {
		t.Fatalf("own legacy subscription: active %v, device cookie %q", active, newDevice)
	}
	var stored string
	if err := f.db.NewSelect().Table("push_subscriptions").Column("device_hash").Where("id = ?", legacy.ID).Scan(context.Background(), &stored); err != nil || stored != push.HashDevice(newDevice) {
		t.Fatalf("device hash = %q, %v", stored, err)
	}
	if active, _ := sync(f.otherToken, "", endpointFor("legacy")); active {
		t.Fatal("another player's subscription reported active")
	}
	if f.subscriptionCount(endpointFor("legacy")) != 0 {
		t.Fatal("another player's subscription was kept for the browser now used by someone else")
	}
	if active, _ := sync(f.memberToken, newDevice, endpointFor("unknown")); active {
		t.Fatal("unknown endpoint reported active")
	}
	if status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions/sync", "", newDevice, map[string]any{"endpoint": endpointFor("legacy")}); status != http.StatusUnauthorized {
		t.Fatalf("without session: %d %s", status, raw)
	}
}

// TestReqPush015_SignOutAndAccountChanges: signing out ends the opt-in on that
// device only; an own password change ends it on the other devices; an Admin
// password change ends it everywhere (FR-2.13, FR-2.14, FR-32.3).
func TestReqPush015_SignOutAndAccountChanges(t *testing.T) {
	f := newPushFixture(t)
	devices := map[string]string{"phone": "dev-phone", "laptop": "dev-laptop", "tablet": "dev-tablet"}
	tokens := map[string]string{}
	for name, device := range devices {
		tokens[name] = f.deviceLogin("push-member", device)
		if status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions", tokens[name], device, map[string]any{"endpoint": endpointFor(name), "p256dh": testP256dh, "auth": testAuth}); status != http.StatusCreated {
			t.Fatalf("subscribe %s: %d %s", name, status, raw)
		}
	}
	if status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/auth/logout", tokens["phone"], devices["phone"], nil); status != http.StatusNoContent {
		t.Fatalf("logout: %d %s", status, raw)
	}
	if f.subscriptionCount(endpointFor("phone")) != 0 || f.subscriptionCount(endpointFor("laptop")) != 1 {
		t.Fatal("signing out did not end exactly the phone's subscription")
	}
	if status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/auth/password", tokens["laptop"], devices["laptop"], map[string]any{"current_password": "password", "new_password": "a-new-long-password"}); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("password change: %d %s", status, raw)
	}
	if f.subscriptionCount(endpointFor("laptop")) != 1 || f.subscriptionCount(endpointFor("tablet")) != 0 {
		t.Fatal("own password change must keep this device and end the others")
	}
	if err := push.RevokeForPlayer(context.Background(), f.db, f.staff, f.member, ""); err != nil {
		t.Fatal(err)
	}
	if f.subscriptionCount(endpointFor("laptop")) != 0 {
		t.Fatal("an account change by an Admin must end every device")
	}
}

// TestReqPush013_RepeatedSubscribeIsQuiet: subscribing the same browser again
// for the same player and device writes nothing new.
func TestReqPush013_RepeatedSubscribeIsQuiet(t *testing.T) {
	f := newPushFixture(t)
	f.subscribe(f.memberToken, endpointFor("member"), http.StatusCreated)
	f.subscribe(f.memberToken, endpointFor("member"), http.StatusCreated)
	count, err := f.db.NewSelect().Table("audit_log").Where("entity_type = ?", "push_subscription").Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("audit entries = %d, want 1", count)
	}
}

// loginDeviceCookie signs a player in with the given device cookie (empty for
// none) and returns the raid_device cookie the login response set.
func (f *pushFixture) loginDeviceCookie(username, device string) string {
	f.t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+"/api/v1/auth/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":"password"}`, username)))
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if device != "" {
		req.AddCookie(&http.Cookie{Name: "raid_device", Value: device})
	}
	addCSRF(req)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.t.Fatalf("login %s: status %d", username, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "raid_device" {
			return c.Value
		}
	}
	return ""
}

// TestReqPush016_SignInWithoutDeviceCookie: a browser that lost its device
// cookie still holds the previous player's push subscription. The login gives
// it a new device cookie; the server cannot know which subscription the
// browser holds, so the check-in of the new player removes it. Until then the
// old row stays, which is why the web client checks in before it shows the new
// player as signed in and drops the subscription itself if that fails.
// A login with a device cookie renews it.
func TestReqPush016_SignInWithoutDeviceCookie(t *testing.T) {
	f := newPushFixture(t)
	const original = "lost-device"
	token := f.deviceLogin("push-member", original)
	if status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions", token, original, map[string]any{"endpoint": endpointFor("lost"), "p256dh": testP256dh, "auth": testAuth}); status != http.StatusCreated {
		t.Fatalf("subscribe: %d %s", status, raw)
	}
	if renewed := f.loginDeviceCookie("push-member", original); renewed != original {
		t.Fatalf("login with a device cookie set %q, want the same cookie renewed", renewed)
	}

	fresh := f.loginDeviceCookie("push-other", "")
	if fresh == "" || fresh == original {
		t.Fatalf("login without a device cookie set %q, want a new one", fresh)
	}
	otherToken := f.deviceLogin("push-other", fresh)
	status, raw, _ := f.deviceDo(http.MethodPost, "/api/v1/push/subscriptions/sync", otherToken, fresh, map[string]any{"endpoint": endpointFor("lost")})
	if status != http.StatusOK || !strings.Contains(string(raw), `"active":false`) {
		t.Fatalf("check-in: %d %s", status, raw)
	}
	if f.subscriptionCount(endpointFor("lost")) != 0 {
		t.Fatal("the previous player's subscription survived the new player's check-in")
	}
	if got := f.pollSent("Nach dem Wechsel"); len(got) != 0 {
		t.Fatalf("sent = %v, want nothing", got)
	}
}

// TestReqPush017_UnclaimedRowsGetNothingAndExpire: a subscription without a
// device (from before migration 081) gets no messages until its own player's
// browser claims it, and is deleted after 30 days unclaimed (ADR 103).
func TestReqPush017_UnclaimedRowsGetNothingAndExpire(t *testing.T) {
	f := newPushFixture(t)
	now := time.Now().UTC()
	old := &push.Subscription{PlayerID: f.member, Endpoint: endpointFor("old"), P256dh: testP256dh, Auth: testAuth, CreatedAt: now.Add(-push.UnclaimedLifetime - time.Hour)}
	recent := &push.Subscription{PlayerID: f.member, Endpoint: endpointFor("recent"), P256dh: testP256dh, Auth: testAuth, CreatedAt: now}
	for _, sub := range []*push.Subscription{old, recent} {
		if _, err := f.db.NewInsert().Model(sub).Exec(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.pollSent("Ohne Gerät"); len(got) != 0 {
		t.Fatalf("sent = %v, want nothing for rows without a device", got)
	}
	removed, err := push.NewService(push.NewRepo(f.db), nil, t.Logf).DeleteUnclaimed(context.Background())
	if err != nil || removed != 1 {
		t.Fatalf("removed %d, %v; want 1", removed, err)
	}
	if f.subscriptionCount(endpointFor("old")) != 0 || f.subscriptionCount(endpointFor("recent")) != 1 {
		t.Fatal("cleanup must remove only rows unclaimed for 30 days")
	}
}
