package raidcorrection

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/loot"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

type removeFixture struct {
	r     *Repo
	s     *Service
	actor *player.Player
	raid  *raid.Raid
	rows  []*attendance.Attendance
}

func newRemoveFixture(t *testing.T, status validation.RaidStatus) removeFixture {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "remove.db")+"?_pragma=foreign_keys(1)")
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
	f := removeFixture{r: NewRepo(database.DB)}
	f.s = NewService(f.r)
	now := time.Now().UTC()
	f.actor = &player.Player{Name: "Leader", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := f.r.players.Create(ctx, f.actor); err != nil {
		t.Fatal(err)
	}
	f.raid = &raid.Raid{Name: "ICC", RaidLeaderID: f.actor.ID, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := f.r.raids.Create(ctx, f.raid); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Keeper", "Leaver"} {
		c := &character.Character{PlayerID: f.actor.ID, Name: name, Class: "Magier", MainSpec: "Feuer", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := f.r.characters.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		row := &attendance.Attendance{RaidID: f.raid.ID, PlayerID: f.actor.ID, CharacterID: c.ID, Role: validation.RoleDD}
		if err := f.s.SaveAttendance(ctx, row, f.actor.ID, ""); err != nil {
			t.Fatal(err)
		}
		f.rows = append(f.rows, row)
	}
	if status != validation.RaidStatusOpen {
		f.raid.Status = status
		if err := f.r.raids.Update(ctx, f.raid); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f removeFixture) addLoot(t *testing.T, characterID int64, withdrawn bool) {
	t.Helper()
	now := time.Now().UTC()
	award := &loot.Award{RaidID: f.raid.ID, Boss: "Boss", ItemID: "1", ItemName: "Schwert", AwardedToPlayerID: f.actor.ID, AwardedToCharacterID: &characterID, AwardedByPlayerID: f.actor.ID, Source: "manual", AwardMethod: loot.Main, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	if withdrawn {
		award.WithdrawnAt = &now
	}
	if err := f.r.loot.CreateAward(context.Background(), award); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveAttendanceHardDeletesRowFromOpenRaidWithAudit(t *testing.T) {
	ctx := context.Background()
	f := newRemoveFixture(t, validation.RaidStatusOpen)
	leaver := f.rows[1]
	// A quick-add idempotency record points at the row by foreign key.
	if err := f.r.saveQuickAddRequest(ctx, f.raid.ID, f.actor.ID, "req-1", "{}", leaver.ID); err != nil {
		t.Fatal(err)
	}
	f.addLoot(t, leaver.CharacterID, true) // withdrawn loot does not block
	if err := f.s.RemoveAttendance(ctx, f.raid.ID, leaver.ID, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.r.attendance.Get(ctx, leaver.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("row still present: %v", err)
	}
	rows, err := f.r.attendance.ListByRaid(ctx, f.raid.ID)
	if err != nil || len(rows) != 1 || rows[0].ID != f.rows[0].ID {
		t.Fatalf("other rows touched: %+v %v", rows, err)
	}
	var count int
	if err := f.r.db.NewRaw("SELECT COUNT(*) FROM audit_log WHERE entity_type = 'attendance' AND action = 'remove' AND entity_id = ?", leaver.ID).Scan(ctx, &count); err != nil || count != 1 {
		t.Fatalf("audit entries = %d, %v", count, err)
	}
	if err := f.s.RemoveAttendance(ctx, f.raid.ID, leaver.ID, f.actor.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second removal: %v", err)
	}
}

func TestRemoveAttendanceRejectsLootHoldersWrongRaidAndClosedRaids(t *testing.T) {
	ctx := context.Background()
	f := newRemoveFixture(t, validation.RaidStatusPlanned)
	f.addLoot(t, f.rows[0].CharacterID, false)
	if err := f.s.RemoveAttendance(ctx, f.raid.ID, f.rows[0].ID, f.actor.ID); !errors.Is(err, ErrHasLoot) {
		t.Fatalf("loot holder: %v", err)
	}
	if err := f.s.RemoveAttendance(ctx, f.raid.ID+1, f.rows[1].ID, f.actor.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong raid: %v", err)
	}
	for _, status := range []validation.RaidStatus{validation.RaidStatusCompleted, validation.RaidStatusCancelled} {
		closed := newRemoveFixture(t, status)
		if err := closed.s.RemoveAttendance(ctx, closed.raid.ID, closed.rows[1].ID, closed.actor.ID); !errors.Is(err, ErrRaidState) {
			t.Fatalf("%s: %v", status, err)
		}
	}
	archived := newRemoveFixture(t, validation.RaidStatusOpen)
	now := time.Now().UTC()
	archived.raid.ArchivedAt = &now
	if err := archived.r.raids.Update(ctx, archived.raid); err != nil {
		t.Fatal(err)
	}
	if err := archived.s.RemoveAttendance(ctx, archived.raid.ID, archived.rows[1].ID, archived.actor.ID); !errors.Is(err, ErrRaidState) {
		t.Fatalf("archived: %v", err)
	}
}
