package lootcatalog

import (
	"context"
	"strings"
	"testing"
)

func TestEncounterChestSelections(t *testing.T) {
	s := NewService(NewRepo(nil))
	ctx := context.Background()
	for _, boss := range []string{"36789", "encounter:631:gunship", "encounter:649:faction-champions"} {
		for _, size := range []int{10, 25} {
			for _, mode := range []Mode{ModeNormal, ModeHeroic} {
				items, err := s.Items(ctx, Filter{BossID: boss, RaidSize: size, Mode: mode})
				if err != nil || len(items) == 0 {
					t.Fatalf("missing chest drops: %s %d %s: %v", boss, size, mode, err)
				}
				seen := map[string]bool{}
				for _, item := range items {
					if seen[item.ItemID] {
						t.Fatalf("duplicate chest item: %s", item.ItemID)
					}
					seen[item.ItemID] = true
					if item.RaidSize != size || item.Mode != mode {
						t.Fatal("variant leaked")
					}
					chests := map[string]bool{}
					for _, variant := range item.SourceVariants {
						for _, token := range strings.Split(variant.Difficulty, "/") {
							if strings.HasPrefix(strings.TrimSpace(token), "source-chest:") {
								chests[strings.TrimSpace(token)] = true
							}
						}
					}
					if len(chests) == 0 || boss == "encounter:631:gunship" && len(chests) != 2 {
						t.Fatalf("chest provenance lost: %+v", item)
					}
				}
			}
		}
	}
	for _, boss := range []string{"32845", "32906"} {
		for _, size := range []int{10, 25} {
			for _, mode := range []Mode{ModeNormal, ModeHardmode} {
				items, err := s.Items(ctx, Filter{BossID: boss, RaidSize: size, Mode: mode})
				if err != nil || len(items) == 0 {
					t.Fatalf("missing Ulduar chests %s %d %s", boss, size, mode)
				}
			}
		}
	}
	for _, boss := range []string{"encounter:533:four-horsemen", "encounter:649:tribute"} {
		items, err := s.Items(ctx, Filter{BossID: boss})
		if err != nil || len(items) == 0 {
			t.Fatal("missing collective reward", boss, err)
		}
		for _, item := range items {
			if boss == "encounter:649:tribute" && (item.Mode != ModeHeroic || !item.Conditional || !strings.Contains(item.SourceVariants[0].Difficulty, "attempts:")) {
				t.Fatal("tribute condition lost")
			}
		}
	}
}

func TestChestAwardContextAndCanonicalName(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	if _, err := s.EnsureBundled(ctx); err != nil {
		t.Fatal(err)
	}
	size := 10
	id := "encounter:631:gunship"
	items, err := s.Items(ctx, Filter{BossID: id, RaidSize: size, Mode: ModeNormal})
	if err != nil || len(items) == 0 {
		t.Fatal(err)
	}
	boss, item, err := s.ValidateDrop(ctx, "631", &size, &id, items[0].ItemID)
	if err != nil || boss.Name != "Luftschiffkampf" || item.Name != items[0].Name {
		t.Fatalf("encounter resolution: %+v %+v %v", boss, item, err)
	}
	if _, _, err := s.ValidateDrop(ctx, "603", &size, &id, item.ItemID); err == nil {
		t.Fatal("cross-instance chest accepted")
	}
	wrongBoss := "36789"
	// Shared emblems legitimately occur in both caches. An encounter-specific
	// equipment item must still be rejected for the other boss.
	for _, candidate := range items {
		if _, _, err := s.ValidateDrop(ctx, "631", &size, &wrongBoss, candidate.ItemID); err != nil {
			return
		}
	}
	t.Fatal("all Gunship items accepted for Valithria")
}
