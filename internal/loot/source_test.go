package loot

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestCatalogSelectionRequiredForEveryNewAward(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "source.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	m, err := migrations.NewMigrator(database.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	catalog := lootcatalog.NewService(lootcatalog.NewRepo(database.DB))
	if _, err := catalog.EnsureBundled(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := catalog.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	drop := data.Drops[0]
	var instance string
	for _, boss := range data.Bosses {
		if boss.ID == drop.BossID {
			instance = boss.InstanceID
		}
	}
	now := time.Now().UTC()
	p := &player.Player{Name: "Actor", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(database.DB).Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "ICC25 never infers association", RaidLeaderID: p.ID, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database.DB).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database.DB)
	for _, setup := range []string{"unassociated", "missing_size", "name_only", "unknown_item", "source_drop"} {
		t.Run(setup, func(t *testing.T) {
			rd.CatalogInstanceID, rd.RaidSize = &instance, &drop.RaidSize
			if setup == "unassociated" {
				rd.CatalogInstanceID, rd.RaidSize = nil, nil
			}
			if setup == "missing_size" {
				rd.RaidSize = nil
			}
			if err := raid.NewRepo(database.DB).Update(ctx, rd); err != nil {
				t.Fatal(err)
			}
			for _, method := range []Method{Main, Second, BOE, Enhanced} {
				for _, source := range []string{"manual", "legacy_import"} {
					a := &Award{RaidID: rd.ID, Boss: "untrusted boss", ItemName: "untrusted item", CatalogBossID: &drop.BossID, ItemID: drop.ItemID, AwardMethod: method, Source: source, EnhancedCost: 0}
					want := error(nil)
					switch setup {
					case "unassociated", "missing_size":
						want = ErrCatalogContextRequired
					case "name_only":
						a.CatalogBossID, a.ItemID = nil, ""
						want = lootcatalog.ErrInvalid
					case "unknown_item":
						a.ItemID = "99999999"
						want = lootcatalog.ErrInvalid
					}
					if err := ValidateCatalog(ctx, repo, a, nil); !errors.Is(err, want) {
						t.Fatalf("%s %s: got %v want %v", method, source, err, want)
					}
					if want == nil {
						boss, item, err := catalog.ValidateDrop(ctx, instance, &drop.RaidSize, &drop.BossID, drop.ItemID)
						if err != nil || a.Boss != boss.Name || a.ItemName != item.Name {
							t.Fatalf("untrusted names survived: %+v %v", a, err)
						}
					}
				}
			}
		})
	}

	rd.CatalogInstanceID, rd.RaidSize = nil, nil
	if err := raid.NewRepo(database.DB).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	old := &Award{RaidID: rd.ID, Boss: "external boss", ItemName: "external item", Source: "legacy_import", AwardMethod: Second, AwardedToPlayerID: p.ID}
	for _, change := range []string{"notes", "recipient", "item", "boss", "enhanced"} {
		a := *old
		want := ErrCatalogContextRequired
		switch change {
		case "notes":
			a.Notes = "clarification"
			want = nil
		case "recipient":
			a.AwardedToPlayerID++
		case "item":
			a.ItemName = "new identity"
		case "boss":
			a.Boss = "new identity"
		case "enhanced":
			a.AwardMethod = Enhanced
		}
		if err := ValidateCatalog(ctx, repo, &a, old); !errors.Is(err, want) {
			t.Fatalf("historical %s: got %v want %v", change, err, want)
		}
	}
}
