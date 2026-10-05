package raidcorrection

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestCompletedCorrectionsOnlyApplyEligibilityDeltaWithoutOriginalAwards(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		openingBalance bool
		legacyLoot     bool
		archived       bool
	}{
		{name: "imported raid without loot", openingBalance: true},
		{name: "imported raid with loot", openingBalance: true, legacyLoot: true},
		{name: "manually constructed completed raid"},
		{name: "archived imported raid with opening balance", openingBalance: true, legacyLoot: true, archived: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "correction.db")+"?_pragma=foreign_keys(1)")
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
			r := NewRepo(database.DB)
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
			s := NewService(r)
			now := time.Now().UTC()
			players := []*player.Player{
				{Name: "First", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now},
				{Name: "Second", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now},
			}
			chars := make([]*character.Character, 2)
			for i, p := range players {
				if err := r.players.Create(ctx, p); err != nil {
					t.Fatal(err)
				}
				chars[i] = &character.Character{PlayerID: p.ID, Name: p.Name, Class: "Mage", MainSpec: "Fire", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
				if err := r.characters.Create(ctx, chars[i]); err != nil {
					t.Fatal(err)
				}
				if scenario.openingBalance {
					if err := r.points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: p.ID, Amount: 175, Type: "legacy_open", Reason: "opening balance", ActorID: players[0].ID, CreatedAt: now}); err != nil {
						t.Fatal(err)
					}
				}
			}
			rd := &raid.Raid{Name: "ICC25-01", CatalogInstanceID: &instance, RaidSize: &drop.RaidSize, RaidLeaderID: players[0].ID, Status: validation.RaidStatusCompleted, CreatedAt: now, UpdatedAt: now}
			if scenario.archived {
				rd.ArchivedAt, rd.CompletedAt = &now, &now
			}
			if err := r.raids.Create(ctx, rd); err != nil {
				t.Fatal(err)
			}
			a := &attendance.Attendance{RaidID: rd.ID, PlayerID: players[0].ID, CharacterID: chars[0].ID, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
			if err := r.attendance.Create(ctx, a); err != nil {
				t.Fatal(err)
			}
			if scenario.legacyLoot {
				if err := r.loot.CreateAward(ctx, &loot.Award{RaidID: rd.ID, ItemName: "historical offspec item", AwardedToPlayerID: players[0].ID, AwardedByPlayerID: players[0].ID, Source: "legacy_import", AwardMethod: loot.Second, AwardedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			}
			baseline := 0
			if scenario.openingBalance {
				baseline = 175
			}
			assertBalance := func(playerID int64, want int) {
				t.Helper()
				balance, err := r.points.Balance(ctx, playerID)
				if err != nil || balance != want {
					t.Fatalf("player %d balance: got %d want %d error %v", playerID, balance, want, err)
				}
			}
			assertCorrectionAmounts := func(playerID int64, want ...int) {
				t.Helper()
				rows, err := r.points.ListByPlayer(ctx, playerID)
				if err != nil {
					t.Fatal(err)
				}
				var amounts []int
				for _, row := range rows {
					if row.Type == "legacy_open" {
						if row.Amount != 175 || row.RaidID != nil {
							t.Fatal("opening balance mutated")
						}
						continue
					}
					if row.Type != "correction_no_mainspec_loot" || row.RaidID == nil || *row.RaidID != rd.ID {
						t.Fatalf("unexpected historical award: %+v", row)
					}
					amounts = append(amounts, row.Amount)
				}
				if len(amounts) != len(want) {
					t.Fatalf("correction amounts: got %v want %v", amounts, want)
				}
				for i := range want {
					if amounts[i] != want[i] {
						t.Fatalf("correction amounts: got %v want %v", amounts, want)
					}
				}
			}
			if _, err := s.CorrectAttendance(ctx, rd.ID, a.ID, players[0].ID, "notes correction", func(a *attendance.Attendance) { a.Notes = "clarification"; a.IsBench = true }); err != nil {
				t.Fatal(err)
			}
			assertBalance(players[0].ID, baseline)
			assertCorrectionAmounts(players[0].ID)
			award := &loot.Award{RaidID: rd.ID, CatalogBossID: &drop.BossID, ItemID: drop.ItemID, AwardedToPlayerID: players[0].ID, AwardedToCharacterID: &chars[0].ID, AwardMethod: loot.Second}
			if err := s.SaveLoot(ctx, award, players[0].ID, "missing offspec loot"); err != nil {
				t.Fatal(err)
			}
			assertCorrectionAmounts(players[0].ID)
			for i := 0; i < 2; i++ {
				if _, err := s.CorrectLoot(ctx, rd.ID, award.ID, players[0].ID, "main spec correction", func(a *loot.Award) { a.AwardMethod = loot.Main }); err != nil {
					t.Fatal(err)
				}
				assertBalance(players[0].ID, baseline-10)
				if _, err := s.CorrectLoot(ctx, rd.ID, award.ID, players[0].ID, "unchanged correction", func(a *loot.Award) { a.Notes = "same eligibility" }); err != nil {
					t.Fatal(err)
				}
				if _, err := s.CorrectLoot(ctx, rd.ID, award.ID, players[0].ID, "undo main spec correction", func(a *loot.Award) { a.AwardMethod = loot.Second }); err != nil {
					t.Fatal(err)
				}
				assertBalance(players[0].ID, baseline)
			}
			assertCorrectionAmounts(players[0].ID, -10, 10, -10, 10)
			if _, err := s.CorrectAttendance(ctx, rd.ID, a.ID, players[0].ID, "participant correction", func(a *attendance.Attendance) { a.PlayerID = players[1].ID; a.CharacterID = chars[1].ID }); err != nil {
				t.Fatal(err)
			}
			assertBalance(players[0].ID, baseline-10)
			assertBalance(players[1].ID, baseline+10)
			if _, err := s.CorrectAttendance(ctx, rd.ID, a.ID, players[0].ID, "undo participant correction", func(a *attendance.Attendance) { a.PlayerID = players[0].ID; a.CharacterID = chars[0].ID }); err != nil {
				t.Fatal(err)
			}
			assertBalance(players[0].ID, baseline)
			assertBalance(players[1].ID, baseline)
			addition := &attendance.Attendance{RaidID: rd.ID, PlayerID: players[1].ID, CharacterID: chars[1].ID, Role: validation.RoleDD}
			if err := s.SaveAttendance(ctx, addition, players[0].ID, "missing participant"); err != nil {
				t.Fatal(err)
			}
			assertBalance(players[1].ID, baseline+10)
			assertCorrectionAmounts(players[1].ID, 10, -10, 10)
			assertCorrectionAmounts(players[0].ID, -10, 10, -10, 10, -10, 10)
		})
	}
}
