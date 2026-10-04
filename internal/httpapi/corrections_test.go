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
	"raid-website/internal/raid"
	"raid-website/internal/tokens"
	"raid-website/internal/validation"
)

type correctionFixture struct {
	t                     *testing.T
	srv                   *httptest.Server
	db                    *bun.DB
	token, memberToken    string
	actor, first, second  int64
	firstChar, secondChar int64
	raidID                int64
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
	rd := &raid.Raid{Name: "Correction raid", RaidLeaderID: f.actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(db).Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	f.raidID = rd.ID
	return f
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

func (f *correctionFixture) points(playerID int64, want int) {
	f.t.Helper()
	got, err := enhancepoints.NewRepo(f.db).Balance(context.Background(), playerID)
	if err != nil || got != want {
		f.t.Fatalf("points player %d: got %d want %d error %v", playerID, got, want, err)
	}
}

func (f *correctionFixture) tokenBalance(playerID int64, want int) {
	f.t.Helper()
	rows, err := tokens.NewRepo(f.db).ListByPlayer(context.Background(), playerID)
	if err != nil {
		f.t.Fatal(err)
	}
	net := 0
	for _, row := range rows {
		net += row.Amount
	}
	if net != want {
		f.t.Fatalf("tokens player %d: got %d want %d", playerID, net, want)
	}
}

func TestCorrectionsRepeatedEPAndTokenCompensation(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", map[string]any{"item_name": "Weiheabzeichen des Eroberers", "awarded_to_player_id": f.first, "is_main_spec": true}, http.StatusCreated))
	f.request(http.MethodPost, "/complete", nil, http.StatusOK)
	f.points(f.first, 0)
	f.points(f.second, 10)
	f.tokenBalance(f.first, 1)
	suffix := fmt.Sprintf("/loot/%d", awardID)
	for i := 0; i < 3; i++ {
		f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_player_id": f.second, "reason": "recipient correction"}, http.StatusOK)
		f.points(f.first, 10)
		f.points(f.second, 0)
		f.tokenBalance(f.first, 0)
		f.tokenBalance(f.second, 1)
		f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_player_id": f.first, "reason": "undo recipient correction"}, http.StatusOK)
		f.points(f.first, 0)
		f.points(f.second, 10)
		f.tokenBalance(f.first, 1)
		f.tokenBalance(f.second, 0)
	}
	f.request(http.MethodPatch, suffix, map[string]any{"item_name": "ordinary item", "is_main_spec": false, "reason": "item correction"}, http.StatusOK)
	f.points(f.first, 10)
	f.tokenBalance(f.first, 0)
	f.request(http.MethodPatch, suffix, map[string]any{"item_name": "Weiheabzeichen des Eroberers", "is_main_spec": true, "reason": "restore"}, http.StatusOK)
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
	initialTokens, err := tokens.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
	if err != nil {
		t.Fatal(err)
	}
	if len(initialTokens) == 0 || initialTokens[0].Type != "award" || initialTokens[0].Amount != 1 {
		t.Fatal("initial token transaction was changed")
	}
}

func TestCorrectionsAttendanceAndCompletedAdditions(t *testing.T) {
	f := newCorrectionFixture(t)
	id := f.attend(f.first, f.firstChar)
	f.request(http.MethodPost, "/complete", nil, http.StatusOK)
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
	f.request(http.MethodPost, "/loot", map[string]any{"item_name": "missing item", "awarded_to_player_id": f.second, "is_main_spec": true, "reason": "missing award"}, http.StatusCreated)
	f.points(f.second, 0)
}

func TestCorrectionsPermissionsReferencesAndStates(t *testing.T) {
	f := newCorrectionFixture(t)
	id := f.attend(f.first, f.firstChar)
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.first, "character_id": f.secondChar, "role": "DD"}, http.StatusBadRequest)
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": 99999, "character_id": f.firstChar, "role": "DD"}, http.StatusNotFound)
	f.request(http.MethodPost, "/loot", map[string]any{"item_name": "item", "awarded_to_player_id": f.first, "awarded_to_character_id": f.secondChar}, http.StatusBadRequest)
	f.request(http.MethodPost, "/loot", map[string]any{"item_name": "item", "awarded_to_player_id": f.first, "awarded_to_character_id": 99999}, http.StatusNotFound)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", map[string]any{"item_name": "item", "awarded_to_player_id": f.first}, http.StatusCreated))
	f.request(http.MethodPost, "/complete", nil, http.StatusOK)
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
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"awarded_to_character_id": nil, "reason": "clear character"}, http.StatusOK)
	cleared, err := loot.NewRepo(f.db).Get(context.Background(), awardID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.AwardedToCharacterID != nil {
		t.Fatal("explicit null did not clear character")
	}
	f.request(http.MethodPatch, "/attendance/99999", map[string]any{"reason": "missing"}, http.StatusNotFound)
	f.request(http.MethodPatch, "/loot/99999", map[string]any{"reason": "missing"}, http.StatusNotFound)
	for _, state := range []validation.RaidStatus{validation.RaidStatusCancelled, validation.RaidStatusCompleted} {
		rd, err := raid.NewRepo(f.db).Get(context.Background(), f.raidID)
		if err != nil {
			t.Fatal(err)
		}
		rd.Status = state
		if state == validation.RaidStatusCompleted {
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

func TestCorrectionsAuditFailureRollsBackRecordAndLedgers(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", map[string]any{"item_name": "Weiheabzeichen des Eroberers", "awarded_to_player_id": f.first, "is_main_spec": true}, http.StatusCreated))
	f.request(http.MethodPost, "/complete", nil, http.StatusOK)
	if _, err := f.db.ExecContext(context.Background(), "CREATE TRIGGER fail_correction_audit BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"item_name": "ordinary item", "is_main_spec": false, "reason": "rollback"}, http.StatusInternalServerError)
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

func TestOrdinaryLootLockResolutionAndCorrectionDoesNotRevive(t *testing.T) {
	f := newCorrectionFixture(t)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemID: "123", ItemName: "locked item", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	awardID := correctionID(t, f.request(http.MethodPost, "/loot", map[string]any{"item_id": "123", "item_name": "renamed item", "awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar}, http.StatusCreated))
	award, err := loot.NewRepo(f.db).Get(context.Background(), awardID)
	if err != nil {
		t.Fatal(err)
	}
	if award.AwardedToPlayerID != f.first || !award.IsMainSpec || award.ItemLockID == nil || *award.ItemLockID != lock.ID || award.AwardedToCharacterID != nil {
		t.Fatalf("lock not applied: %+v", award)
	}
	f.request(http.MethodPost, "/complete", nil, http.StatusOK)
	f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", awardID), map[string]any{"awarded_to_player_id": f.second, "reason": "explicit recipient correction"}, http.StatusOK)
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
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemID: "123", ItemName: " locked item ", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, "/loot", map[string]any{"item_id": "different-id", "item_name": "locked item", "item_lock_id": lock.ID, "awarded_to_player_id": f.second}, http.StatusBadRequest)
	f.request(http.MethodPost, "/loot", map[string]any{"item_name": "unlocked", "item_lock_id": 99999, "awarded_to_player_id": f.second}, http.StatusNotFound)
	duplicate := *lock
	duplicate.ID = 0
	duplicate.PlayerID = f.second
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), &duplicate); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, "/loot", map[string]any{"item_name": "locked item", "awarded_to_player_id": f.second}, http.StatusConflict)
	locks, err := enhancepoints.NewRepo(f.db).ListActiveLocksByRaid(context.Background(), f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 2 {
		t.Fatal("ambiguous match consumed a lock")
	}
}

func TestOrdinaryLootTokenFailureRollsBackLockAndAward(t *testing.T) {
	f := newCorrectionFixture(t)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemName: " Weiheabzeichen des Eroberers ", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(context.Background(), "CREATE TRIGGER fail_loot_token BEFORE INSERT ON token_transactions BEGIN SELECT RAISE(ABORT, 'token ledger unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"item_name": "Weiheabzeichen des Eroberers", "item_lock_id": lock.ID, "awarded_to_player_id": f.second}
	f.request(http.MethodPost, "/loot", body, http.StatusInternalServerError)
	stored, err := enhancepoints.NewRepo(f.db).GetLock(context.Background(), lock.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.LockStatusActive || stored.UsedAt != nil {
		t.Fatal("failed token derivation consumed lock")
	}
	awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	if len(awards) != 0 {
		t.Fatal("failed token derivation persisted award")
	}
	if _, err := f.db.ExecContext(context.Background(), "DROP TRIGGER fail_loot_token"); err != nil {
		t.Fatal(err)
	}
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
			consumed := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemID: "123", ItemName: "Weiheabzeichen des Eroberers", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
			if err := enhancepoints.NewRepo(f.db).CreateLock(ctx, consumed); err != nil {
				t.Fatal(err)
			}
			awardID := correctionID(t, f.request(http.MethodPost, "/loot", map[string]any{"item_id": "123", "item_name": consumed.ItemName, "awarded_to_player_id": f.second}, http.StatusCreated))
			active := &enhancepoints.ItemLock{PlayerID: f.second, RaidID: f.raidID, ItemID: "456", ItemName: "replacement item", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.second, CreatedAt: now, UpdatedAt: now}
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
				for _, reason := range []string{"", " \t "} {
					f.request(http.MethodPatch, suffix, map[string]any{"reason": reason}, http.StatusBadRequest)
				}
			}
			// The minimum raid-operations role can perform audited corrections.
			f.actor = seedAccount(t, f.db, "correction-assistent", "password", validation.RoleRaidassistent)
			f.token = login(t, f.srv.Client(), f.srv, "correction-assistent", "password")
			f.request(http.MethodPatch, attendanceSuffix, map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "HEAL", "is_bench": true, "reason": "correct participant"}, http.StatusOK)
			f.request(http.MethodPatch, lootSuffix, map[string]any{"awarded_to_player_id": f.second, "reason": "correct recipient"}, http.StatusOK)
			f.tokenBalance(f.first, 0)
			f.tokenBalance(f.second, 1)
			f.request(http.MethodPatch, lootSuffix, map[string]any{"item_id": "456", "item_name": "replacement item", "awarded_to_player_id": f.first, "is_main_spec": false, "reason": "correct item"}, http.StatusOK)
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
			f.request(http.MethodPatch, lootSuffix, map[string]any{"item_id": "123", "item_name": consumed.ItemName, "reason": "restore token item"}, http.StatusOK)
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
			}{{"attendance", attendanceID, 1}, {"loot_award", awardID, 3}} {
				logs, err := audit.NewRepo(f.db).ListByEntity(ctx, expected.entity, fmt.Sprint(expected.id))
				if err != nil {
					t.Fatal(err)
				}
				if len(logs) != expected.count {
					t.Fatalf("%s audit count: got %d want %d", expected.entity, len(logs), expected.count)
				}
				for _, log := range logs {
					if log.ActorID == nil || *log.ActorID != f.actor || log.Reason == "" || log.BeforeJSON == log.AfterJSON || !json.Valid([]byte(log.BeforeJSON)) || !json.Valid([]byte(log.AfterJSON)) {
						t.Fatalf("invalid active correction audit: %+v", log)
					}
				}
			}
			if _, err := f.db.ExecContext(ctx, "CREATE TRIGGER fail_active_audit BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END"); err != nil {
				t.Fatal(err)
			}
			f.request(http.MethodPatch, lootSuffix, map[string]any{"awarded_to_player_id": f.second, "reason": "rollback recipient"}, http.StatusInternalServerError)
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
