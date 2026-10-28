package accountmerge

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func setupDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	bdb, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = bdb.Close() })
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return bdb.DB
}

// AC-05: a failure half-way leaves every row as it was.
func TestReqPrivacy011_AC05(t *testing.T) {
	ctx := context.Background()
	bdb := setupDB(t)
	now := time.Now().UTC()
	uname := "rollback-user"
	admin := &player.Player{Name: "admin", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
	target := &player.Player{Name: "Original", Username: &uname, PasswordHash: "hash", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	for _, p := range []*player.Player{admin, target} {
		if err := player.NewRepo(bdb).Create(ctx, p); err != nil {
			t.Fatalf("create player: %v", err)
		}
	}
	if _, err := bdb.ExecContext(ctx, "INSERT INTO sessions (token_hash, player_id, created_at, expires_at, last_seen_at) VALUES ('t', ?, ?, ?, ?)", target.ID, now, now.Add(time.Hour), now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// Breaking a later step makes the operation fail after the sessions were deleted.
	if _, err := bdb.ExecContext(ctx, "ALTER TABLE login_attempts RENAME TO login_attempts_broken"); err != nil {
		t.Fatalf("break table: %v", err)
	}

	if err := NewService(NewRepo(bdb)).Anonymise(ctx, target.ID, admin.ID, ""); err == nil {
		t.Fatal("anonymise succeeded despite the failure")
	}
	got, err := player.NewRepo(bdb).Get(ctx, target.ID)
	if err != nil || got.Name != "Original" || got.Username == nil || got.PasswordHash != "hash" {
		t.Fatalf("player changed: %+v, %v", got, err)
	}
	var sessions, audits int
	if err := bdb.NewSelect().TableExpr("sessions").ColumnExpr("COUNT(*)").Where("player_id = ?", target.ID).Scan(ctx, &sessions); err != nil || sessions != 1 {
		t.Fatalf("sessions = %d, %v", sessions, err)
	}
	if err := bdb.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("action = ?", AnonymiseAction).Scan(ctx, &audits); err != nil || audits != 0 {
		t.Fatalf("audits = %d, %v", audits, err)
	}
}
