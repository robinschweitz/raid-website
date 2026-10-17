package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestNewLootRequiresSourceSelectionWithoutSideEffects(t *testing.T) {
	for _, completed := range []bool{false, true} {
		for _, setup := range []string{"unassociated", "missing_size", "name_only", "invalid_item"} {
			t.Run(fmt.Sprintf("%t_%s", completed, setup), func(t *testing.T) {
				f := newCorrectionFixture(t)
				f.attend(f.first, f.firstChar)
				f.seedPoints(f.first, 150)
				if completed {
					f.legacyCompleted()
				}
				ctx := context.Background()
				rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
				if err != nil {
					t.Fatal(err)
				}
				want, code := http.StatusBadRequest, "catalog_item_invalid"
				if setup == "unassociated" || setup == "missing_size" {
					rd.RaidSize = nil
					if setup == "unassociated" {
						rd.CatalogInstanceID = nil
					}
					if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
						t.Fatal(err)
					}
					want, code = http.StatusConflict, "catalog_context_required"
				}
				now := time.Now().UTC().Truncate(time.Microsecond)
				lock := &enhancepoints.ItemLock{PlayerID: f.first, RaidID: f.raidID, ItemID: f.tokenItem, ItemName: "Weiheabzeichen des Eroberers", Status: validation.LockStatusActive, LockedAt: now, ActorID: f.first, CreatedAt: now, UpdatedAt: now}
				if err := enhancepoints.NewRepo(f.db).CreateLock(ctx, lock); err != nil {
					t.Fatal(err)
				}
				for _, method := range []string{"MAIN", "SECOND", "BOE", "ENHANCED"} {
					body := f.lootBody(method)
					body["reason"] = "record missing winner"
					body["boss"], body["item_name"] = "made up boss", lock.ItemName
					if setup == "name_only" {
						delete(body, "catalog_boss_id")
						delete(body, "item_id")
					}
					if setup == "invalid_item" {
						body["item_id"] = "99999999"
					}
					result := f.request(http.MethodPost, "/loot", body, want)
					if !bytes.Contains(result, []byte(`"code":"`+code+`"`)) {
						t.Fatalf("wrong error: %s", result)
					}
				}
				f.points(f.first, 150)
				f.tokenBalance(f.first, 0)
				awards, err := loot.NewRepo(f.db).ListByRaid(ctx, f.raidID)
				if err != nil || len(awards) != 0 {
					t.Fatalf("awards escaped rejection: %v %v", awards, err)
				}
				stored, err := enhancepoints.NewRepo(f.db).GetLock(ctx, lock.ID)
				unchanged := err == nil && stored.ID == lock.ID && stored.PlayerID == lock.PlayerID && stored.RaidID == lock.RaidID &&
					stored.ItemID == lock.ItemID && stored.ItemName == lock.ItemName && stored.Status == lock.Status && stored.Notes == lock.Notes &&
					stored.UsedAt == nil && stored.ExpiredAt == nil && stored.CancelledAt == nil &&
					stored.LockedAt.Equal(lock.LockedAt) && stored.CreatedAt.Equal(lock.CreatedAt) && stored.UpdatedAt.Equal(lock.UpdatedAt)
				if !unchanged {
					t.Fatalf("lock changed: %v %v", stored, err)
				}
				var count int
				if err := f.db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = 'loot_award'").Scan(ctx, &count); err != nil || count != 0 {
					t.Fatalf("audit escaped rejection: %d %v", count, err)
				}
				entries, err := enhancepoints.NewRepo(f.db).ListByPlayer(ctx, f.first)
				if err != nil || len(entries) != 1 {
					t.Fatalf("EP history changed: %v %v", entries, err)
				}
			})
		}
	}
}

func TestLegacyUnassociatedLootNotesAndEnhancedTransition(t *testing.T) {
	f := newCorrectionFixture(t)
	f.attend(f.first, f.firstChar)
	f.seedPoints(f.first, 70)
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.CatalogInstanceID, rd.RaidSize = nil, nil
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	old := &loot.Award{RaidID: f.raidID, Boss: "Historical boss", ItemID: "old external ID", ItemName: "Weiheabzeichen des Eroberers", AwardedToPlayerID: f.first, AwardedToCharacterID: &f.firstChar, AwardedByPlayerID: f.actor, Source: "legacy_import", AwardMethod: loot.Second, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := loot.NewRepo(f.db).CreateAward(ctx, old); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("/loot/%d", old.ID)
	f.request(http.MethodPatch, suffix, map[string]any{"notes": "clarification", "reason": "notes only"}, http.StatusOK)
	stored := f.award(old.ID)
	if stored.Boss != old.Boss || stored.ItemID != old.ItemID || stored.ItemName != old.ItemName || stored.CatalogBossID != nil || stored.Source != old.Source || !stored.AwardedAt.Equal(old.AwardedAt) {
		t.Fatalf("history rewritten: %+v", stored)
	}
	for _, patch := range []map[string]any{
		{"award_method": "ENHANCED", "reason": "new spending"},
		{"awarded_to_player_id": f.second, "reason": "new recipient"},
		{"item_name": "invented item", "reason": "new item"},
		{"boss": "invented boss", "reason": "new boss"},
	} {
		result := f.request(http.MethodPatch, suffix, patch, http.StatusConflict)
		if !bytes.Contains(result, []byte(`"code":"catalog_context_required"`)) {
			t.Fatalf("wrong error: %s", result)
		}
	}
	f.points(f.first, 70)
	if !reflect.DeepEqual(stored, f.award(old.ID)) {
		t.Fatal("rejection changed award")
	}
	f.tokenBalance(f.first, 1)
}

func TestHistoricalEnhancedRefundPreservesUnassociatedIdentity(t *testing.T) {
	f := newCorrectionFixture(t)
	ctx := context.Background()
	rd, err := raid.NewRepo(f.db).Get(ctx, f.raidID)
	if err != nil {
		t.Fatal(err)
	}
	rd.CatalogInstanceID, rd.RaidSize = nil, nil
	if err := raid.NewRepo(f.db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	old := &loot.Award{RaidID: f.raidID, Boss: "Historical boss", ItemName: "Historical item", AwardMethod: loot.Enhanced, EnhancedCost: 30, AwardedToPlayerID: f.first, AwardedByPlayerID: f.actor, Source: "manual", AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := loot.NewRepo(f.db).CreateAward(ctx, old); err != nil {
		t.Fatal(err)
	}
	f.seedPoints(f.first, 40)
	if err := enhancepoints.NewRepo(f.db).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: f.first, RaidID: &f.raidID, LootAwardID: &old.ID, Amount: -30, Type: "enhanced_loot", Reason: "historical winner", ActorID: f.actor, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("/loot/%d", old.ID)
	f.request(http.MethodPatch, suffix, map[string]any{"notes": "clarify", "reason": "notes only"}, http.StatusOK)
	f.points(f.first, 10)
	if f.award(old.ID).EnhancedCost != 30 {
		t.Fatal("notes recalculated cost")
	}
	f.request(http.MethodPatch, suffix, map[string]any{"award_method": "SECOND", "reason": "refund incorrect winner"}, http.StatusOK)
	f.points(f.first, 40)
	stored := f.award(old.ID)
	if stored.Boss != old.Boss || stored.ItemName != old.ItemName || stored.ItemID != old.ItemID || stored.CatalogBossID != nil || stored.AwardedToCharacterID != nil || stored.Source != old.Source || stored.EnhancedCost != 0 {
		t.Fatalf("refund rewrote identity: %+v", stored)
	}
}
