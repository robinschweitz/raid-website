package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/auth"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestAdminAccountMergeTransfersOwnershipAndRevokesSourceAuth(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	adminID := seedAccount(t, bdb, "admin-merge", "admin-merge-password", validation.RoleAdmin)
	targetID := seedAccount(t, bdb, "target-merge", "target-merge-password", validation.RoleMember)
	sourceID := seedAccount(t, bdb, "source-merge", "source-merge-password", validation.RoleRaidleader)
	adminToken := login(t, client, srv, "admin-merge", "admin-merge-password")
	sourceToken := login(t, client, srv, "source-merge", "source-merge-password")
	now := time.Now().UTC()
	raidID := mergeTestRaid(t, bdb, adminID, now)

	char := &character.Character{PlayerID: sourceID, Name: "Sourcechar", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(bdb).Create(ctx, char); err != nil {
		t.Fatalf("create source character: %v", err)
	}
	lock := &enhancepoints.ItemLock{PlayerID: sourceID, RaidID: raidID, ItemID: "123", ItemName: "Merge item", Status: validation.LockStatusActive, LockedAt: now, ActorID: sourceID, CreatedAt: now, UpdatedAt: now}
	if err := enhancepoints.NewRepo(bdb).CreateLock(ctx, lock); err != nil {
		t.Fatalf("create source lock: %v", err)
	}
	for _, row := range []*enhancepoints.Transaction{
		{PlayerID: sourceID, Amount: 35, Type: "ADJUSTMENT", Reason: "seed", ActorID: adminID, CreatedAt: now},
		{PlayerID: targetID, Amount: 9, Type: "ADJUSTMENT", Reason: "seed", ActorID: adminID, CreatedAt: now},
	} {
		if err := enhancepoints.NewRepo(bdb).InsertTransaction(ctx, row); err != nil {
			t.Fatalf("seed points: %v", err)
		}
	}
	authRepo := auth.NewRepo(bdb)
	if err := authRepo.UpsertMFASettings(ctx, &auth.MFASettings{PlayerID: targetID, TOTPSecret: "target-secret", TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed target mfa: %v", err)
	}
	if err := authRepo.UpsertMFASettings(ctx, &auth.MFASettings{PlayerID: sourceID, TOTPSecret: "source-secret", TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed source mfa: %v", err)
	}
	if err := authRepo.CreateRecoveryCode(ctx, &auth.MFARecoveryCode{PlayerID: sourceID, CodeHash: "source-recovery", CreatedAt: now}); err != nil {
		t.Fatalf("seed recovery code: %v", err)
	}

	status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/merge", srv.URL, targetID), adminToken, map[string]any{"source_player_id": sourceID, "reason": "Doppeltes Konto"})
	if status != http.StatusNoContent {
		t.Fatalf("merge status %d: %s", status, body)
	}

	if _, err := player.NewRepo(bdb).Get(ctx, sourceID); err == nil {
		t.Fatal("source player was not deleted")
	}
	target, err := player.NewRepo(bdb).Get(ctx, targetID)
	if err != nil || target.Role != validation.RoleMember {
		t.Fatalf("target credentials or role changed: %+v, %v", target, err)
	}
	if char, err := character.NewRepo(bdb).Get(ctx, char.ID); err != nil || char.PlayerID != targetID {
		t.Fatalf("character owner = %+v, %v", char, err)
	}
	if lock, err := enhancepoints.NewRepo(bdb).GetLock(ctx, lock.ID); err != nil || lock.PlayerID != targetID {
		t.Fatalf("lock owner = %+v, %v", lock, err)
	}
	points := enhancepoints.NewRepo(bdb)
	if balance, err := points.Balance(ctx, targetID); err != nil || balance != 44 {
		t.Fatalf("target balance = %d, %v", balance, err)
	}
	for _, table := range []string{"sessions", "player_mfa_settings", "mfa_recovery_codes"} {
		var count int
		if err := bdb.NewSelect().TableExpr(table).ColumnExpr("COUNT(*)").Where("player_id = ?", sourceID).Scan(ctx, &count); err != nil || count != 0 {
			t.Fatalf("%s source rows = %d, %v", table, count, err)
		}
	}
	settings, err := authRepo.GetMFASettings(ctx, targetID)
	if err != nil || settings.TOTPSecret != "target-secret" {
		t.Fatalf("target mfa changed: %+v, %v", settings, err)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", sourceToken, nil); status != http.StatusUnauthorized {
		t.Fatalf("revoked source session status = %d", status)
	}
	var audits int
	if err := bdb.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("actor_id = ?", adminID).Where("reason = ?", "Doppeltes Konto").Scan(ctx, &audits); err != nil || audits < 5 {
		t.Fatalf("merge audits = %d, %v", audits, err)
	}
}

func TestAccountMergeRejectsLockConflictAndNonAdmin(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	adminID := seedAccount(t, bdb, "admin-conflict", "admin-conflict-password", validation.RoleAdmin)
	targetID := seedAccount(t, bdb, "target-conflict", "target-conflict-password", validation.RoleMember)
	sourceID := seedAccount(t, bdb, "source-conflict", "source-conflict-password", validation.RoleMember)
	adminToken := login(t, client, srv, "admin-conflict", "admin-conflict-password")
	sourceToken := login(t, client, srv, "source-conflict", "source-conflict-password")
	now := time.Now().UTC()
	raidID := mergeTestRaid(t, bdb, adminID, now)
	for _, playerID := range []int64{targetID, sourceID} {
		lock := &enhancepoints.ItemLock{PlayerID: playerID, RaidID: raidID, ItemID: "same", ItemName: "Same", Status: validation.LockStatusActive, LockedAt: now, ActorID: adminID, CreatedAt: now, UpdatedAt: now}
		if err := enhancepoints.NewRepo(bdb).CreateLock(ctx, lock); err != nil {
			t.Fatalf("create lock: %v", err)
		}
	}
	url := fmt.Sprintf("%s/api/v1/players/%d/merge", srv.URL, targetID)
	if status, body := do(t, client, http.MethodPost, url, sourceToken, map[string]any{"source_player_id": sourceID, "reason": "nope"}); status != http.StatusForbidden {
		t.Fatalf("non-admin merge status %d: %s", status, body)
	}
	if status, body := do(t, client, http.MethodPost, url, adminToken, map[string]any{"source_player_id": sourceID, "reason": "Duplikat"}); status != http.StatusConflict {
		t.Fatalf("conflict merge status %d: %s", status, body)
	}
	if source, err := player.NewRepo(bdb).Get(ctx, sourceID); err != nil || !source.Active {
		t.Fatalf("conflicting source mutated: %+v, %v", source, err)
	}
}

func mergeTestRaid(t *testing.T, bdb *bun.DB, leaderID int64, now time.Time) int64 {
	t.Helper()
	rd := &raid.Raid{Name: "Merge test", RaidLeaderID: leaderID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(bdb).Create(context.Background(), rd); err != nil {
		t.Fatalf("create merge test raid: %v", err)
	}
	return rd.ID
}
