package raidcorrection

import (
	"context"
	"errors"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

// ADR 092: a player has at most one character in a raid's roster, bench included.
func TestSaveAttendanceRejectsSecondCharacterOfSamePlayer(t *testing.T) {
	ctx := context.Background()
	f := newRemoveFixture(t, validation.RaidStatusOpen)
	keeper := f.rows[0]
	now := time.Now().UTC()
	twink := &character.Character{PlayerID: keeper.PlayerID, Name: "Keepertwink", Class: "Priester", MainSpec: "Heilig", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := f.r.characters.Create(ctx, twink); err != nil {
		t.Fatal(err)
	}

	for _, bench := range []bool{false, true} {
		row := &attendance.Attendance{RaidID: f.raid.ID, PlayerID: keeper.PlayerID, CharacterID: twink.ID, Role: validation.RoleHeal, IsBench: bench}
		err := f.s.SaveAttendance(ctx, row, f.actor.ID, "")
		var inRoster *PlayerInRosterError
		if !errors.Is(err, ErrPlayerInRoster) || !errors.As(err, &inRoster) {
			t.Fatalf("bench=%v: got %v, want ErrPlayerInRoster", bench, err)
		}
		if inRoster.CharacterName != "Keeper" || inRoster.AttendanceID != keeper.ID {
			t.Fatalf("bench=%v: error names %+v, want the Keeper slot", bench, inRoster)
		}
	}

	// Switching the character inside the existing slot stays possible.
	switched, err := f.s.CorrectAttendance(ctx, f.raid.ID, keeper.ID, f.actor.ID, "", func(a *attendance.Attendance) { a.CharacterID = twink.ID })
	if err != nil {
		t.Fatalf("switch within slot: %v", err)
	}
	if switched.CharacterName != "Keepertwink" {
		t.Fatalf("switched snapshot = %q", switched.CharacterName)
	}

	// Moving another slot onto a player who already has one is rejected.
	leaver := f.rows[1]
	leaverCharacter, err := f.r.characters.Get(ctx, leaver.CharacterID)
	if err != nil {
		t.Fatal(err)
	}
	leaverCharacter.PlayerID = keeper.PlayerID
	if err := f.r.characters.Update(ctx, leaverCharacter); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CorrectAttendance(ctx, f.raid.ID, leaver.ID, f.actor.ID, "", func(a *attendance.Attendance) { a.PlayerID = keeper.PlayerID })
	if !errors.Is(err, ErrPlayerInRoster) {
		t.Fatalf("move onto occupied player: %v", err)
	}
}

// Rows that predate the rule (legacy imports, account merges) stay correctable.
func TestSaveAttendanceKeepsCorrectingExistingDuplicateSlots(t *testing.T) {
	ctx := context.Background()
	f := newRemoveFixture(t, validation.RaidStatusOpen)
	now := time.Now().UTC()
	owner := &player.Player{Name: "Legacy", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := f.r.players.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	var legacy []*attendance.Attendance
	for _, name := range []string{"Altmain", "Alttwink"} {
		c := &character.Character{PlayerID: owner.ID, Name: name, Class: "Krieger", MainSpec: "Furor", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := f.r.characters.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		row := &attendance.Attendance{RaidID: f.raid.ID, PlayerID: owner.ID, CharacterID: c.ID, CharacterName: name, Class: "Krieger", Spec: "Furor", Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
		if err := f.r.attendance.Create(ctx, row); err != nil {
			t.Fatal(err)
		}
		legacy = append(legacy, row)
	}
	if _, err := f.s.CorrectAttendance(ctx, f.raid.ID, legacy[1].ID, f.actor.ID, "", func(a *attendance.Attendance) { a.Role = validation.RoleTank }); err != nil {
		t.Fatalf("correct legacy duplicate: %v", err)
	}
}
