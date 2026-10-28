package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

type correctionFixture struct {
	t                                                *testing.T
	srv                                              *httptest.Server
	db                                               *bun.DB
	token, memberToken                               string
	actor, first, second                             int64
	firstChar, secondChar                            int64
	raidID                                           int64
	tokenBoss, tokenItem, ordinaryBoss, ordinaryItem string
}

func newCorrectionFixture(t *testing.T) *correctionFixture {
	t.Helper()
	srv, db := setupServer(t)
	f := &correctionFixture{t: t, srv: srv, db: db}
	f.actor = seedAccount(t, db, "correction-admin", "password", validation.RoleAdmin)
	f.first = seedAccount(t, db, "correction-first", "password", validation.RoleMember)
	f.second = seedAccount(t, db, "correction-second", "password", validation.RoleMember)
	f.token = login(t, srv.Client(), srv, "correction-admin", "password")
	f.memberToken = login(t, srv.Client(), srv, "correction-first", "password")
	now := time.Now().UTC()
	for i, playerID := range []int64{f.first, f.second} {
		c := &character.Character{PlayerID: playerID, Name: fmt.Sprintf("Correction%d", i), Class: "Mage", MainSpec: "Fire", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := character.NewRepo(db).Create(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			f.firstChar = c.ID
		} else {
			f.secondChar = c.ID
		}
	}
	f.tokenBoss, f.tokenItem, f.ordinaryBoss, f.ordinaryItem = bundledICCDrops(t, db)
	instance, size := "631", 25
	rd := &raid.Raid{Name: "Correction raid", CatalogInstanceID: &instance, RaidSize: &size, RaidLeaderID: f.actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(db).Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	f.raidID = rd.ID
	return f
}

func bundledICCDrops(t *testing.T, db *bun.DB) (tokenBoss, tokenItem, ordinaryBoss, ordinaryItem string) {
	t.Helper()
	catalog, err := lootcatalog.NewService(lootcatalog.NewRepo(db)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, item := range catalog.Items {
		names[item.ItemID] = item.Name
	}
	bosses := map[string]bool{}
	for _, boss := range catalog.Bosses {
		bosses[boss.ID] = boss.InstanceID == "631"
	}
	for _, drop := range catalog.Drops {
		if !bosses[drop.BossID] || drop.RaidSize != 25 {
			continue
		}
		if names[drop.ItemID] == "Weiheabzeichen des Eroberers" {
			tokenBoss, tokenItem = drop.BossID, drop.ItemID
		} else if !loot.IsTokenAward(names[drop.ItemID]) {
			ordinaryBoss, ordinaryItem = drop.BossID, drop.ItemID
		}
	}
	if tokenItem == "" || ordinaryItem == "" {
		t.Fatal("bundled ICC25 token and ordinary drops required")
	}
	return
}

func (f *correctionFixture) selection(body map[string]any, token bool) map[string]any {
	body["catalog_boss_id"], body["item_id"] = f.ordinaryBoss, f.ordinaryItem
	if token {
		body["catalog_boss_id"], body["item_id"] = f.tokenBoss, f.tokenItem
	}
	return body
}

func (f *correctionFixture) request(method, suffix string, body any, want int) []byte {
	f.t.Helper()
	status, result := do(f.t, f.srv.Client(), method, fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix), f.token, body)
	if status != want {
		f.t.Fatalf("%s %s: want %d got %d: %s", method, suffix, want, status, result)
	}
	return result
}

func correctionID(t *testing.T, body []byte) int64 {
	t.Helper()
	var out struct{ Data struct{ ID int64 } }
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out.Data.ID
}

func (f *correctionFixture) attend(playerID, charID int64) int64 {
	return correctionID(f.t, f.request(http.MethodPost, "/attendance", map[string]any{"player_id": playerID, "character_id": charID, "role": "DD"}, http.StatusCreated))
}

// legacyCompleted models historical unarchived raids, not the current final archive flow.
func (f *correctionFixture) legacyCompleted(rewardedPlayerIDs ...int64) {
	f.t.Helper()
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		f.t.Fatal(err)
	}
	now := time.Now().UTC()
	rd.Status = validation.RaidStatusCompleted
	rd.CompletedAt = &now
	rd.ArchivedAt = nil
	rd.UpdatedAt = now
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		f.t.Fatal(err)
	}
	for _, playerID := range rewardedPlayerIDs {
		entry := &enhancepoints.Transaction{PlayerID: playerID, RaidID: &f.raidID, Amount: 10, Type: "award_no_mainspec_loot", Reason: "legacy raid completion", ActorID: f.actor, CreatedAt: now}
		if err := enhancepoints.NewRepo(f.db).InsertTransaction(ctx, entry); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *correctionFixture) points(playerID int64, want int) {
	f.t.Helper()
	got, err := enhancepoints.NewRepo(f.db).Balance(context.Background(), playerID)
	if err != nil || got != want {
		f.t.Fatalf("points player %d: got %d want %d error %v", playerID, got, want, err)
	}
}

func (f *correctionFixture) archiveHistory() {
	f.t.Helper()
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		f.t.Fatal(err)
	}
	now := time.Now().UTC()
	rd.Status = validation.RaidStatusCompleted
	rd.ArchivedAt = &now
	if rd.CompletedAt == nil {
		rd.CompletedAt = &now
	}
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		f.t.Fatal(err)
	}
}

func (f *correctionFixture) tokenBalance(playerID int64, want int) {
	f.t.Helper()
	rows, err := loot.NewRepo(f.db).ListByPlayer(context.Background(), playerID)
	if err != nil {
		f.t.Fatal(err)
	}
	net := 0
	for _, row := range rows {
		if loot.IsTokenAward(row.ItemName) {
			net++
		}
	}
	if net != want {
		f.t.Fatalf("tokens player %d: got %d want %d", playerID, net, want)
	}
}

func TestLegacyCorrectionsRepeatedEPAndTokenCompensation(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "MAIN"}, true), http.StatusCreated))
	f.legacyCompleted(f.second)
	f.archiveHistory()
	f.points(f.first, 0)
	f.points(f.second, 10)
	f.tokenBalance(f.first, 1)
	suffix := fmt.Sprintf("/loot/%d", awardID)
	for i := 0; i < 3; i++ {
		f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "recipient correction"}, http.StatusOK)
		f.points(f.first, 0)
		f.points(f.second, 10)
		f.tokenBalance(f.first, 0)
		f.tokenBalance(f.second, 1)
		f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "reason": "undo recipient correction"}, http.StatusOK)
		f.points(f.first, 0)
		f.points(f.second, 10)
		f.tokenBalance(f.first, 1)
		f.tokenBalance(f.second, 0)
	}
	f.request(http.MethodPatch, suffix, f.selection(map[string]any{"award_method": "SECOND", "reason": "item correction"}, false), http.StatusOK)
	f.points(f.first, 0)
	f.tokenBalance(f.first, 0)
	f.request(http.MethodPatch, suffix, f.selection(map[string]any{"award_method": "MAIN", "reason": "restore"}, true), http.StatusOK)
	f.points(f.first, 0)
	f.tokenBalance(f.first, 1)
	before, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
	if err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPatch, suffix, map[string]any{"notes": "notes only", "reason": "clarify"}, http.StatusOK)
	after, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("unchanged eligibility appended EP")
	}
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "loot_award", fmt.Sprint(awardID))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 9 {
		t.Fatalf("audit count: %d", len(logs))
	}
	for _, log := range logs {
		if log.ActorID == nil || *log.ActorID != f.actor || log.Reason == "" || !json.Valid([]byte(log.BeforeJSON)) || !json.Valid([]byte(log.AfterJSON)) {
			t.Fatalf("invalid audit: %+v", log)
		}
	}
	initialEP, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.second)
	if err != nil {
		t.Fatal(err)
	}
	if len(initialEP) == 0 || initialEP[0].Type != "award_no_mainspec_loot" || initialEP[0].Amount != 10 {
		t.Fatal("initial EP transaction was changed")
	}
	f.tokenBalance(f.first, 1)
}

func TestLegacyCorrectionsAttendanceAndCompletedAdditions(t *testing.T) {
	f := newCorrectionFixture(t)
	id := f.attend(f.first, f.firstChar)
	f.legacyCompleted(f.first)
	f.archiveHistory()
	f.points(f.first, 10)
	suffix := fmt.Sprintf("/attendance/%d", id)
	for i := 0; i < 2; i++ {
		f.request(http.MethodPatch, suffix, map[string]any{"player_id": f.second, "character_id": f.secondChar, "is_bench": true, "is_substitute": true, "role": "HEAL", "reason": "participant correction"}, http.StatusOK)
		f.points(f.first, 0)
		f.points(f.second, 10)
		f.request(http.MethodPatch, suffix, map[string]any{"player_id": f.first, "character_id": f.firstChar, "reason": "undo"}, http.StatusOK)
		f.points(f.first, 10)
		f.points(f.second, 0)
	}
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "DD", "reason": "missing participant"}, http.StatusCreated)
	f.points(f.second, 10)
	f.request(http.MethodPost, "/loot", f.selection(map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "award_method": "MAIN", "reason": "missing award"}, false), http.StatusCreated)
	f.points(f.second, 0)
}

func TestLegacyCorrectionsPermissionsReferencesAndStates(t *testing.T) {
	f := newCorrectionFixture(t)
	id := f.attend(f.first, f.firstChar)
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.first, "character_id": f.secondChar, "role": "DD"}, http.StatusBadRequest)
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": 99999, "character_id": f.firstChar, "role": "DD"}, http.StatusNotFound)
	f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "awarded_to_player_id": f.first, "awarded_to_character_id": f.secondChar}, false), http.StatusBadRequest)
	f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "awarded_to_player_id": f.first, "awarded_to_character_id": 99999}, false), http.StatusNotFound)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar}, false), http.StatusCreated))
	f.legacyCompleted(f.first)
	f.archiveHistory()
	for _, suffix := range []string{fmt.Sprintf("/attendance/%d", id), fmt.Sprintf("/loot/%d", awardID)} {
		status, _ := do(t, f.srv.Client(), http.MethodPatch, fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix), f.memberToken, map[string]any{"reason": "denied"})
		if status != http.StatusForbidden {
			t.Fatalf("permission status %d", status)
		}
		f.request(http.MethodPatch, suffix, map[string]any{"reason": " \t "}, http.StatusBadRequest)
	}
	for _, suffix := range []string{"/attendance", "/loot"} {
		status, _ := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix), f.memberToken, map[string]any{"reason": "denied"})
		if status != http.StatusForbidden {
			t.Fatalf("addition permission status %d", status)
		}
	}
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "DD"}, http.StatusBadRequest)
	f.request(http.MethodPost, "/loot", map[string]any{"item_name": "item", "awarded_to_player_id": f.first}, http.StatusBadRequest)
	f.request(http.MethodPatch, fmt.Sprintf("/attendance/%d", id), map[string]any{"character_id": f.secondChar, "reason": "wrong owner"}, http.StatusBadRequest)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"awarded_to_character_id": f.secondChar, "reason": "wrong owner"}, http.StatusBadRequest)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"awarded_to_character_id": f.firstChar, "reason": "add character"}, http.StatusOK)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"awarded_to_character_id": nil, "reason": "clear character"}, http.StatusBadRequest)
	cleared, err := loot.NewRepo(f.db).Get(context.Background(), awardID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.AwardedToCharacterID == nil || *cleared.AwardedToCharacterID != f.firstChar {
		t.Fatal("invalid clear changed character")
	}
	f.request(http.MethodPatch, "/attendance/99999", map[string]any{"reason": "missing"}, http.StatusNotFound)
	f.request(http.MethodPatch, "/loot/99999", map[string]any{"reason": "missing"}, http.StatusNotFound)
	for _, state := range []validation.RaidStatus{validation.RaidStatusCancelled, validation.RaidStatusOpen} {
		rd, err := raid.NewRepo(f.db).Get(context.Background(), f.raidID)
		if err != nil {
			t.Fatal(err)
		}
		rd.Status = state
		if state == validation.RaidStatusOpen {
			now := time.Now().UTC()
			rd.ArchivedAt = &now
		}
		if err := raid.NewRepo(f.db).Update(context.Background(), rd); err != nil {
			t.Fatal(err)
		}
		f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "DD", "reason": "blocked"}, http.StatusConflict)
		f.request(http.MethodPost, "/loot", map[string]any{"item_name": "item", "awarded_to_player_id": f.first, "reason": "blocked"}, http.StatusConflict)
		f.request(http.MethodPatch, fmt.Sprintf("/attendance/%d", id), map[string]any{"reason": "blocked"}, http.StatusConflict)
		f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"reason": "blocked"}, http.StatusConflict)
	}
}

func TestLegacyCorrectionsAuditFailureRollsBackRecordAndLedgers(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "MAIN", "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar}, true), http.StatusCreated))
	f.legacyCompleted()
	f.archiveHistory()
	if _, err := f.db.ExecContext(context.Background(), "CREATE TRIGGER fail_correction_audit BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), f.selection(map[string]any{"award_method": "SECOND", "reason": "rollback"}, false), http.StatusInternalServerError)
	f.points(f.first, 0)
	f.tokenBalance(f.first, 1)
	award, err := loot.NewRepo(f.db).Get(context.Background(), awardID)
	if err != nil {
		t.Fatal(err)
	}
	if award.ItemName != "Weiheabzeichen des Eroberers" || !award.IsMainSpec {
		t.Fatal("record update escaped rollback")
	}
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "DD", "reason": "rollback"}, http.StatusInternalServerError)
	f.points(f.second, 0)
	rows, err := attendance.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatal("attendance addition escaped rollback")
	}
}

func TestLegacyOrdinaryLootLockResolutionAndCorrectionDoesNotRevive(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemID: f.ordinaryItem, ItemName: "old snapshot", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"item_name": "untrusted name", "award_method": "SECOND", "awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar}, false), http.StatusCreated))
	award, err := loot.NewRepo(f.db).Get(context.Background(), awardID)
	if err != nil {
		t.Fatal(err)
	}
	if award.AwardedToPlayerID != f.first || !award.IsMainSpec || award.ItemLockID == nil || *award.ItemLockID != lock.ID || award.AwardedToCharacterID == nil || *award.AwardedToCharacterID != f.firstChar {
		t.Fatalf("lock not applied: %+v", award)
	}
	f.legacyCompleted()
	f.archiveHistory()
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "explicit recipient correction", "confirm_ep_shortfall": true}, http.StatusOK)
	stored, err := enhancepoints.NewRepo(f.db).GetLock(context.Background(), lock.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.LockStatusUsed || stored.UsedAt == nil {
		t.Fatal("correction revived consumed lock")
	}
}

func TestOrdinaryLootAmbiguousAndInvalidLocks(t *testing.T) {
	f := newCorrectionFixture(t)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemID: f.ordinaryItem, ItemName: "old snapshot", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "item_lock_id": lock.ID, "awarded_to_player_id": f.second}, true), http.StatusBadRequest)
	f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "item_lock_id": 99999, "awarded_to_player_id": f.second}, true), http.StatusNotFound)
	duplicate := *lock
	duplicate.ID = 0
	duplicate.PlayerID = f.second
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), &duplicate); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "awarded_to_player_id": f.second}, false), http.StatusConflict)
	locks, err := enhancepoints.NewRepo(f.db).ListActiveLocksByRaid(context.Background(), f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 2 {
		t.Fatal("ambiguous match consumed a lock")
	}
}

func TestOrdinaryLootTokenLockFallback(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemName: " Weiheabzeichen des Eroberers ", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	body := f.selection(map[string]any{"award_method": "SECOND", "item_lock_id": lock.ID, "awarded_to_player_id": f.second}, true)
	id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	award, err := loot.NewRepo(f.db).Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if award.AwardedToPlayerID != f.first || !award.IsMainSpec {
		t.Fatal("name fallback did not resolve lock")
	}
	f.tokenBalance(f.first, 1)
	f.tokenBalance(f.second, 0)
	f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
}

func TestCorrectionsPlannedAndOpenAuditTokensPermissionsAndLocks(t *testing.T) {
	for _, state := range []validation.RaidStatus{validation.RaidStatusPlanned, validation.RaidStatusOpen} {
		t.Run(string(state), func(t *testing.T) {
			f := newCorrectionFixture(t)
			ctx := context.Background()
			rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
			if err != nil {
				t.Fatal(err)
			}
			rd.Status = state
			if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
				t.Fatal(err)
			}
			attendanceID := f.attend(f.first, f.firstChar)
			now := time.Now().UTC()
			consumed := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemID: f.tokenItem, ItemName: "Weiheabzeichen des Eroberers", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
			if err := enhancepoints.NewRepo(f.db).CreateLock(ctx, consumed); err != nil {
				t.Fatal(err)
			}
			awardID := correctionID(t, f.request(http.MethodPost, "/loot", f.selection(map[string]any{"award_method": "SECOND", "awarded_to_player_id": f.second}, true), http.StatusCreated))
			active := &enhancepoints.ItemLock{PlayerID: f.second, RaidID: f.raidID, ItemID: f.ordinaryItem, ItemName: "old snapshot", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.second, CreatedAt: now, UpdatedAt: now}
			if err := enhancepoints.NewRepo(f.db).CreateLock(ctx, active); err != nil {
				t.Fatal(err)
			}
			attendanceSuffix := fmt.Sprintf("/attendance/%d", attendanceID)
			lootSuffix := fmt.Sprintf("/loot/%d", awardID)
			for _, suffix := range []string{attendanceSuffix, lootSuffix} {
				url := fmt.Sprintf("%s/api/v1/raids/%d%s", f.srv.URL, f.raidID, suffix)
				status, body := do(t, f.srv.Client(), http.MethodPatch, url, f.memberToken, map[string]any{"reason": "denied"})
				if status != http.StatusForbidden {
					t.Fatalf("member correction: %d %s", status, body)
				}
				status, _ = do(t, f.srv.Client(), http.MethodPatch, url, "", map[string]any{"reason": "denied"})
				if status != http.StatusUnauthorized {
					t.Fatalf("unauthenticated correction: %d", status)
				}
			}
			// The minimum raid-operations role can perform audited corrections.
			f.actor = seedAccount(t, f.db, "correction-assistent", "password", validation.RoleRaidassistent)
			f.token = login(t, f.srv.Client(), f.srv, "correction-assistent", "password")
			f.request(http.MethodPatch, attendanceSuffix, map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "HEAL", "is_bench": true, "reason": "correct participant"}, http.StatusOK)
			f.request(http.MethodPatch, lootSuffix, map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "correct recipient"}, http.StatusOK)
			f.tokenBalance(f.first, 0)
			f.tokenBalance(f.second, 1)
			f.attend(f.first, f.firstChar)
			f.request(http.MethodPatch, lootSuffix, f.selection(map[string]any{"awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "award_method": "SECOND", "reason": "correct item"}, false), http.StatusOK)
			f.tokenBalance(f.first, 0)
			f.tokenBalance(f.second, 0)
			award, err := loot.NewRepo(f.db).Get(ctx, awardID)
			if err != nil {
				t.Fatal(err)
			}
			if award.AwardedToPlayerID != f.first || award.IsMainSpec || award.ItemLockID == nil || *award.ItemLockID != consumed.ID {
				t.Fatalf("correction reapplied lock: %+v", award)
			}
			f.request(http.MethodPatch, lootSuffix, map[string]any{"item_lock_id": active.ID, "reason": "new lock not allowed"}, http.StatusBadRequest)
			f.request(http.MethodPatch, lootSuffix, f.selection(map[string]any{"reason": "restore token item"}, true), http.StatusOK)
			f.tokenBalance(f.first, 1)
			f.tokenBalance(f.second, 0)
			for _, playerID := range []int64{f.first, f.second} {
				rows, err := enhancepoints.NewRepo(f.db).ListByPlayer(ctx, playerID)
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 0 {
					t.Fatal("active corrections posted EP")
				}
			}
			awards, err := loot.NewRepo(f.db).ListByRaid(ctx, f.raidID)
			if err != nil {
				t.Fatal(err)
			}
			if len(awards) != 1 || awards[0].ID != awardID {
				t.Fatal("correction inserted another award")
			}
			for _, expected := range []struct {
				lockID int64
				status validation.LockStatus
			}{{consumed.ID, validation.LockStatusUsed}, {active.ID, validation.LockStatusActive}} {
				stored, err := enhancepoints.NewRepo(f.db).GetLock(ctx, expected.lockID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.Status != expected.status {
					t.Fatalf("correction changed lock %d to %s", expected.lockID, stored.Status)
				}
			}
			for _, expected := range []struct {
				entity string
				id     int64
				count  int
			}{{"attendance", attendanceID, 2}, {"loot_award", awardID, 3}} {
				logs, err := audit.NewRepo(f.db).ListByEntity(ctx, expected.entity, fmt.Sprint(expected.id))
				if err != nil {
					t.Fatal(err)
				}
				if len(logs) != expected.count {
					t.Fatalf("%s audit count: got %d want %d", expected.entity, len(logs), expected.count)
				}
				for _, log := range logs {
					if log.BeforeJSON == "null" {
						// The original addition is logged too (ADR 077); it has no reason.
						if log.ActorID == nil || !json.Valid([]byte(log.AfterJSON)) {
							t.Fatalf("invalid addition audit: %+v", log)
						}
						continue
					}
					if log.ActorID == nil || *log.ActorID != f.actor || log.Reason == "" || log.BeforeJSON == log.AfterJSON || !json.Valid([]byte(log.BeforeJSON)) || !json.Valid([]byte(log.AfterJSON)) {
						t.Fatalf("invalid active correction audit: %+v", log)
					}
				}
			}
			if _, err := f.db.ExecContext(ctx, "CREATE TRIGGER fail_active_audit BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END"); err != nil {
				t.Fatal(err)
			}
			f.request(http.MethodPatch, lootSuffix, map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "rollback recipient"}, http.StatusInternalServerError)
			f.tokenBalance(f.first, 1)
			f.tokenBalance(f.second, 0)
			award, err = loot.NewRepo(f.db).Get(ctx, awardID)
			if err != nil {
				t.Fatal(err)
			}
			if award.AwardedToPlayerID != f.first {
				t.Fatal("active loot correction escaped rollback")
			}
			f.request(http.MethodPatch, attendanceSuffix, map[string]any{"notes": "rollback notes", "reason": "rollback attendance"}, http.StatusInternalServerError)
			att, err := attendance.NewRepo(f.db).Get(ctx, attendanceID)
			if err != nil {
				t.Fatal(err)
			}
			if att.Notes != "" {
				t.Fatal("active attendance correction escaped rollback")
			}
		})
	}
}
