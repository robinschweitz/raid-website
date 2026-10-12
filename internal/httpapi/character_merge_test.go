package httpapi_test

import (
	"context"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestMergeDuplicateMovesHistoricalPlayerReferencesWithCharacter(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	defer f.srv.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	formerOwnerID := seedAccount(t, f.db, "former-owner", "password", validation.RoleGuest)
	source := &character.Character{PlayerID: f.actorID, Name: "Typo", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(f.db).Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "ICC25-01", RaidLeaderID: f.actorID, Status: validation.RaidStatusCompleted, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(f.db).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	entry := &attendance.Attendance{RaidID: rd.ID, PlayerID: formerOwnerID, CharacterID: source.ID, CharacterName: source.Name, Class: source.Class, Spec: source.MainSpec, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
	if err := attendance.NewRepo(f.db).Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	award := &loot.Award{RaidID: rd.ID, Boss: "Boss", ItemID: "1", ItemName: "Item", AwardedToPlayerID: formerOwnerID, AwardedToCharacterID: &source.ID, AwardedToCharacterName: source.Name, AwardedByPlayerID: f.actorID, Source: "legacy_import", AwardMethod: loot.Main, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := loot.NewRepo(f.db).CreateAward(ctx, award); err != nil {
		t.Fatal(err)
	}

	if err := character.NewService(character.NewRepo(f.db)).MergeDuplicate(ctx, f.actorID, f.initial.ID, source.ID, "legacy typo"); err != nil {
		t.Fatal(err)
	}

	storedAttendance, err := attendance.NewRepo(f.db).Get(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedAward, err := loot.NewRepo(f.db).Get(ctx, award.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedAttendance.PlayerID != f.actorID || storedAttendance.CharacterID != f.initial.ID || storedAttendance.CharacterName != f.initial.Name {
		t.Fatalf("attendance not moved consistently: %+v", storedAttendance)
	}
	if storedAward.AwardedToPlayerID != f.actorID || storedAward.AwardedToCharacterID == nil || *storedAward.AwardedToCharacterID != f.initial.ID || storedAward.AwardedToCharacterName != f.initial.Name {
		t.Fatalf("loot not moved consistently: %+v", storedAward)
	}
	if _, err := character.NewRepo(f.db).Get(ctx, source.ID); err == nil {
		t.Fatal("source character was not deleted")
	}
}
