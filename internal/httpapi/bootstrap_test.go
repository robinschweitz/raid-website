package httpapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"raid-website/internal/auth"
	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func TestBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []string{"success", "hash failure", "insert failure", "lookup failure", "unconfigured"} {
		t.Run(scenario, func(t *testing.T) {
			bdb, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "bootstrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := bdb.Close(); err != nil {
					t.Error(err)
				}
			})
			m, err := migrations.NewMigrator(bdb.DB)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			opts := Options{AdminUsername: " Owner ", AdminPassword: "initial-password"}
			switch scenario {
			case "hash failure":
				opts.AdminPassword = strings.Repeat("x", 73)
			case "insert failure":
				if _, err := bdb.DB.ExecContext(ctx, "CREATE TRIGGER reject_admin AFTER INSERT ON players BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
					t.Fatal(err)
				}
			case "lookup failure":
				if _, err := bdb.DB.ExecContext(ctx, "ALTER TABLE players RENAME COLUMN username TO unavailable_username"); err != nil {
					t.Fatal(err)
				}
			case "unconfigured":
				opts = Options{}
			}
			h, err := New(bdb.DB, opts)
			isFailure := strings.HasSuffix(scenario, "failure")
			if isFailure {
				if err == nil || h != nil {
					t.Fatalf("New = %v, %v; want no handler and startup error", h, err)
				}
			} else if err != nil || h == nil {
				t.Fatalf("New = %v, %v", h, err)
			}
			var count int
			if err := bdb.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM players").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if scenario != "success" {
				if count != 0 {
					t.Fatalf("bootstrap left %d partial players", count)
				}
				return
			}
			players := player.NewRepo(bdb.DB)
			p, err := players.FindByUsername(ctx, "owner")
			if err != nil || count != 1 {
				t.Fatalf("bootstrap account: count %d, error %v", count, err)
			}
			if p.Role != validation.RoleAdmin || !p.Active || p.PasswordHash == "" {
				t.Fatal("bootstrap account lacks admin credentials")
			}
			// Existing accounts must not be reactivated, promoted, or have credentials replaced.
			p.Role, p.Active = validation.RoleGuest, false
			if err := players.Update(ctx, p); err != nil {
				t.Fatal(err)
			}
			opts.AdminPassword = strings.Repeat("x", 73)
			if _, err := New(bdb.DB, opts); err != nil {
				t.Fatalf("existing account should skip password hashing: %v", err)
			}
			after, err := players.FindByUsername(ctx, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if after.ID != p.ID || after.Role != p.Role || after.Active != p.Active || after.PasswordHash != p.PasswordHash || !after.UpdatedAt.Equal(p.UpdatedAt) {
				t.Fatal("existing account was overwritten")
			}
			svc := auth.NewService(players, auth.NewRepo(bdb.DB))
			p.Active = true
			if err := players.Update(ctx, p); err != nil {
				t.Fatal(err)
			}
			if _, _, err := svc.Login(ctx, "owner", "initial-password"); err != nil {
				t.Fatalf("initial credentials do not work: %v", err)
			}
		})
	}
}
