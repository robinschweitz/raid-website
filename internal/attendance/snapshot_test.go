package attendance

import (
	"context"
	"path/filepath"
	"testing"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
)

func TestSnapshotMigrationLeavesExistingHistoryUnknown(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "snapshot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.DB.ExecContext(ctx, "CREATE TABLE raid_attendance (id INTEGER PRIMARY KEY); INSERT INTO raid_attendance (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	up, err := migrations.MigrationFiles.ReadFile("005_roster_snapshots.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	var name, class, spec string
	if err := database.DB.QueryRowContext(ctx, "SELECT character_name, class, spec FROM raid_attendance WHERE id = 1").Scan(&name, &class, &spec); err != nil {
		t.Fatal(err)
	}
	if name != "" || class != "" || spec != "" {
		t.Fatalf("fabricated history: %q %q %q", name, class, spec)
	}
	down, err := migrations.MigrationFiles.ReadFile("005_roster_snapshots.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := database.DB.QueryRowContext(ctx, "SELECT id FROM raid_attendance").Scan(&id); err != nil || id != 1 {
		t.Fatalf("down lost row: %d %v", id, err)
	}
}
