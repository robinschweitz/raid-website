package auth

import (
	"context"
	"testing"
	"time"

	"raid-website/internal/player"
)

func TestReqAuth018(t *testing.T) {
	for _, tc := range []struct {
		name           string
		age, idle      time.Duration
		valid, touched bool
	}{
		{"AC01_idle", 8 * 24 * time.Hour, 7*24*time.Hour + time.Minute, false, false},
		{"idle_boundary", 8 * 24 * time.Hour, 7 * 24 * time.Hour, false, false},
		{"AC02_absolute", 30 * 24 * time.Hour, 24 * time.Hour, false, false},
		{"AC03_recent", 24 * time.Hour, 10 * time.Minute, true, false},
		{"AC04_hourly", 24 * time.Hour, 61 * time.Minute, true, true},
		{"active_day29", 29 * 24 * time.Hour, 24 * time.Hour, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := reliabilityDB(t)
			players := player.NewRepo(db.DB)
			p := &player.Player{Name: "Session user", Active: true}
			if err := players.Create(ctx, p); err != nil {
				t.Fatal(err)
			}
			repo := NewRepo(db.DB)
			svc := NewService(players, repo)
			now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			lastSeen := now.Add(-tc.idle)
			sess := &Session{TokenHash: hashToken("token"), PlayerID: p.ID, CreatedAt: now.Add(-tc.age), ExpiresAt: now.Add(-tc.age).Add(defaultSessionTTL), LastSeenAt: lastSeen}
			if err := repo.CreateSession(ctx, sess); err != nil {
				t.Fatal(err)
			}
			got, err := svc.authenticateAt(ctx, "token", now)
			if err != nil || (got != nil) != tc.valid {
				t.Fatalf("authenticate = %v, %v", got, err)
			}
			saved, err := repo.FindSessionByHash(ctx, hashToken("token"))
			if err != nil {
				t.Fatal(err)
			}
			want := lastSeen
			if tc.touched {
				want = now
			}
			if !saved.LastSeenAt.Equal(want) {
				t.Fatalf("last seen = %v, want %v", saved.LastSeenAt, want)
			}
			if tc.valid {
				if _, err := svc.authenticateAt(ctx, "token", now.Add(time.Minute)); err != nil {
					t.Fatal(err)
				}
				again, err := repo.FindSessionByHash(ctx, hashToken("token"))
				if err != nil || !again.LastSeenAt.Equal(want) {
					t.Fatalf("repeat touch = %v, %v", again, err)
				}
			}
		})
	}
}

func TestReqAuth018_TouchFailureDoesNotFailRead(t *testing.T) {
	ctx := context.Background()
	db := reliabilityDB(t)
	players := player.NewRepo(db.DB)
	p := &player.Player{Name: "Session user", Active: true}
	if err := players.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db.DB)
	now := time.Now().UTC()
	if err := repo.CreateSession(ctx, &Session{TokenHash: hashToken("token"), PlayerID: p.ID, CreatedAt: now.Add(-2 * time.Hour), LastSeenAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, "CREATE TRIGGER reject_touch BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT, 'touch failed'); END"); err != nil {
		t.Fatal(err)
	}
	if got, err := NewService(players, repo).authenticateAt(ctx, "token", now); err != nil || got == nil {
		t.Fatalf("read = %v, %v", got, err)
	}
}
