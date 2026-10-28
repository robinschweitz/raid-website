package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"raid-website/internal/player"
)

func TestReqAuth025_AC01(t *testing.T) {
	ctx := context.Background()
	db := reliabilityDB(t)
	repo := NewRepo(db.DB)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for _, age := range []time.Duration{16 * time.Minute, 15 * time.Minute, time.Minute} {
		for _, success := range []bool{false, true} {
			if err := repo.CreateLoginAttempt(ctx, &LoginAttempt{Username: "name", ClientIP: "192.0.2.1", Success: success, CreatedAt: now.Add(-age)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := repo.PruneSecurityRecords(ctx, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.DB.NewSelect().Model((*LoginAttempt)(nil)).ColumnExpr("count(*)").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("remaining = %d, want four recent attempts", count)
	}
}

func TestReqAuth025_AC02(t *testing.T) {
	ctx := context.Background()
	db := reliabilityDB(t)
	players := player.NewRepo(db.DB)
	p := &player.Player{Name: "User", Active: true}
	if err := players.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db.DB)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for i, expiry := range []time.Time{now.Add(-time.Minute), now, now.Add(time.Hour)} {
		key := strings.Repeat("x", i+1)
		if err := repo.CreateSession(ctx, &Session{TokenHash: key, PlayerID: p.ID, CreatedAt: now.Add(-time.Hour), LastSeenAt: now, ExpiresAt: expiry}); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateMFAChallenge(ctx, &MFAChallenge{ChallengeHash: key, PlayerID: p.ID, CreatedAt: now, ExpiresAt: expiry}); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateWebAuthnSession(ctx, &WebAuthnSession{SessionHash: key, PlayerID: p.ID, Purpose: "login", SessionJSON: "{}", CreatedAt: now, ExpiresAt: expiry}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateMFAChallenge(ctx, &MFAChallenge{ChallengeHash: "used", PlayerID: p.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), UsedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWebAuthnSession(ctx, &WebAuthnSession{SessionHash: "used", PlayerID: p.ID, Purpose: "login", SessionJSON: "{}", CreatedAt: now, ExpiresAt: now.Add(time.Hour), UsedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.PruneSecurityRecords(ctx, now); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{(*Session)(nil), (*MFAChallenge)(nil), (*WebAuthnSession)(nil)} {
		count, err := db.DB.NewSelect().Model(model).Count(ctx)
		if err != nil || count != 1 {
			t.Fatalf("remaining %T = %d, %v", model, count, err)
		}
	}
}

func TestReqAuth025_AC03(t *testing.T) {
	db, svc := loginLimitService(t)
	ctx := context.Background()
	if _, err := db.DB.ExecContext(ctx, "CREATE TRIGGER reject_cleanup BEFORE DELETE ON login_attempts BEGIN SELECT RAISE(ABORT, 'cleanup failed'); END"); err != nil {
		t.Fatal(err)
	}
	if err := svc.sessions.CreateLoginAttempt(ctx, &LoginAttempt{Username: "old", ClientIP: "192.0.2.1", CreatedAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.StartLogin(ctx, "owner", "valid-password", "192.0.2.1")
	if err != nil || result.Token == "" {
		t.Fatalf("login = %v, %v", result, err)
	}
}
