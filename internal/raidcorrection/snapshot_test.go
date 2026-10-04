package raidcorrection

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestRosterSnapshotCorrectionDefaultsValidationAndArchive(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "snapshot.db")+"?_pragma=foreign_keys(1)")
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
	s := NewService(r)
	now := time.Now().UTC()
	p := &player.Player{Name: "Actor", Role: validation.RoleAdmin, CreatedAt: now, UpdatedAt: now}
	if err := r.players.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &character.Character{PlayerID: p.ID, Name: "Before", Class: "Magier", MainSpec: "Feuer", CreatedAt: now, UpdatedAt: now}
	if err := r.characters.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "Planned", RaidLeaderID: p.ID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := r.raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	row := &attendance.Attendance{RaidID: rd.ID, PlayerID: p.ID, CharacterID: c.ID, Role: validation.RoleDD}
	if err := s.SaveAttendance(ctx, row, p.ID, ""); err != nil {
		t.Fatal(err)
	}
	if row.CharacterName != c.Name || row.Class != c.Class || row.Spec != c.MainSpec {
		t.Fatalf("defaults: %+v", row)
	}
	c.Name, c.MainSpec = "After", "Arkan"
	if err := r.characters.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	row, err = s.CorrectAttendance(ctx, rd.ID, row.ID, p.ID, "note only", func(a *attendance.Attendance) { a.Notes = "note" })
	if err != nil || row.CharacterName != "Before" || row.Spec != "Feuer" {
		t.Fatalf("live changes leaked: %+v %v", row, err)
	}
	playedSpec := "Actual hybrid build"
	row, err = s.CorrectAttendance(ctx, rd.ID, row.ID, p.ID, "used spec", nil, SnapshotPatch{Spec: &playedSpec})
	if err != nil || row.Spec != playedSpec {
		t.Fatalf("spec patch: %+v %v", row, err)
	}
	baseline, err := r.attendance.Get(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, patch := range []SnapshotPatch{
		{CharacterName: stringPointer(" ")}, {Class: stringPointer("unknown")}, {Spec: stringPointer("")},
	} {
		if _, err := s.CorrectAttendance(ctx, rd.ID, row.ID, p.ID, "invalid", nil, patch); !errors.Is(err, ErrInvalid) {
			t.Fatalf("typed error: %v", err)
		}
	}
	stored, err := r.attendance.Get(ctx, row.ID)
	if err != nil || !reflect.DeepEqual(stored, baseline) {
		t.Fatalf("invalid edit persisted: %+v %v", stored, err)
	}
	if _, err := s.CorrectAttendance(ctx, rd.ID, row.ID, p.ID, " ", nil, SnapshotPatch{Spec: &playedSpec}); !errors.Is(err, ErrReason) {
		t.Fatalf("reason: %v", err)
	}
	other := &character.Character{PlayerID: p.ID, Name: "Alternate", Class: "Priester", MainSpec: "Disziplin", CreatedAt: now, UpdatedAt: now}
	if err := r.characters.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	row, err = s.CorrectAttendance(ctx, rd.ID, row.ID, p.ID, "switch character", func(a *attendance.Attendance) { a.CharacterID = other.ID })
	if err != nil || row.CharacterName != other.Name || row.Class != other.Class || row.Spec != other.MainSpec {
		t.Fatalf("reference refresh: %+v %v", row, err)
	}
	rd.Status = validation.RaidStatusCompleted
	if err := r.raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CorrectAttendance(ctx, rd.ID, row.ID, p.ID, "historical metadata only", nil, SnapshotPatch{Spec: &playedSpec}); err != nil {
		t.Fatal(err)
	}
	if balance, err := r.points.Balance(ctx, p.ID); err != nil || balance != 0 {
		t.Fatalf("metadata awarded points: %d %v", balance, err)
	}
	rd.ArchivedAt = &now
	if err := r.raids.Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CorrectAttendance(ctx, rd.ID, row.ID, p.ID, "archived", nil, SnapshotPatch{Spec: &playedSpec}); !errors.Is(err, ErrRaidState) {
		t.Fatalf("archive: %v", err)
	}
}

func stringPointer(value string) *string { return &value }
