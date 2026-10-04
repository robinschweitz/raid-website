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
	"raid-website/internal/validation"
)

func TestArchiveRejectsIncompleteRosterBeforeWrites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*attendance.Attendance)
	}{
		{"name_missing", func(a *attendance.Attendance) { a.CharacterName = "" }},
		{"name_blank", func(a *attendance.Attendance) { a.CharacterName = " \t" }},
		{"class_missing", func(a *attendance.Attendance) { a.Class = "" }},
		{"class_invalid", func(a *attendance.Attendance) { a.Class = "Unknown" }},
		{"spec_missing", func(a *attendance.Attendance) { a.Spec = "" }},
		{"spec_blank", func(a *attendance.Attendance) { a.Spec = " \t" }},
		{"role_missing", func(a *attendance.Attendance) { a.Role = "" }},
		{"role_invalid", func(a *attendance.Attendance) { a.Role = "SUPPORT" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			database := setupDB(t)
			actor := seedPlayer(t, ctx, player.NewRepo(database), "Actor")
			characterID := seedCharacter(t, ctx, character.NewRepo(database), actor, "LiveCharacter")
			now := time.Now().UTC()
			rd := &raid.Raid{Name: "Current", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
			seedSchedule(t, ctx, database, rd)
			raids := raid.NewRepo(database)
			if err := raids.Create(ctx, rd); err != nil {
				t.Fatal(err)
			}
			row := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: characterID, CharacterName: "PlayedCharacter", Class: "Priester", Spec: "Raid-specific hybrid", Role: validation.RoleHeal, CreatedAt: now, UpdatedAt: now}
			tc.change(row)
			att := attendance.NewRepo(database)
			if err := att.Create(ctx, row); err != nil {
				t.Fatal(err)
			}
			before, err := att.Get(ctx, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			lock := &enhancepoints.ItemLock{PlayerID: actor, RaidID: rd.ID, ItemName: "Item", Status: validation.LockStatusActive, LockedAt: now, ActorID: actor, CreatedAt: now, UpdatedAt: now}
			if _, err := database.NewInsert().Model(lock).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			// A guard discovered only after successor creation would return this trigger error instead.
			if _, err := database.ExecContext(ctx, "CREATE TRIGGER fail_successor BEFORE INSERT ON raids BEGIN SELECT RAISE(ABORT, 'write before roster validation'); END"); err != nil {
				t.Fatal(err)
			}
			archived, next, err := NewService(NewRepo(database)).ArchiveRaid(ctx, rd.ID, actor)
			if !errors.Is(err, ErrRosterIncomplete) || archived != nil || next != nil {
				t.Fatalf("roster guard: %+v %+v %v", archived, next, err)
			}
			stored, err := raids.Get(ctx, rd.ID)
			if err != nil || stored.CompletedAt != nil || stored.ArchivedAt != nil || stored.Status != rd.Status {
				t.Fatalf("raid changed: %+v %v", stored, err)
			}
			after, err := att.Get(ctx, row.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("snapshot guessed from live character: %+v %v", after, err)
			}
			balance, err := enhancepoints.NewRepo(database).Balance(ctx, actor)
			if err != nil || balance != 0 {
				t.Fatalf("points written: %d %v", balance, err)
			}
			if err := database.NewSelect().Model(lock).WherePK().Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if lock.Status != validation.LockStatusActive || lock.ExpiredAt != nil {
				t.Fatalf("lock expired: %+v", lock)
			}
			next, err = raids.Successor(ctx, rd.ID)
			if err != nil || next != nil {
				t.Fatalf("successor written: %+v %v", next, err)
			}
		})
	}
}

func TestArchivePreservesActualRosterSnapshots(t *testing.T) {
	ctx := context.Background()
	database := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(database), "Actor")
	characterID := seedCharacter(t, ctx, character.NewRepo(database), actor, "LiveCharacter")
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Current", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	seedSchedule(t, ctx, database, rd)
	if err := raid.NewRepo(database).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	row := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: characterID, CharacterName: "PlayedCharacter", Class: " priest ", Spec: "Raid-specific hybrid", Role: " heal ", CreatedAt: now, UpdatedAt: now}
	att := attendance.NewRepo(database)
	if err := att.Create(ctx, row); err != nil {
		t.Fatal(err)
	}
	before, err := att.Get(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewService(NewRepo(database)).ArchiveRaid(ctx, rd.ID, actor); err != nil {
		t.Fatal(err)
	}
	after, err := att.Get(ctx, row.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("raid reality overwritten: %+v %v", after, err)
	}
}
