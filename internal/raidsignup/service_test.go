package raidsignup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestSignupWorkflow(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "signup.db")+"?_pragma=foreign_keys(1)")
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
	s := NewService(r)
	now := time.Date(2030, 1, 1, 18, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	staff := &player.Player{Name: "Staff", Role: validation.RoleRaidassistent, Active: true, CreatedAt: now, UpdatedAt: now}
	member := &player.Player{Name: "Member", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	for _, p := range []*player.Player{staff, member} {
		if err := r.players.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	var chars []*character.Character
	for _, name := range []string{"First", "Second", "Third"} {
		c := &character.Character{PlayerID: member.ID, Name: name, Class: "Paladin", MainSpec: "Vergeltung", SecondSpec: "Heilig", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := r.characters.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		chars = append(chars, c)
	}
	foreign := &character.Character{PlayerID: staff.ID, Name: "Foreign", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := r.characters.Create(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	start := now.Add(time.Hour)
	rd := &raid.Raid{Name: "Future", RaidLeaderID: staff.ID, Status: validation.RaidStatusPlanned, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
	if err := r.raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	wantError := func(err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	}
	overview := func() *Overview { t.Helper(); v, err := s.Overview(ctx, rd.ID); require(err); return v }
	choices := []Choice{{chars[0].ID, "Schutz (Tank)"}, {chars[1].ID, "Heilig"}, {chars[2].ID, "Vergeltung"}}

	if v := overview(); v.IsOpen || v.Entries == nil || len(v.Entries) != 0 {
		t.Fatalf("initial overview: %+v", v)
	}
	wantError(s.Save(ctx, rd.ID, member.ID, choices), ErrClosed)
	wantError(s.SetEnabled(ctx, rd.ID, member.ID, true), ErrForbidden)
	require(s.SetEnabled(ctx, rd.ID, staff.ID, true))
	wantError(s.Save(ctx, rd.ID, member.ID, []Choice{choices[0], {foreign.ID, "Frost"}}), ErrForbidden)
	if len(overview().Entries) != 0 {
		t.Fatal("partial bulk submission persisted")
	}
	wantError(s.Save(ctx, rd.ID, member.ID, []Choice{{chars[0].ID, "Frost"}}), ErrInvalid)
	wantError(s.Save(ctx, rd.ID, member.ID, []Choice{choices[0], choices[0]}), ErrInvalid)
	require(s.Save(ctx, rd.ID, member.ID, choices))
	entries := overview().Entries
	if len(entries) != 3 {
		t.Fatal(entries)
	}
	c, err := r.characters.Get(ctx, chars[0].ID)
	require(err)
	if c.MainSpec != "Vergeltung" {
		t.Fatal("saved main spec changed")
	}

	wantError(s.Accept(ctx, rd.ID, entries[0].ID, member.ID, 0, 0), ErrForbidden)
	require(s.Accept(ctx, rd.ID, entries[0].ID, staff.ID, 0, 0))
	require(s.Accept(ctx, rd.ID, entries[0].ID, staff.ID, 0, 0))
	roster, err := r.attendance.ListByRaid(ctx, rd.ID)
	require(err)
	if len(roster) != 1 || roster[0].Spec != "Schutz (Tank)" || roster[0].Role != validation.RoleTank || roster[0].CharacterName != "First" {
		t.Fatalf("roster: %+v", roster)
	}
	wantError(s.Save(ctx, rd.ID, member.ID, []Choice{{chars[0].ID, "Heilig"}}), ErrConflict)
	wantError(s.Withdraw(ctx, rd.ID, entries[0].ID, member.ID), ErrConflict)
	wantError(s.Withdraw(ctx, rd.ID, entries[1].ID, staff.ID), ErrForbidden)
	wantError(s.Accept(ctx, rd.ID, entries[1].ID, staff.ID, 0, 0), ErrConflict)
	require(s.Accept(ctx, rd.ID, entries[1].ID, staff.ID, roster[0].ID, chars[0].ID))
	wantError(s.Accept(ctx, rd.ID, entries[2].ID, staff.ID, roster[0].ID, chars[0].ID), ErrConflict)
	roster, err = r.attendance.ListByRaid(ctx, rd.ID)
	require(err)
	if len(roster) != 1 || roster[0].CharacterID != chars[1].ID || roster[0].Role != validation.RoleHeal {
		t.Fatalf("switched roster: %+v", roster)
	}
	if v := overview(); v.Entries[0].IsAccepted || !v.Entries[1].IsAccepted {
		t.Fatalf("acceptance: %+v", v)
	}
	require(s.Withdraw(ctx, rd.ID, entries[0].ID, member.ID))
	wantError(s.Accept(ctx, rd.ID, entries[0].ID, staff.ID, roster[0].ID, chars[1].ID), ErrConflict)
	require(s.Save(ctx, rd.ID, member.ID, []Choice{choices[0]}))
	if len(overview().Entries) != 3 {
		t.Fatal("resubmission duplicated a row")
	}

	require(s.SetEnabled(ctx, rd.ID, staff.ID, false))
	wantError(s.Save(ctx, rd.ID, member.ID, []Choice{choices[0]}), ErrClosed)
	require(s.SetEnabled(ctx, rd.ID, staff.ID, true))
	now = start
	if overview().IsOpen {
		t.Fatal("deadline not enforced at exact start")
	}
	wantError(s.Save(ctx, rd.ID, member.ID, []Choice{choices[0]}), ErrClosed)
	wantError(s.Withdraw(ctx, rd.ID, entries[0].ID, member.ID), ErrClosed)
	wantError(s.SetEnabled(ctx, rd.ID, staff.ID, true), ErrClosed)
	require(s.Accept(ctx, rd.ID, entries[0].ID, staff.ID, roster[0].ID, chars[1].ID))

	// Audit failure must roll back the roster switch as well.
	_, err = database.DB.ExecContext(ctx, "CREATE TRIGGER fail_signup_audit BEFORE INSERT ON audit_log WHEN NEW.entity_type = 'raid_signup' BEGIN SELECT RAISE(ABORT, 'audit failure'); END")
	require(err)
	if err := s.Accept(ctx, rd.ID, entries[2].ID, staff.ID, roster[0].ID, chars[0].ID); err == nil {
		t.Fatal("expected audit failure")
	}
	roster, err = r.attendance.ListByRaid(ctx, rd.ID)
	require(err)
	if roster[0].CharacterID != chars[0].ID {
		t.Fatal("failed audit left roster changed")
	}
	_, err = database.DB.ExecContext(ctx, "DROP TRIGGER fail_signup_audit")
	require(err)
	for _, status := range []validation.RaidStatus{validation.RaidStatusCancelled, validation.RaidStatusCompleted} {
		rd.Status = status
		require(r.raids.Update(ctx, rd))
		wantError(s.Accept(ctx, rd.ID, entries[2].ID, staff.ID, roster[0].ID, chars[0].ID), ErrRaidState)
		wantError(s.SetEnabled(ctx, rd.ID, staff.ID, false), ErrRaidState)
	}
}

func TestSubmitForPlayerIsStaffOnlyAndKeepsPendingChoices(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "submit.db")+"?_pragma=foreign_keys(1)")
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
	s := NewService(r)
	now := time.Now().UTC()
	staff := &player.Player{Name: "Staff", Role: validation.RoleRaidassistent, Active: true, CreatedAt: now, UpdatedAt: now}
	member := &player.Player{Name: "Member", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	for _, p := range []*player.Player{staff, member} {
		if err := r.players.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	c := &character.Character{PlayerID: member.ID, Name: "Kalender", Class: "Paladin", MainSpec: "Vergeltung", SecondSpec: "Heilig", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := r.characters.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "Calendar", RaidLeaderID: staff.ID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := r.raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitForPlayer(ctx, rd.ID, member.ID, c.ID, "Vergeltung"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member: %v", err)
	}
	if err := s.SubmitForPlayer(ctx, rd.ID, staff.ID, c.ID, "Frost"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("spec of another class: %v", err)
	}
	// Signups need not be open: staff signs up from the game calendar.
	if err := s.SubmitForPlayer(ctx, rd.ID, staff.ID, c.ID, "Vergeltung"); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitForPlayer(ctx, rd.ID, staff.ID, c.ID, "Heilig"); !errors.Is(err, ErrAlreadySignedUp) {
		t.Fatalf("pending choice overwritten: %v", err)
	}
	overview, err := s.Overview(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Entries) != 1 || overview.Entries[0].Spec != "Vergeltung" || overview.Entries[0].PlayerID != member.ID {
		t.Fatalf("entries = %+v", overview.Entries)
	}
}

// ADR 092: a calendar acceptance removes the player's absence in the same
// transaction as the signup, also after the raid has started.
func TestSubmitForPlayerRemovesAbsence(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "submit-absence.db")+"?_pragma=foreign_keys(1)")
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
	s := NewService(r)
	now := time.Now().UTC()
	staff := &player.Player{Name: "Staff", Role: validation.RoleRaidassistent, Active: true, CreatedAt: now, UpdatedAt: now}
	member := &player.Player{Name: "Member", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	for _, p := range []*player.Player{staff, member} {
		if err := r.players.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	c := &character.Character{PlayerID: member.ID, Name: "Kalender", Class: "Paladin", MainSpec: "Vergeltung", SecondSpec: "Heilig", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := r.characters.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	started := now.Add(-time.Hour)
	rd := &raid.Raid{Name: "Calendar", RaidLeaderID: staff.ID, Status: validation.RaidStatusOpen, ScheduledStart: &started, CreatedAt: now, UpdatedAt: now}
	if err := r.raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	if err := s.Decline(ctx, rd.ID, staff.ID, member.ID, "Urlaub", AbsenceDeclined); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitForPlayer(ctx, rd.ID, staff.ID, c.ID, "Vergeltung"); err != nil {
		t.Fatal(err)
	}
	overview, err := s.Overview(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Absences) != 0 || len(overview.Entries) != 1 {
		t.Fatalf("absences = %+v, entries = %+v", overview.Absences, overview.Entries)
	}
	var revokes int
	if err := database.DB.NewRaw("SELECT COUNT(*) FROM audit_log WHERE entity_type = 'raid_signup' AND action = 'absence_revoke'").Scan(ctx, &revokes); err != nil || revokes != 1 {
		t.Fatalf("absence_revoke audits = %d, %v", revokes, err)
	}
	// An unchanged signup leaves a newer absence alone.
	if err := s.Decline(ctx, rd.ID, staff.ID, member.ID, "", AbsenceTentative); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitForPlayer(ctx, rd.ID, staff.ID, c.ID, "Vergeltung"); !errors.Is(err, ErrAlreadySignedUp) {
		t.Fatalf("second submit: %v", err)
	}
	overview, err = s.Overview(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Absences) != 1 || overview.Absences[0].Status != AbsenceTentative {
		t.Fatalf("absence removed without a signup: %+v", overview.Absences)
	}
}

// ADR 093: "kann nicht" takes the player out of the raid: roster rows (bench
// included) are removed and open signups withdrawn. "Unsicher" changes nothing.
func TestDeclineRemovesRosterSlotAndWithdrawsSignups(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "decline.db")+"?_pragma=foreign_keys(1)")
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
	s := NewService(r)
	now := time.Now().UTC()
	staff := &player.Player{Name: "Staff", Role: validation.RoleRaidassistent, Active: true, CreatedAt: now, UpdatedAt: now}
	member := &player.Player{Name: "Member", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	for _, p := range []*player.Player{staff, member} {
		if err := r.players.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	main := &character.Character{PlayerID: member.ID, Name: "Hauptfigur", Class: "Paladin", MainSpec: "Vergeltung", SecondSpec: "Heilig", IsMain: true, Active: true, CreatedAt: now, UpdatedAt: now}
	twink := &character.Character{PlayerID: member.ID, Name: "Zweitfigur", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	for _, c := range []*character.Character{main, twink} {
		if err := r.characters.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	rd := &raid.Raid{Name: "Decline", RaidLeaderID: staff.ID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := r.raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitForPlayer(ctx, rd.ID, staff.ID, main.ID, "Vergeltung"); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitForPlayer(ctx, rd.ID, staff.ID, twink.ID, "Feuer"); err != nil {
		t.Fatal(err)
	}
	overview, err := s.Overview(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range overview.Entries {
		if entry.CharacterID == main.ID {
			if err := s.Accept(ctx, rd.ID, entry.ID, staff.ID, 0, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	rosterSize := func() int {
		rows, err := r.attendance.ListByRaid(ctx, rd.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(rows)
	}
	if rosterSize() != 1 {
		t.Fatal("accepted main character missing from the roster")
	}
	// "Unsicher" keeps roster slot and signups.
	if err := s.Decline(ctx, rd.ID, member.ID, 0, "Spätschicht", AbsenceTentative); err != nil {
		t.Fatal(err)
	}
	if rosterSize() != 1 {
		t.Fatal("unsure answer removed the roster slot")
	}
	// The player's own refusal takes them out of the raid.
	if err := s.Decline(ctx, rd.ID, member.ID, 0, "Spätschicht", AbsenceDeclined); err != nil {
		t.Fatal(err)
	}
	if rosterSize() != 0 {
		t.Fatal("refusal kept the roster slot")
	}
	overview, err = s.Overview(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range overview.Entries {
		if !entry.IsWithdrawn || entry.IsAccepted {
			t.Fatalf("signup still open after the refusal: %+v", entry)
		}
	}
	if len(overview.Absences) != 1 || overview.Absences[0].Status != AbsenceDeclined || overview.Absences[0].Reason != "Spätschicht" {
		t.Fatalf("absences = %+v", overview.Absences)
	}
	var removals, withdrawals int
	if err := database.DB.NewRaw("SELECT COUNT(*) FROM audit_log WHERE entity_type = 'attendance' AND action = 'remove'").Scan(ctx, &removals); err != nil || removals != 1 {
		t.Fatalf("attendance remove audits = %d, %v", removals, err)
	}
	if err := database.DB.NewRaw("SELECT COUNT(*) FROM audit_log WHERE entity_type = 'raid_signup' AND action = 'withdraw'").Scan(ctx, &withdrawals); err != nil || withdrawals != 2 {
		t.Fatalf("withdraw audits = %d, %v", withdrawals, err)
	}
}
