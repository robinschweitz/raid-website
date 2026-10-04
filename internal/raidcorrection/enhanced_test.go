package raidcorrection

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestConcurrentEnhancedAwardsAcrossDatabaseHandles(t *testing.T) {
	for _, sameRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_request_%t", sameRequest), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			dsn := "file:" + filepath.Join(t.TempDir(), "enhanced.db") + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)"
			database, err := db.Open(ctx, dsn)
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
			r := NewRepo(database.DB)
			now := time.Now().UTC()
			p := &player.Player{Name: "Winner", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
			if err := r.players.Create(ctx, p); err != nil {
				t.Fatal(err)
			}
			c := &character.Character{PlayerID: p.ID, Name: "Winnerchar", Class: "Mage", MainSpec: "Fire", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
			if err := r.characters.Create(ctx, c); err != nil {
				t.Fatal(err)
			}
			rd := &raid.Raid{Name: "ICC", RaidLeaderID: p.ID, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
			if err := r.raids.Create(ctx, rd); err != nil {
				t.Fatal(err)
			}
			if err := r.attendance.Create(ctx, &attendance.Attendance{RaidID: rd.ID, PlayerID: p.ID, CharacterID: c.ID, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
			if err := r.points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: p.ID, Amount: 70, Type: "legacy_open", Reason: "history", ActorID: p.ID, CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			services := make([]*Service, 8)
			for i := range services {
				connection, err := db.Open(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = connection.Close() })
				services[i] = NewService(NewRepo(connection.DB))
			}
			start := make(chan struct{})
			results := make(chan *loot.Award, 8)
			errs := make(chan error, 8)
			var wg sync.WaitGroup
			for i, service := range services {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					requestID := fmt.Sprint(i)
					if sameRequest {
						requestID = "same"
					}
					award := &loot.Award{RaidID: rd.ID, ItemName: "winning loot", AwardMethod: loot.Enhanced, AwardedToPlayerID: p.ID, AwardedToCharacterID: &c.ID, RequestID: &requestID}
					if err := service.SaveLoot(ctx, award, p.ID, ""); err != nil {
						errs <- err
						return
					}
					results <- award
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			close(errs)
			if sameRequest {
				if len(results) != 8 || len(errs) != 0 {
					t.Fatalf("retry successes %d errors %d", len(results), len(errs))
				}
				var awardID int64
				for result := range results {
					if awardID == 0 {
						awardID = result.ID
					}
					if result.ID != awardID || result.EnhancedCost != 50 {
						t.Fatalf("duplicate cost: %+v", result)
					}
				}
			} else {
				if len(results) != 2 || len(errs) != 6 {
					t.Fatalf("successes %d errors %d", len(results), len(errs))
				}
				costs := map[int]int{}
				for result := range results {
					costs[result.EnhancedCost]++
				}
				if costs[50] != 1 || costs[20] != 1 {
					t.Fatalf("stale balance costs: %v", costs)
				}
			}
			for err := range errs {
				if !errors.Is(err, enhancepoints.ErrInsufficientBalance) {
					t.Errorf("unexpected concurrency error: %v", err)
				}
			}
			wantBalance, wantAwards := 0, 2
			if sameRequest {
				wantBalance, wantAwards = 20, 1
			}
			balance, err := r.points.Balance(ctx, p.ID)
			if err != nil || balance != wantBalance {
				t.Fatalf("balance %d error %v", balance, err)
			}
			awards, err := r.loot.ListByRaid(ctx, rd.ID)
			if err != nil || len(awards) != wantAwards {
				t.Fatalf("awards %v error %v", awards, err)
			}
			entries, err := r.points.ListByPlayer(ctx, p.ID)
			if err != nil || len(entries) != wantAwards+1 {
				t.Fatalf("entries %v error %v", entries, err)
			}
			if sameRequest {
				correctionStart := make(chan struct{})
				correctionErrors := make(chan error, len(services))
				for _, service := range services {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-correctionStart
						_, err := service.CorrectLoot(ctx, rd.ID, awards[0].ID, p.ID, "not enhanced", func(a *loot.Award) { a.AwardMethod = loot.Second })
						correctionErrors <- err
					}()
				}
				close(correctionStart)
				wg.Wait()
				close(correctionErrors)
				for err := range correctionErrors {
					if err != nil {
						t.Fatal(err)
					}
				}
				balance, err := r.points.Balance(ctx, p.ID)
				if err != nil || balance != 70 {
					t.Fatalf("concurrent refunds: %d %v", balance, err)
				}
				entries, err := r.points.ListByPlayer(ctx, p.ID)
				if err != nil || len(entries) != 3 {
					t.Fatalf("duplicate refunds: %v %v", entries, err)
				}
			}
		})
	}
}
