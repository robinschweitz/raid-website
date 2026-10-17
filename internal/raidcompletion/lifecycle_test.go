package raidcompletion

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

func TestConcurrentRaidLifecycle(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	db.SetMaxOpenConns(4)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	charID := seedCharacter(t, ctx, character.NewRepo(db), actor, "Actorchar")
	raids := raid.NewRepo(db)
	lifecycle := raid.NewService(raids)
	archive := NewService(NewRepo(db))
	for _, cancel := range []bool{false, true} {
		for i := 0; i < 20; i++ {
			now := time.Now().UTC()
			rd := &raid.Raid{Name: fmt.Sprintf("race-%t-%d", cancel, i), RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
			seedSchedule(t, ctx, db, rd)
			if err := raids.Create(ctx, rd); err != nil {
				t.Fatal(err)
			}
			a := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: charID, CharacterName: "Actorchar", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, IsBench: true, CreatedAt: now, UpdatedAt: now}
			if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() { <-start; _, _, err := archive.ArchiveRaid(ctx, rd.ID, actor); results <- err }()
			go func() {
				<-start
				var err error
				if cancel {
					_, err = lifecycle.CancelRaid(ctx, rd.ID, actor)
				} else {
					_, _, err = archive.ArchiveRaid(ctx, rd.ID, actor)
				}
				results <- err
			}()
			close(start)
			for j := 0; j < 2; j++ {
				err := <-results
				if err != nil && !errors.Is(err, ErrRaidCancelled) && !errors.Is(err, raid.ErrNotCancellable) {
					t.Fatal(err)
				}
			}
			stored, err := raids.Get(ctx, rd.ID)
			if err != nil {
				t.Fatal(err)
			}
			transactions, err := enhancepoints.NewRepo(db).ListByPlayer(ctx, actor)
			if err != nil {
				t.Fatal(err)
			}
			awards := 0
			for _, entry := range transactions {
				if entry.RaidID != nil && *entry.RaidID == rd.ID {
					awards++
					if entry.Amount != 10 {
						t.Fatal(entry)
					}
				}
			}
			next, err := raids.Successor(ctx, rd.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status == validation.RaidStatusCompleted {
				if stored.CompletedAt == nil || stored.ArchivedAt == nil || stored.CancelledAt != nil || awards != 1 || next == nil {
					t.Fatalf("archive effects: %+v, awards=%d next=%+v", stored, awards, next)
				}
			} else if stored.Status != validation.RaidStatusCancelled || stored.CompletedAt != nil || stored.ArchivedAt != nil || awards != 0 || next != nil {
				t.Fatalf("cancel effects: %+v, awards=%d next=%+v", stored, awards, next)
			}
		}
	}
}

func TestLifecycleMissingActorAndRaid(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	raids := raid.NewRepo(db)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Empty", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	operations := []func(int64, int64) error{
		func(id, actor int64) error {
			_, _, err := NewService(NewRepo(db)).ArchiveRaid(ctx, id, actor)
			return err
		},
		func(id, actor int64) error { _, err := raid.NewService(raids).CancelRaid(ctx, id, actor); return err },
		func(id, actor int64) error {
			seriesID := int64(1)
			_, err := raid.NewService(raids).ConfigureSchedule(ctx, id, actor, raid.ScheduleChanges{SeriesID: &seriesID})
			return err
		},
	}
	for _, operation := range operations {
		if err := operation(rd.ID, 999999); !errors.Is(err, raid.ErrActorNotFound) {
			t.Fatalf("missing actor: %v", err)
		}
		if err := operation(999999, actor); !errors.Is(err, raid.ErrNotFound) {
			t.Fatalf("missing raid: %v", err)
		}
	}
}

func TestArchiveRollsBackLedgersLocksAndSuccessor(t *testing.T) {
	for _, failure := range []string{"expiry", "successor", "lifecycle"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			db := setupDB(t)
			actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
			charID := seedCharacter(t, ctx, character.NewRepo(db), actor, "Actorchar")
			raids := raid.NewRepo(db)
			now := time.Now().UTC()
			rd := &raid.Raid{Name: "Rollback", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
			seedSchedule(t, ctx, db, rd)
			if err := raids.Create(ctx, rd); err != nil {
				t.Fatal(err)
			}
			before, err := raids.Get(ctx, rd.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: charID, CharacterName: "Actorchar", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
			if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
				t.Fatal(err)
			}
			lock := &enhancepoints.ItemLock{PlayerID: actor, RaidID: rd.ID, ItemName: "Item", Status: validation.LockStatusActive, LockedAt: now, ActorID: actor, CreatedAt: now, UpdatedAt: now}
			if _, err := db.NewInsert().Model(lock).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			trigger := "CREATE TRIGGER fail_archive BEFORE UPDATE ON item_locks BEGIN SELECT RAISE(ABORT, 'expiry failed'); END"
			if failure == "successor" {
				trigger = "CREATE TRIGGER fail_archive BEFORE INSERT ON raids BEGIN SELECT RAISE(ABORT, 'successor failed'); END"
			}
			if failure == "lifecycle" {
				trigger = "CREATE TRIGGER fail_archive BEFORE UPDATE ON raids BEGIN SELECT RAISE(ABORT, 'lifecycle failed'); END"
			}
			if _, err := db.ExecContext(ctx, trigger); err != nil {
				t.Fatal(err)
			}
			svc := NewService(NewRepo(db))
			if _, _, err := svc.ArchiveRaid(ctx, rd.ID, actor); err == nil {
				t.Fatal("expected failure")
			}
			stored, err := raids.Get(ctx, rd.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != validation.RaidStatusOpen || stored.CompletedAt != nil || stored.ArchivedAt != nil || !stored.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("raid did not roll back: %+v", stored)
			}
			balance, err := enhancepoints.NewRepo(db).Balance(ctx, actor)
			if err != nil || balance != 0 {
				t.Fatalf("ledger did not roll back: %d %v", balance, err)
			}
			if err := db.NewSelect().Model(lock).WherePK().Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if lock.Status != validation.LockStatusActive || lock.ExpiredAt != nil {
				t.Fatalf("lock did not roll back: %+v", lock)
			}
			next, err := raids.Successor(ctx, rd.ID)
			if err != nil || next != nil {
				t.Fatalf("successor did not roll back: %+v %v", next, err)
			}
			if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_archive"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if _, _, err := svc.ArchiveRaid(ctx, rd.ID, actor); err != nil {
					t.Fatal(err)
				}
			}
			balance, err = enhancepoints.NewRepo(db).Balance(ctx, actor)
			if err != nil || balance != 10 {
				t.Fatalf("retry ledger: %d %v", balance, err)
			}
		})
	}
}

func TestHistoricAndOldArchivedActiveRaids(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	charID := seedCharacter(t, ctx, character.NewRepo(db), actor, "Actorchar")
	raids := raid.NewRepo(db)
	now := time.Now().UTC()
	opening := &enhancepoints.Transaction{PlayerID: actor, Amount: 73, Type: "import_opening", Reason: "imported balance", ActorID: actor, CreatedAt: now}
	if err := enhancepoints.NewRepo(db).InsertTransaction(ctx, opening); err != nil {
		t.Fatal(err)
	}
	for _, status := range []validation.RaidStatus{validation.RaidStatusCompleted, validation.RaidStatusOpen} {
		rd := &raid.Raid{Name: "Legacy", RaidLeaderID: actor, Status: status, CreatedAt: now, UpdatedAt: now}
		if status == validation.RaidStatusCompleted {
			rd.CompletedAt = &now
		} else {
			rd.ArchivedAt = &now
		}
		if err := raids.Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
		a := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: charID, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
		if status != validation.RaidStatusCompleted {
			a.CharacterName = "Actorchar"
			a.Class = "Krieger"
			a.Spec = "Waffen"
		}
		if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
			t.Fatal(err)
		}
		svc := NewService(NewRepo(db))
		if status == validation.RaidStatusCompleted {
			before, err := raids.Get(ctx, rd.ID)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				stored, next, err := svc.ArchiveRaid(ctx, rd.ID, actor)
				if err != nil || next != nil || stored.ArchivedAt == nil || !stored.CompletedAt.Equal(*before.CompletedAt) {
					t.Fatalf("historical result: %+v %+v %v", stored, next, err)
				}
			}
			balance, err := enhancepoints.NewRepo(db).Balance(ctx, actor)
			if err != nil || balance != 73 {
				t.Fatalf("historical award recreated: %d %v", balance, err)
			}
		} else {
			if _, _, err := svc.ArchiveRaid(ctx, rd.ID, actor); !errors.Is(err, raidseries.ErrSeriesRequired) {
				t.Fatalf("missing series: %v", err)
			}
			seedSchedule(t, ctx, db, rd)
			if err := raids.Update(ctx, rd); err != nil {
				t.Fatal(err)
			}
			if _, _, err := svc.ArchiveRaid(ctx, rd.ID, actor); err != nil {
				t.Fatal(err)
			}
			balance, err := enhancepoints.NewRepo(db).Balance(ctx, actor)
			if err != nil || balance != 83 {
				t.Fatalf("explicit legacy finalization: %d %v", balance, err)
			}
		}
	}
	listed, err := raids.List(ctx, false)
	if err != nil || len(listed) != 3 {
		t.Fatalf("history excluded: %d %v", len(listed), err)
	}
}

func TestArchivePrerequisitesAndChangedSeriesLeader(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	leader := seedPlayer(t, ctx, player.NewRepo(db), "New leader")
	now := time.Now().UTC()
	raids := raid.NewRepo(db)
	rd := &raid.Raid{Name: "Existing raid", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	seedSchedule(t, ctx, db, rd)
	start := rd.ScheduledStart
	rd.ScheduledStart = nil
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepo(db))
	if _, _, err := svc.ArchiveRaid(ctx, rd.ID, actor); !errors.Is(err, raidseries.ErrScheduledStartRequired) {
		t.Fatal(err)
	}
	rd.ScheduledStart = start
	if err := raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	seriesRepo := raidseries.NewRepo(db)
	series, err := seriesRepo.Get(ctx, *rd.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	series.Name = "Changed series"
	series.RaidLeaderID = leader
	if err := raidseries.NewService(seriesRepo).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	stored, next, err := svc.ArchiveRaid(ctx, rd.ID, actor)
	if err != nil || next.RaidLeaderID != leader || next.Name != series.Name || stored.Name != rd.Name || stored.RaidLeaderID != actor {
		t.Fatalf("series/historical metadata: %+v %+v %v", stored, next, err)
	}
	if _, err := raid.NewService(raids).ConfigureSchedule(ctx, rd.ID, actor, raid.ScheduleChanges{SeriesID: &series.ID}); !errors.Is(err, raid.ErrScheduleNotEditable) {
		t.Fatalf("historical overwrite allowed: %v", err)
	}
}

func TestArchiveScheduleErrorsAndOccurrenceConflictAreAtomic(t *testing.T) {
	for _, scenario := range []string{"gap", "fold", "occupied", "invalid_series"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db := setupDB(t)
			actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
			charID := seedCharacter(t, ctx, character.NewRepo(db), actor, "Actorchar")
			now := time.Date(2026, 3, 22, 2, 0, 0, 0, time.UTC)
			if scenario == "fold" {
				now = time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
			}
			start := now.Add(-time.Hour)
			series := &raidseries.Series{Name: "Series", RaidLeaderID: actor, Weekdays: []int{0}, IntervalWeeks: 1, StartTime: "02:30", Timezone: "Europe/Berlin"}
			if scenario == "occupied" {
				series.StartTime = "20:00"
			}
			seriesRepo := raidseries.NewRepo(db)
			if err := raidseries.NewService(seriesRepo).Save(ctx, series); err != nil {
				t.Fatal(err)
			}
			rd := &raid.Raid{Name: "Current", RaidLeaderID: actor, SeriesID: &series.ID, Status: validation.RaidStatusOpen, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
			raids := raid.NewRepo(db)
			if err := raids.Create(ctx, rd); err != nil {
				t.Fatal(err)
			}
			a := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: charID, CharacterName: "Actorchar", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
			if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
				t.Fatal(err)
			}
			want := raidseries.ErrLocalTimeNonexistent
			if scenario == "fold" {
				want = raidseries.ErrLocalTimeAmbiguous
			}
			if scenario == "invalid_series" {
				series.Timezone = "Unknown/Place"
				if err := seriesRepo.Save(ctx, series); err != nil {
					t.Fatal(err)
				}
				want = raidseries.ErrInvalid
			}
			if scenario == "occupied" {
				nextStart, err := raidseries.NextOccurrence(series, start, now)
				if err != nil {
					t.Fatal(err)
				}
				// Cancelled occurrences are skipped (ADR 078); a completed one still conflicts.
				existing := &raid.Raid{Name: "Existing occurrence", RaidLeaderID: actor, SeriesID: &series.ID, Status: validation.RaidStatusCompleted, CompletedAt: &now, ScheduledStart: &nextStart, CreatedAt: now, UpdatedAt: now}
				if err := raids.Create(ctx, existing); err != nil {
					t.Fatal(err)
				}
				want = raid.ErrOccurrenceConflict
			}
			svc := NewService(NewRepo(db))
			svc.now = func() time.Time { return now }
			archived, next, err := svc.ArchiveRaid(ctx, rd.ID, actor)
			if !errors.Is(err, want) || archived != nil || next != nil {
				t.Fatalf("want %v got %+v %+v %v", want, archived, next, err)
			}
			stored, err := raids.Get(ctx, rd.ID)
			if err != nil || stored.Status != validation.RaidStatusOpen || stored.CompletedAt != nil || stored.ArchivedAt != nil {
				t.Fatalf("scheduling failure mutated raid: %+v %v", stored, err)
			}
			balance, err := enhancepoints.NewRepo(db).Balance(ctx, actor)
			if err != nil || balance != 0 {
				t.Fatalf("scheduling failure wrote ledger: %d %v", balance, err)
			}
			next, err = raids.Successor(ctx, rd.ID)
			if err != nil || next != nil {
				t.Fatalf("scheduling failure wrote successor: %+v %v", next, err)
			}
		})
	}
}

func TestSuccessorLinkAndOccurrenceUniqueness(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Current", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	seedSchedule(t, ctx, db, rd)
	raids := raid.NewRepo(db)
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	_, next, err := NewService(NewRepo(db)).ArchiveRaid(ctx, rd.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := *next
	duplicate.ID = 0
	duplicate.PreviousRaidID = nil
	if err := raids.Create(ctx, &duplicate); !errors.Is(err, raid.ErrOccurrenceConflict) {
		t.Fatalf("duplicate occurrence accepted: %v", err)
	}
	duplicate = *next
	duplicate.ID = 0
	duplicate.SeriesID = nil
	duplicate.ScheduledStart = nil
	if err := raids.Create(ctx, &duplicate); !errors.Is(err, raid.ErrOccurrenceConflict) {
		t.Fatalf("duplicate successor link accepted: %v", err)
	}
}

func TestSuccessorUsesNameTemplate(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	charID := seedCharacter(t, ctx, character.NewRepo(db), actor, "Actorchar")
	raids := raid.NewRepo(db)
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "icc25-33", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	seedSchedule(t, ctx, db, rd)
	series, err := raidseries.NewRepo(db).Get(ctx, *rd.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	series.NameTemplate = "icc25-{number}"
	series.NextRaidNumber = 34
	if err := raidseries.NewRepo(db).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	a := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: charID, CharacterName: "Actorchar", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, IsBench: true, CreatedAt: now, UpdatedAt: now}
	if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	_, next, err := NewService(NewRepo(db)).ArchiveRaid(ctx, rd.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if next.Name != "icc25-34" {
		t.Fatalf("want successor name icc25-34, got %s", next.Name)
	}
	updated, err := raidseries.NewRepo(db).Get(ctx, *rd.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.NextRaidNumber != 35 {
		t.Fatalf("want next raid number 35, got %d", updated.NextRaidNumber)
	}
}

func TestArchiveWithoutPointsForDeclinedPlayers(t *testing.T) {
	ctx := context.Background()
	archive := func(t *testing.T, without []int64) (balances [2]int, err error) {
		db := setupDB(t)
		actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
		other := seedPlayer(t, ctx, player.NewRepo(db), "Other")
		now := time.Now().UTC()
		rd := &raid.Raid{Name: "Absence", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
		seedSchedule(t, ctx, db, rd)
		if err := raid.NewRepo(db).Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
		for _, p := range []int64{actor, other} {
			c := seedCharacter(t, ctx, character.NewRepo(db), p, "Char"+strconv.FormatInt(p, 10))
			a := &attendance.Attendance{RaidID: rd.ID, PlayerID: p, CharacterID: c, CharacterName: "Char", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
			if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
				t.Fatal(err)
			}
		}
		// Only "other" declined.
		if _, err := db.ExecContext(ctx, "INSERT INTO raid_absences (raid_id, player_id, reason, created_at) VALUES (?, ?, '', ?)", rd.ID, other, now); err != nil {
			t.Fatal(err)
		}
		translate := func(ids []int64) []int64 {
			out := []int64{}
			for _, id := range ids {
				if id == 1 {
					out = append(out, actor)
				} else {
					out = append(out, other)
				}
			}
			return out
		}
		_, _, err = NewService(NewRepo(db)).ArchiveRaidWithout(ctx, rd.ID, actor, translate(without))
		for i, p := range []int64{actor, other} {
			balances[i], _ = enhancepoints.NewRepo(db).Balance(ctx, p)
		}
		return balances, err
	}
	if b, err := archive(t, nil); err != nil || b != [2]int{10, 10} {
		t.Fatalf("default: %v %v", b, err)
	}
	if b, err := archive(t, []int64{2}); err != nil || b != [2]int{10, 0} {
		t.Fatalf("declined player excluded: %v %v", b, err)
	}
	if _, err := archive(t, []int64{1}); !errors.Is(err, ErrInvalidExclusion) {
		t.Fatalf("player who did not decline must be rejected: %v", err)
	}
}

func TestPreviewArchiveShowsAwardsAndMainSpecExclusion(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	other := seedPlayer(t, ctx, player.NewRepo(db), "Other")
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Preview", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	seedSchedule(t, ctx, db, rd)
	if err := raid.NewRepo(db).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int64{actor, other} {
		c := seedCharacter(t, ctx, character.NewRepo(db), p, "Char"+strconv.FormatInt(p, 10))
		a := &attendance.Attendance{RaidID: rd.ID, PlayerID: p, CharacterID: c, CharacterName: "Char", Class: "Krieger", Spec: "Waffen", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
		if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO enhance_point_transactions (player_id, amount, type, reason, ref_type, ref_id, actor_id, created_at) VALUES (?, 25, 'manual', 'x', 'manual', '1', ?, ?)", actor, actor, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO loot_awards (raid_id, boss, item_id, item_name, awarded_to_player_id, awarded_by_player_id, source, is_main_spec, awarded_at, created_at, updated_at) VALUES (?, 'B', '1', 'Schwert', ?, ?, 'manual', 1, ?, ?, ?)", rd.ID, other, actor, now, now, now); err != nil {
		t.Fatal(err)
	}
	rows, err := NewService(NewRepo(db)).PreviewArchive(ctx, rd.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %v", rows, err)
	}
	if rows[0] != (PreviewRow{PlayerID: actor, Balance: 25, Award: 10, Reason: PreviewAward}) {
		t.Fatalf("awarded row: %+v", rows[0])
	}
	if rows[1] != (PreviewRow{PlayerID: other, Balance: 0, Award: 0, Reason: PreviewMainSpecLoot}) {
		t.Fatalf("main-spec row: %+v", rows[1])
	}
}
