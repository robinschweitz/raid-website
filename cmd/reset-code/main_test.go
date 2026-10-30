package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

func operatorDB(t *testing.T) *db.BunDB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "operator.db")
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("ENV", "development")
	database, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	m, err := migrations.NewMigrator(database.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return database
}

func TestReqAccount019_AC01(t *testing.T) {
	database := operatorDB(t)
	login := "locked-admin"
	p := &player.Player{Name: "Locked Admin", Username: &login, Role: validation.RoleAdmin, Active: true}
	if err := player.NewRepo(database.DB).Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"--player", "Locked Admin"}, &out); err != nil {
		t.Fatal(err)
	}
	plain := strings.TrimSpace(out.String())
	if !strings.HasPrefix(plain, "BD-") || strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("expected one code line, got %q", out.String())
	}
	var row registration.ResetCode
	if err := database.DB.NewSelect().Model(&row).Where("player_id = ?", p.ID).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(plain)))
	if row.CodeHash != hash || row.ExpiresAt.Sub(row.CreatedAt) != 72*time.Hour || row.UsedAt != nil || row.RevokedAt != nil {
		t.Fatal("code is not a valid 72-hour single-use code")
	}
	entries, err := audit.NewRepo(database.DB).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ActorName == nil || *entries[0].ActorName != "server operator" || strings.Contains(entries[0].AfterJSON, plain) {
		t.Fatalf("AC03 operator audit=%+v", entries)
	}
}
func TestReqAccount019_AC02(t *testing.T) {
	database := operatorDB(t)
	var out bytes.Buffer
	if err := run([]string{"--player-id", "999999"}, &out); err == nil {
		t.Fatal("unknown player accepted")
	}
	if out.Len() != 0 {
		t.Fatal("failure wrote a code")
	}
	count, err := database.DB.NewSelect().Model((*registration.ResetCode)(nil)).Count(context.Background())
	if err != nil || count != 0 {
		t.Fatalf("failed command changed rows: %d %v", count, err)
	}
}
