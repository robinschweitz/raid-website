package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	status, body := do(f.t, f.srv.Client(), http.MethodPost, f.srv.URL+"/api/v1/push/subscriptions", token, map[string]any{"endpoint": endpoint, "p256dh": testP256dh, "auth": testAuth})
	if status != want {
		f.t.Fatalf("subscribe %s: status %d body %s", endpoint, status, body)
	}
	return body
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

// TestReqPush011_SubscriptionEndsWithItsSession: a subscription belongs to the
// session that made it. Logging out on the server, or any other end of that
// session, removes it, while the player's other devices keep theirs; a legacy
// row without a session gets no messages (FR-32.3, ADR 103).
func TestReqPush011_SubscriptionEndsWithItsSession(t *testing.T) {
	f := newPushFixture(t)
	client := f.srv.Client()
	secondToken := login(t, client, f.srv, "push-member", "password")
	f.subscribe(f.memberToken, endpointFor("phone"), http.StatusCreated)
	f.subscribe(secondToken, endpointFor("laptop"), http.StatusCreated)

	if status, raw := do(t, client, http.MethodPost, f.srv.URL+"/api/v1/auth/logout", f.memberToken, nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("logout: %d %s", status, raw)
	}
	if f.subscriptionCount(endpointFor("phone")) != 0 {
		t.Fatal("the logged-out session kept its push subscription")
	}
	if f.subscriptionCount(endpointFor("laptop")) != 1 {
		t.Fatal("logging out one device removed another device's subscription")
	}

	legacy := &push.Subscription{PlayerID: f.other, Endpoint: endpointFor("legacy"), P256dh: testP256dh, Auth: testAuth, CreatedAt: time.Now().UTC()}
	if _, err := f.db.NewInsert().Model(legacy).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status, raw := do(t, client, http.MethodPost, f.srv.URL+"/api/v1/polls", f.staffToken, map[string]any{"title": "Nach dem Abmelden"}); status != http.StatusCreated {
		t.Fatalf("create poll: %d %s", status, raw)
	}
	got := kindsByEndpoint(f.sender.take())
	if len(got) != 1 || len(got[endpointFor("laptop")]) != 1 {
		t.Fatalf("sent = %v, want only the laptop", got)
	}
}

// TestReqPush012_EndedSessionsEndSubscriptions: ending all sessions of a player,
// as an Admin password or role change does (FR-2.14, SC-017), also ends the
// push subscriptions of those sessions (FR-32.3).
func TestReqPush012_AdminPasswordChangeEndsSubscriptions(t *testing.T) {
	f := newPushFixture(t)
	f.subscribe(f.memberToken, endpointFor("member"), http.StatusCreated)
	if err := auth.NewRepo(f.db).DeleteSessionsOfPlayer(context.Background(), f.member, ""); err != nil {
		t.Fatal(err)
	}
	if f.subscriptionCount(endpointFor("member")) != 0 {
		t.Fatal("ending the player's sessions kept the push subscription")
	}
}

// TestReqPush013_RepeatedSubscribeIsQuiet: the client repeats the subscribe
// call on every visit; an unchanged subscription writes nothing new.
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
