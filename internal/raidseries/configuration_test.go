package raidseries_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raidseries"
)

func TestConcurrentConfigurationPatchesPreserveUnrelatedFields(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "series.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	migrator, err := migrations.NewMigrator(database.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	database.DB.SetMaxOpenConns(4)
	now := time.Now().UTC()
	actor := &player.Player{Name: "Leader", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(database.DB).Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	repo := raidseries.NewRepo(database.DB)
	service := raidseries.NewService(repo)
	for i := 0; i < 20; i++ {
		series := &raidseries.Series{Name: "Old name", RaidLeaderID: actor.ID, Weekdays: []int{1}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
		if err := service.Save(ctx, series); err != nil {
			t.Fatal(err)
		}
		name := "Changed name"
		interval := int64(3)
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := service.Configure(ctx, series.ID, raidseries.Changes{Name: &name})
			results <- err
		}()
		go func() {
			<-start
			_, err := service.Configure(ctx, series.ID, raidseries.Changes{IntervalWeeks: &interval})
			results <- err
		}()
		close(start)
		for j := 0; j < 2; j++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		stored, err := repo.Get(ctx, series.ID)
		if err != nil || stored.Name != name || stored.IntervalWeeks != interval || stored.StartTime != series.StartTime || stored.Timezone != series.Timezone {
			t.Fatalf("patch overwrote config: %+v %v", stored, err)
		}
	}
}
