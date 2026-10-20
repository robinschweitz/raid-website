package httpapi_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/validation"
)

func (f *correctionFixture) seedPoints(playerID int64, amount int) {
	f.t.Helper()
	if err := enhancepoints.NewRepo(f.db).InsertTransaction(context.Background(), &enhancepoints.Transaction{PlayerID: playerID, Amount: amount, Type: "legacy_open", Reason: "original history", ActorID: f.actor, CreatedAt: time.Now().UTC()}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *correctionFixture) lootBody(method string) map[string]any {
	return f.selection(map[string]any{"award_method": method, "awarded_to_player_id": f.first, "awarded_to_character_id": f.firstChar, "request_id": "winning-award"}, true)
}

func (f *correctionFixture) award(id int64) *loot.Award {
	f.t.Helper()
	award, err := loot.NewRepo(f.db).Get(context.Background(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return award
}

func TestEnhancedLootBalanceAndRetry(t *testing.T) {
	for _, balance := range []int{0, 30, 70, 150} {
		t.Run(fmt.Sprint(balance), func(t *testing.T) {
			f := newCorrectionFixture(t)
			f.attend(f.first, f.firstChar)
			f.seedPoints(f.first, balance)
			body := f.lootBody("ENHANCED")
			if balance == 0 {
				result := f.request(http.MethodPost, "/loot", body, http.StatusUnprocessableEntity)
				if !bytes.Contains(result, []byte(`"code":"insufficient_balance"`)) {
					t.Fatalf("wrong code: %s", result)
				}
				awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
				if err != nil || len(awards) != 0 {
					t.Fatalf("zero balance persisted loot: %v %v", awards, err)
				}
				f.tokenBalance(f.first, 0)
				return
			}
			id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
			cost := min(balance, 50)
			award := f.award(id)
			if award.AwardMethod != loot.Enhanced || award.EnhancedCost != cost || !award.IsMainSpec {
				t.Fatalf("award: %+v", award)
			}
			for i := 0; i < 3; i++ {
				if retry := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated)); retry != id {
					t.Fatalf("duplicate award %d", retry)
				}
			}
			f.points(f.first, balance-cost)
			f.tokenBalance(f.first, 1)
			awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
			if err != nil || len(awards) != 1 || awards[0].ID != id {
				t.Fatalf("retry persisted duplicate awards: %v %v", awards, err)
			}
			entries, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
			if err != nil || len(entries) != 2 {
				t.Fatalf("entries: %v %v", entries, err)
			}
			entry := entries[1]
			if entry.Amount != -cost || entry.Type != "enhanced_loot" || entry.RefType != "loot_award" || entry.RefID != fmt.Sprint(id) || entry.LootAwardID == nil || *entry.LootAwardID != id || entry.ActorID != f.actor {
				t.Fatalf("debit: %+v", entry)
			}
			logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "loot_award", fmt.Sprint(id))
			if err != nil || len(logs) != 1 {
				t.Fatalf("duplicate initial audit: %v %v", logs, err)
			}
			f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"notes": "corrected", "reason": "clarify"}, http.StatusOK)
			if retry := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated)); retry != id {
				t.Fatal("original request lost after correction")
			}
			body["notes"] = "corrected"
			result := f.request(http.MethodPost, "/loot", body, http.StatusConflict)
			if !bytes.Contains(result, []byte(`"code":"request_conflict"`)) {
				t.Fatalf("wrong conflict: %s", result)
			}
			f.points(f.first, balance-cost)
		})
	}
}

func TestNewEnhancedLootRequiresRequestIDWithoutSideEffects(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprintf("completed_%t", completed), func(t *testing.T) {
			f := newCorrectionFixture(t)
			f.attend(f.first, f.firstChar)
			f.seedPoints(f.first, 70)
			if completed {
				f.legacyCompleted()
			}
			for _, key := range []string{"missing", "null", "empty", "whitespace"} {
				t.Run(key, func(t *testing.T) {
					fixture := *f
					fixture.t = t
					f := &fixture
					body := f.lootBody("ENHANCED")
					body["reason"] = "record historical winner"
					switch key {
					case "missing":
						delete(body, "request_id")
					case "null":
						body["request_id"] = nil
					case "empty":
						body["request_id"] = ""
					case "whitespace":
						body["request_id"] = " \t "
					}
					for i := 0; i < 2; i++ {
						result := f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
						if !bytes.Contains(result, []byte(`"code":"request_id_required"`)) {
							t.Fatalf("request key error: %s", result)
						}
					}
					f.points(f.first, 70)
					awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
					if err != nil || len(awards) != 0 {
						t.Fatalf("award escaped rejection: %v %v", awards, err)
					}
					entries, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
					if err != nil || len(entries) != 1 {
						t.Fatalf("ledger escaped rejection: %v %v", entries, err)
					}
					var count int
					if err := f.db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = 'loot_award'").Scan(context.Background(), &count); err != nil || count != 0 {
						t.Fatalf("audit escaped rejection: %d %v", count, err)
					}
				})
			}
		})
	}
}

func TestDomainNewEnhancedAwardsRequireRequestID(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprintf("completed_%t", completed), func(t *testing.T) {
			f := newCorrectionFixture(t)
			f.attend(f.first, f.firstChar)
			f.seedPoints(f.first, 70)
			if completed {
				f.legacyCompleted()
			}
			empty, whitespace := "", " \t "
			for _, key := range []*string{nil, &empty, &whitespace} {
				for _, directLoot := range []bool{false, true} {
					if completed && directLoot {
						continue
					}
					award := &loot.Award{RaidID: f.raidID, CatalogBossID: &f.tokenBoss, ItemID: f.tokenItem, AwardMethod: loot.Enhanced, AwardedToPlayerID: f.first, AwardedToCharacterID: &f.firstChar, AwardedByPlayerID: f.actor, RequestID: key}
					var err error
					if directLoot {
						err = loot.NewService(loot.NewRepo(f.db)).Award(context.Background(), award)
					} else {
						err = raidcorrection.NewService(raidcorrection.NewRepo(f.db)).SaveLoot(context.Background(), award, f.actor, "record winner")
					}
					if !errors.Is(err, loot.ErrRequestRequired) {
						t.Fatalf("direct loot %t: expected request required, got %v", directLoot, err)
					}
				}
			}
			f.points(f.first, 70)
			awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
			if err != nil || len(awards) != 0 {
				t.Fatalf("domain rejection persisted loot: %v %v", awards, err)
			}
			entries, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
			if err != nil || len(entries) != 1 {
				t.Fatalf("domain rejection persisted debit: %v %v", entries, err)
			}
			f.tokenBalance(f.first, 0)
		})
	}
}

func TestEnteringEnhancedRequiresHistoricalRecipientInRoster(t *testing.T) {
	for _, nullable := range []bool{true, false} {
		t.Run(fmt.Sprintf("nullable_%t", nullable), func(t *testing.T) {
			f := newCorrectionFixture(t)
			f.seedPoints(f.first, 70)
			f.legacyCompleted()
			now := time.Now().UTC()
			old := &loot.Award{RaidID: f.raidID, AwardedToPlayerID: f.first, AwardedByPlayerID: f.actor, Source: "legacy_import", ItemName: "Weiheabzeichen des Eroberers", AwardMethod: loot.Second, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
			if !nullable {
				old.AwardedToCharacterID = &f.firstChar
			}
			if err := loot.NewRepo(f.db).CreateAward(context.Background(), old); err != nil {
				t.Fatal(err)
			}
			suffix := fmt.Sprintf("/loot/%d", old.ID)
			f.request(http.MethodPatch, suffix, map[string]any{"notes": "harmless historical note", "reason": "clarify"}, http.StatusOK)
			before := f.award(old.ID)
			pointsBefore, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
			if err != nil {
				t.Fatal(err)
			}
			logsBefore, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "loot_award", fmt.Sprint(old.ID))
			if err != nil {
				t.Fatal(err)
			}
			result := f.request(http.MethodPatch, suffix, f.selection(map[string]any{"award_method": "ENHANCED", "reason": "outside winner"}, true), http.StatusBadRequest)
			if !bytes.Contains(result, []byte(`"code":"recipient_not_in_roster"`)) {
				t.Fatalf("roster error: %s", result)
			}
			pointsAfter, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
			if err != nil {
				t.Fatal(err)
			}
			logsAfter, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "loot_award", fmt.Sprint(old.ID))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, f.award(old.ID)) || !reflect.DeepEqual(pointsBefore, pointsAfter) || !reflect.DeepEqual(logsBefore, logsAfter) {
				t.Fatal("failed enhanced transition changed history")
			}
			f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.first, "character_id": f.firstChar, "role": "DD", "reason": "actual historical participant"}, http.StatusCreated)
			f.request(http.MethodPatch, suffix, f.selection(map[string]any{"award_method": "ENHANCED", "awarded_to_character_id": f.firstChar, "reason": "actual roster winner"}, true), http.StatusOK)
			cost := f.award(old.ID).EnhancedCost
			balance, err := enhancepoints.NewRepo(f.db).Balance(context.Background(), f.first)
			if err != nil || cost != 50 {
				t.Fatalf("valid winner cost: %d %v", cost, err)
			}
			f.request(http.MethodPatch, suffix, map[string]any{"award_method": "ENHANCED", "notes": "same winner", "reason": "clarify"}, http.StatusOK)
			f.points(f.first, balance)
			if f.award(old.ID).EnhancedCost != cost {
				t.Fatal("unchanged enhanced winner recalculated cost")
			}
			f.tokenBalance(f.first, 1)
		})
	}
}

func TestLockAutoAwardWithoutEnhancedRequestID(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemName: "Weiheabzeichen des Eroberers", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	body := f.lootBody("ENHANCED")
	delete(body, "request_id")
	id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	if a := f.award(id); a.AwardMethod != loot.Main || a.EnhancedCost != 0 || a.RequestID != nil {
		t.Fatalf("lock award: %+v", a)
	}
	f.points(f.first, 0)
	stored, err := enhancepoints.NewRepo(f.db).GetLock(context.Background(), lock.ID)
	if err != nil || stored.Status != validation.LockStatusUsed {
		t.Fatalf("lock state: %v %v", stored, err)
	}
}

func TestEnhancedLootCoupledCorrections(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.attend(f.second, f.secondChar)
	f.seedPoints(f.first, 70)
	f.seedPoints(f.second, 30)
	id := correctionID(t, f.request(http.MethodPost, "/loot", f.lootBody("ENHANCED"), http.StatusCreated))
	suffix := fmt.Sprintf("/loot/%d", id)
	f.request(http.MethodPatch, suffix, f.selection(map[string]any{"notes": "changed metadata", "reason": "clarify"}, false), http.StatusOK)
	if f.award(id).EnhancedCost != 50 {
		t.Fatal("metadata edit recalculated cost")
	}
	f.points(f.first, 20)
	f.tokenBalance(f.first, 0)
	f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "winner correction"}, http.StatusOK)
	f.points(f.first, 70)
	f.points(f.second, 0)
	if f.award(id).EnhancedCost != 30 {
		t.Fatal("new winner cost not recomputed")
	}
	for i := 0; i < 2; i++ {
		f.request(http.MethodPatch, suffix, map[string]any{"award_method": "BOE", "reason": "not enhanced"}, http.StatusOK)
		f.points(f.second, 30)
		if a := f.award(id); a.EnhancedCost != 0 || a.IsMainSpec {
			t.Fatalf("BOE: %+v", a)
		}
		f.request(http.MethodPatch, suffix, map[string]any{"award_method": "ENHANCED", "reason": "restore enhanced"}, http.StatusOK)
		f.points(f.second, 0)
	}
	// A second roster character of the same player does not create a second cost.
	now := time.Now().UTC()
	alt := &character.Character{PlayerID: f.second, Name: "Alt", Class: "Mage", MainSpec: "Fire", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(f.db).Create(context.Background(), alt); err != nil {
		t.Fatal(err)
	}
	f.legacyDuplicateSlot(f.second, alt.ID)
	f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_character_id": alt.ID, "reason": "actual character"}, http.StatusOK)
	f.points(f.second, 0)
	if f.award(id).EnhancedCost != 30 {
		t.Fatal("same player charged again")
	}
}

func TestEnhancedLootFailedRecipientAndAtomicRollback(t *testing.T) {
	for _, failure := range []string{"insufficient", "audit", "ledger"} {
		t.Run(failure, func(t *testing.T) {
			f := newCorrectionFixture(t)
			f.attend(f.first, f.firstChar)
			f.attend(f.second, f.secondChar)
			f.seedPoints(f.first, 70)
			id := correctionID(t, f.request(http.MethodPost, "/loot", f.lootBody("ENHANCED"), http.StatusCreated))
			want := http.StatusUnprocessableEntity
			if failure != "insufficient" {
				f.seedPoints(f.second, 30)
				table := map[string]string{"audit": "audit_log", "ledger": "enhance_point_transactions"}[failure]
				if _, err := f.db.ExecContext(context.Background(), "CREATE TRIGGER fail_enhanced BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
					t.Fatal(err)
				}
				want = http.StatusInternalServerError
			}
			f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "rollback"}, want)
			f.points(f.first, 20)
			secondBalance := 0
			if failure != "insufficient" {
				secondBalance = 30
			}
			f.points(f.second, secondBalance)
			f.tokenBalance(f.first, 1)
			f.tokenBalance(f.second, 0)
			if a := f.award(id); a.AwardedToPlayerID != f.first || a.EnhancedCost != 50 {
				t.Fatalf("escaped rollback: %+v", a)
			}
			entries, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
			if err != nil || len(entries) != 2 {
				t.Fatalf("refund escaped rollback: %v %v", entries, err)
			}
		})
	}
}

func TestLootMethodsRosterAndPermissions(t *testing.T) {
	f := newCorrectionFixture(t)
	body := f.lootBody("MAIN")
	result := f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
	if !bytes.Contains(result, []byte(`"code":"recipient_not_in_roster"`)) {
		t.Fatalf("roster code: %s", result)
	}
	f.attend(f.first, f.firstChar)
	for _, method := range []string{"MAIN", "SECOND", "BOE"} {
		body = f.lootBody(method)
		delete(body, "request_id")
		id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
		if a := f.award(id); a.IsMainSpec != (method == "MAIN") || a.EnhancedCost != 0 {
			t.Fatalf("method %s: %+v", method, a)
		}
	}
	body = f.lootBody("INVALID")
	result = f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
	if !bytes.Contains(result, []byte(`"code":"invalid_award_method"`)) {
		t.Fatalf("method code: %s", result)
	}
	for _, field := range []string{"award_method", "awarded_to_character_id"} {
		body = f.lootBody("MAIN")
		delete(body, field)
		f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
	}
	for _, forbiddenField := range []string{"amount", "enhanced_cost", "is_main_spec"} {
		body = f.lootBody("ENHANCED")
		body[forbiddenField] = 1
		f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
	}
	body = f.lootBody("ENHANCED")
	status, _ := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/loot", f.srv.URL, f.raidID), f.memberToken, body)
	if status != http.StatusForbidden {
		t.Fatalf("member award: %d", status)
	}
	f.actor = seedAccount(t, f.db, "award-assistent", "password", validation.RoleRaidassistent)
	f.token = login(t, f.srv.Client(), f.srv, "award-assistent", "password")
	f.seedPoints(f.first, 30)
	f.request(http.MethodPost, "/loot", body, http.StatusCreated)
	// Existing player identity does not authorize a character absent from this raid.
	f.request(http.MethodPatch, "/loot/1", map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "not present"}, http.StatusBadRequest)
}

func TestEnhancedCompletedCorrectionEligibilityDelta(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.seedPoints(f.first, 70)
	id := correctionID(t, f.request(http.MethodPost, "/loot", f.lootBody("SECOND"), http.StatusCreated))
	f.legacyCompleted()
	f.archiveHistory()
	suffix := fmt.Sprintf("/loot/%d", id)
	f.request(http.MethodPatch, suffix, map[string]any{"award_method": "ENHANCED", "reason": "enhanced winner"}, http.StatusOK)
	f.points(f.first, 10) // 70 - 50 cost - 10 eligibility delta, no reconstructed reward.
	f.request(http.MethodPatch, suffix, map[string]any{"award_method": "SECOND", "reason": "undo"}, http.StatusOK)
	f.points(f.first, 70)
	f.tokenBalance(f.first, 1)
	entries, err := enhancepoints.NewRepo(f.db).ListByPlayer(context.Background(), f.first)
	if err != nil || len(entries) != 5 || entries[0].Type != "legacy_open" || entries[0].Amount != 70 {
		t.Fatalf("history: %v %v", entries, err)
	}
}

func TestEnhancedNewAwardRollbackAndRetry(t *testing.T) {
	for _, table := range []string{"audit_log", "enhance_point_transactions"} {
		t.Run(table, func(t *testing.T) {
			f := newCorrectionFixture(t)
			f.attend(f.first, f.firstChar)
			f.seedPoints(f.first, 70)
			if _, err := f.db.ExecContext(context.Background(), "CREATE TRIGGER fail_new_enhanced BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
				t.Fatal(err)
			}
			body := f.lootBody("ENHANCED")
			f.request(http.MethodPost, "/loot", body, http.StatusInternalServerError)
			f.points(f.first, 70)
			f.tokenBalance(f.first, 0)
			awards, err := loot.NewRepo(f.db).ListByRaid(context.Background(), f.raidID)
			if err != nil || len(awards) != 0 {
				t.Fatalf("failed creation persisted: %v %v", awards, err)
			}
			if _, err := f.db.ExecContext(context.Background(), "DROP TRIGGER fail_new_enhanced"); err != nil {
				t.Fatal(err)
			}
			id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
			f.request(http.MethodPost, "/loot", body, http.StatusCreated)
			f.points(f.first, 20)
			if f.award(id).EnhancedCost != 50 {
				t.Fatal("failed request retained a cost")
			}
		})
	}
}

func TestHistoricalNullableRecipientPreservedUntilRecipientChange(t *testing.T) {
	f := newCorrectionFixture(t)
	f.legacyCompleted()
	now := time.Now().UTC()
	old := &loot.Award{RaidID: f.raidID, AwardedToPlayerID: f.first, AwardedByPlayerID: f.actor, Source: "legacy_import", ItemName: "historical", AwardMethod: loot.Second, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := loot.NewRepo(f.db).CreateAward(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("/loot/%d", old.ID)
	f.request(http.MethodPatch, suffix, map[string]any{"notes": "historical clarification", "reason": "preserve unknown character"}, http.StatusOK)
	if f.award(old.ID).AwardedToCharacterID != nil {
		t.Fatal("fabricated historical recipient")
	}
	f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_player_id": f.second, "reason": "recipient correction"}, http.StatusBadRequest)
	f.request(http.MethodPatch, suffix, map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "not in roster"}, http.StatusBadRequest)
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.second, "character_id": f.secondChar, "role": "DD", "reason": "missing participant"}, http.StatusCreated)
	f.request(http.MethodPatch, suffix, f.selection(map[string]any{"awarded_to_player_id": f.second, "awarded_to_character_id": f.secondChar, "reason": "recipient correction"}, false), http.StatusOK)
}

func TestLockAwardsRequireUnambiguousRosterCharacter(t *testing.T) {
	f := newCorrectionFixture(t)
	f.seedPoints(f.first, 150)
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemName: "Weiheabzeichen des Eroberers", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(f.db).CreateLock(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	body := f.lootBody("ENHANCED")
	delete(body, "awarded_to_character_id")
	f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
	f.attend(f.first, f.firstChar)
	alt := &character.Character{PlayerID: f.first, Name: "Lockeralt", Class: "Mage", MainSpec: "Fire", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(f.db).Create(context.Background(), alt); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, "/attendance", map[string]any{"player_id": f.first, "character_id": alt.ID, "role": "DD"}, http.StatusConflict)
	f.legacyDuplicateSlot(f.first, alt.ID)
	f.request(http.MethodPost, "/loot", body, http.StatusConflict)
	body["awarded_to_character_id"] = f.secondChar
	f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
	body["awarded_to_character_id"] = f.firstChar
	id := correctionID(t, f.request(http.MethodPost, "/loot", body, http.StatusCreated))
	if a := f.award(id); a.AwardMethod != loot.Main || a.EnhancedCost != 0 || !a.IsMainSpec {
		t.Fatalf("lock charged: %+v", a)
	}
	f.points(f.first, 150)
	f.request(http.MethodPost, "/loot", body, http.StatusCreated)
	f.points(f.first, 150)
	stored, err := enhancepoints.NewRepo(f.db).GetLock(context.Background(), lock.ID)
	if err != nil || stored.Status != validation.LockStatusUsed {
		t.Fatalf("lock: %v %v", stored, err)
	}
}

func TestEnhancedLootCancelledRejectsAndArchiveRequiresReason(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(fmt.Sprint(archived), func(t *testing.T) {
			f := newCorrectionFixture(t)
			f.attend(f.first, f.firstChar)
			f.seedPoints(f.first, 70)
			id := correctionID(t, f.request(http.MethodPost, "/loot", f.lootBody("ENHANCED"), http.StatusCreated))
			rd, err := raid.NewRepo(f.db).Get(context.Background(), f.raidID)
			if err != nil {
				t.Fatal(err)
			}
			rd.Status = validation.RaidStatusCancelled
			if archived {
				now := time.Now().UTC()
				rd.Status = validation.RaidStatusCompleted
				rd.ArchivedAt = &now
			}
			if err := raid.NewRepo(f.db).Update(context.Background(), rd); err != nil {
				t.Fatal(err)
			}
			if archived {
				f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"award_method": "SECOND", "reason": " "}, http.StatusBadRequest)
				f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"notes": "clarification", "reason": "archived note"}, http.StatusOK)
				f.points(f.first, 20)
				f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"award_method": "SECOND", "reason": "refund"}, http.StatusOK)
				f.points(f.first, 80)
			} else {
				f.request(http.MethodPatch, fmt.Sprintf("/loot/%d", id), map[string]any{"award_method": "SECOND", "reason": "blocked refund"}, http.StatusConflict)
			}
			body := f.lootBody("ENHANCED")
			body["request_id"] = "another-award"
			if archived {
				f.request(http.MethodPost, "/loot", body, http.StatusBadRequest)
				body["reason"] = "missing enhanced award"
				f.request(http.MethodPost, "/loot", body, http.StatusCreated)
				f.points(f.first, 20)
			} else {
				f.request(http.MethodPost, "/loot", body, http.StatusConflict)
				f.points(f.first, 20)
			}
		})
	}
}
