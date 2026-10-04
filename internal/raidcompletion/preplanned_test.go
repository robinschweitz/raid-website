package raidcompletion

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestArchiveReusesPreplannedOccurrenceWithoutOverwriting(t *testing.T) {
	for _, scenario := range []string{"planned", "open", "same_parent", "cancelled", "completed", "archived_planned", "archived_open", "different_parent", "parent_has_other_successor", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			database := setupDB(t)
			actor := seedPlayer(t, ctx, player.NewRepo(database), "Actor")
			manualLeader := seedPlayer(t, ctx, player.NewRepo(database), "Manual leader")
			charID := seedCharacter(t, ctx, character.NewRepo(database), actor, "Livechar")
			now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
			series := &raidseries.Series{Name: "Configured series name", RaidLeaderID: actor, Weekdays: []int{3}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
			if err := raidseries.NewService(raidseries.NewRepo(database)).Save(ctx, series); err != nil {
				t.Fatal(err)
			}
			start := now.AddDate(0, 0, -1)
			current := &raid.Raid{Name: "Current", RaidLeaderID: actor, SeriesID: &series.ID, ScheduledStart: &start, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
			raids := raid.NewRepo(database)
			if err := raids.Create(ctx, current); err != nil {
				t.Fatal(err)
			}
			att := attendance.NewRepo(database)
			row := &attendance.Attendance{RaidID: current.ID, PlayerID: actor, CharacterID: charID, CharacterName: "Current snapshot", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
			if err := att.Create(ctx, row); err != nil {
				t.Fatal(err)
			}
			lock := &enhancepoints.ItemLock{PlayerID: actor, RaidID: current.ID, ItemName: "Unused item", Status: validation.LockStatusActive, LockedAt: now, ActorID: actor, CreatedAt: now, UpdatedAt: now}
			if _, err := database.NewInsert().Model(lock).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			nextStart, err := raidseries.NextOccurrence(series, start, now)
			if err != nil {
				t.Fatal(err)
			}
			end := nextStart.Add(3 * time.Hour)
			created := now.AddDate(0, 0, -20)
			updated := created.Add(time.Hour)
			future := &raid.Raid{Name: "Manually configured name", RaidLeaderID: manualLeader, SeriesID: &series.ID, ScheduledStart: &nextStart, ScheduledEnd: &end, Notes: "Keep manual raid configuration", Status: validation.RaidStatusPlanned, CreatedAt: created, UpdatedAt: updated}
			if scenario == "open" {
				future.Status = validation.RaidStatusOpen
				future.StartedAt = &now
			}
			if scenario == "same_parent" {
				future.PreviousRaidID = &current.ID
			}
			if scenario == "cancelled" {
				future.Status = validation.RaidStatusCancelled
				future.CancelledAt = &now
			}
			if scenario == "completed" {
				future.Status = validation.RaidStatusCompleted
				future.CompletedAt = &now
			}
			if scenario == "archived_planned" || scenario == "archived_open" {
				future.ArchivedAt = &now
			}
			if scenario == "archived_open" {
				future.Status = validation.RaidStatusOpen
			}
			if scenario == "different_parent" || scenario == "parent_has_other_successor" {
				other := &raid.Raid{Name: "Other linked raid", RaidLeaderID: actor, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
				if scenario == "parent_has_other_successor" {
					other.PreviousRaidID = &current.ID
				}
				if err := raids.Create(ctx, other); err != nil {
					t.Fatal(err)
				}
				if scenario == "different_parent" {
					future.PreviousRaidID = &other.ID
				}
			}
			if err := raids.Create(ctx, future); err != nil {
				t.Fatal(err)
			}
			futureRow := &attendance.Attendance{RaidID: future.ID, PlayerID: actor, CharacterID: charID, CharacterName: "Future actual name", Class: "Priester", Spec: "Future custom spec", Role: validation.RoleHeal, IsBench: true, Notes: "Preserve future roster", CreatedAt: created, UpdatedAt: updated}
			if err := att.Create(ctx, futureRow); err != nil {
				t.Fatal(err)
			}
			before, err := raids.Get(ctx, future.ID)
			if err != nil {
				t.Fatal(err)
			}
			beforeRow, err := att.Get(ctx, futureRow.ID)
			if err != nil {
				t.Fatal(err)
			}
			beforeList, err := raids.List(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "rollback" {
				if _, err := database.ExecContext(ctx, "CREATE TRIGGER fail_points BEFORE INSERT ON enhance_point_transactions BEGIN SELECT RAISE(ABORT, 'points failed after successor link'); END"); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewService(NewRepo(database))
			svc.now = func() time.Time { return now }
			archived, next, archiveErr := svc.ArchiveRaid(ctx, current.ID, actor)
			shouldSucceed := scenario == "planned" || scenario == "open" || scenario == "same_parent"
			if shouldSucceed {
				if archiveErr != nil || archived.Status != validation.RaidStatusCompleted || next == nil || next.ID != future.ID {
					t.Fatalf("preplanned reuse: %+v %+v %v", archived, next, archiveErr)
				}
				before.PreviousRaidID = &current.ID
				if !reflect.DeepEqual(before, next) {
					t.Fatalf("returned successor metadata overwritten: want %+v got %+v", before, next)
				}
				// A later retry must return the saved occurrence, not recalculate from the later clock.
				now = now.AddDate(0, 0, 30)
				_, retry, err := svc.ArchiveRaid(ctx, current.ID, actor)
				if err != nil || !reflect.DeepEqual(next, retry) {
					t.Fatalf("retry changed successor: %+v %v", retry, err)
				}
			} else {
				if archiveErr == nil || archived != nil || next != nil {
					t.Fatalf("conflicting occurrence accepted: %+v %+v %v", archived, next, archiveErr)
				}
				if scenario != "rollback" && !errors.Is(archiveErr, raid.ErrOccurrenceConflict) {
					t.Fatalf("want typed occurrence conflict, got %v", archiveErr)
				}
				stored, err := raids.Get(ctx, current.ID)
				if err != nil || stored.Status != validation.RaidStatusOpen || stored.ArchivedAt != nil || stored.CompletedAt != nil {
					t.Fatalf("failed archival changed parent: %+v %v", stored, err)
				}
			}
			after, err := raids.Get(ctx, future.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("future configuration overwritten: before %+v after %+v err=%v", before, after, err)
			}
			afterRow, err := att.Get(ctx, futureRow.ID)
			if err != nil || !reflect.DeepEqual(beforeRow, afterRow) {
				t.Fatalf("future roster overwritten: %+v %v", afterRow, err)
			}
			afterList, err := raids.List(ctx, true)
			if err != nil || len(afterList) != len(beforeList) {
				t.Fatalf("duplicate occurrence created: %d %v", len(afterList), err)
			}
			balance, err := enhancepoints.NewRepo(database).Balance(ctx, actor)
			wantBalance := 0
			if shouldSucceed {
				wantBalance = 10
			}
			if err != nil || balance != wantBalance {
				t.Fatalf("EP exactly once: want %d got %d err=%v", wantBalance, balance, err)
			}
			if err := database.NewSelect().Model(lock).WherePK().Scan(ctx); err != nil {
				t.Fatal(err)
			}
			wantLockStatus := validation.LockStatusActive
			if shouldSucceed {
				wantLockStatus = validation.LockStatusExpired
			}
			if lock.Status != wantLockStatus {
				t.Fatalf("lock consequence: want %s got %s", wantLockStatus, lock.Status)
			}
		})
	}
}

func TestConcurrentArchivesDoNotClobberPreplannedParent(t *testing.T) {
	for _, sameSeries := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_series", false: "different_series"}[sameSeries], func(t *testing.T) {
			ctx := context.Background()
			database := setupDB(t)
			database.SetMaxOpenConns(4)
			actor := seedPlayer(t, ctx, player.NewRepo(database), "Actor")
			charID := seedCharacter(t, ctx, character.NewRepo(database), actor, "Actorchar")
			raids := raid.NewRepo(database)
			now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
			svc := NewService(NewRepo(database))
			svc.now = func() time.Time { return now }
			for attempt := 0; attempt < 10; attempt++ {
				var currents, futures []*raid.Raid
				var series *raidseries.Series
				for i := 0; i < 2; i++ {
					if i == 0 || !sameSeries {
						series = &raidseries.Series{Name: "Series", RaidLeaderID: actor, Weekdays: []int{3}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
						if err := raidseries.NewService(raidseries.NewRepo(database)).Save(ctx, series); err != nil {
							t.Fatal(err)
						}
					}
					start := now.AddDate(0, 0, -1-i)
					current := &raid.Raid{Name: "Current", RaidLeaderID: actor, SeriesID: &series.ID, ScheduledStart: &start, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
					if err := raids.Create(ctx, current); err != nil {
						t.Fatal(err)
					}
					currents = append(currents, current)
					row := &attendance.Attendance{RaidID: current.ID, PlayerID: actor, CharacterID: charID, CharacterName: "Actorchar", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
					if err := attendance.NewRepo(database).Create(ctx, row); err != nil {
						t.Fatal(err)
					}
					if i == 0 || !sameSeries {
						nextStart, err := raidseries.NextOccurrence(series, start, now)
						if err != nil {
							t.Fatal(err)
						}
						future := &raid.Raid{Name: "Manual future", RaidLeaderID: actor, SeriesID: &series.ID, ScheduledStart: &nextStart, Status: validation.RaidStatusPlanned, Notes: "Do not overwrite", CreatedAt: now, UpdatedAt: now}
						if err := raids.Create(ctx, future); err != nil {
							t.Fatal(err)
						}
						stored, err := raids.Get(ctx, future.ID)
						if err != nil {
							t.Fatal(err)
						}
						futures = append(futures, stored)
					}
				}
				start := make(chan struct{})
				type result struct {
					id  int64
					err error
				}
				results := make(chan result, 2)
				for _, current := range currents {
					go func() {
						<-start
						_, _, err := svc.ArchiveRaid(ctx, current.ID, actor)
						results <- result{current.ID, err}
					}()
				}
				close(start)
				winners := map[int64]bool{}
				for i := 0; i < 2; i++ {
					outcome := <-results
					if outcome.err == nil {
						winners[outcome.id] = true
					} else if !sameSeries || !errors.Is(outcome.err, raid.ErrOccurrenceConflict) {
						t.Fatal(outcome.err)
					}
				}
				wantWinners := 2
				if sameSeries {
					wantWinners = 1
				}
				if len(winners) != wantWinners {
					t.Fatalf("want %d winners got %v", wantWinners, winners)
				}
				for _, current := range currents {
					stored, err := raids.Get(ctx, current.ID)
					if err != nil {
						t.Fatal(err)
					}
					if winners[current.ID] {
						if stored.Status != validation.RaidStatusCompleted || stored.ArchivedAt == nil {
							t.Fatalf("winner not finalized: %+v", stored)
						}
						_, retry, err := svc.ArchiveRaid(ctx, current.ID, actor)
						if err != nil || retry == nil || *retry.PreviousRaidID != current.ID {
							t.Fatalf("saved parent lost: %+v %v", retry, err)
						}
					} else if stored.Status != validation.RaidStatusOpen || stored.CompletedAt != nil || stored.ArchivedAt != nil {
						t.Fatalf("loser partially finalized: %+v", stored)
					}
					transactions, err := enhancepoints.NewRepo(database).ListByPlayer(ctx, actor)
					if err != nil {
						t.Fatal(err)
					}
					awards := 0
					for _, entry := range transactions {
						if entry.RaidID != nil && *entry.RaidID == current.ID {
							awards++
							if entry.Amount != 10 {
								t.Fatal(entry)
							}
						}
					}
					wantAwards := 0
					if winners[current.ID] {
						wantAwards = 1
					}
					if awards != wantAwards {
						t.Fatalf("parent EP mismatch: %d %d", awards, wantAwards)
					}
				}
				for _, future := range futures {
					stored, err := raids.Get(ctx, future.ID)
					if err != nil {
						t.Fatal(err)
					}
					if stored.PreviousRaidID == nil || !winners[*stored.PreviousRaidID] {
						t.Fatalf("future parent clobbered: %+v", stored)
					}
					future.PreviousRaidID = stored.PreviousRaidID
					if !reflect.DeepEqual(future, stored) {
						t.Fatalf("concurrent archive overwrote future config: %+v", stored)
					}
				}
			}
		})
	}
}
