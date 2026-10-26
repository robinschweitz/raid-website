package poll

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

// Options and vote choices come back in their configured order, not in
// insertion order: each ORDER BY column must reach the SQL (FR-24.2, FR-24.7).
func TestOptionsFollowSortOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	bdb, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "poll.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bdb.Close() })
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	p := &player.Player{Name: "Voter", Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(bdb.DB).Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(bdb.DB)
	created := &Poll{Title: "Raidtag", PollType: "CHOICE", AllowMultiple: true, Status: validation.PollStatusOpen, CreatedBy: p.ID, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreatePollWithOptions(ctx, created, []string{"Montag", "Mittwoch", "Freitag"}); err != nil {
		t.Fatal(err)
	}
	// Reorder against insertion order: Freitag, Montag, Mittwoch.
	for label, sortOrder := range map[string]int{"Freitag": 1, "Montag": 2, "Mittwoch": 3} {
		if _, err := bdb.DB.NewUpdate().Model((*Option)(nil)).Set("sort_order = ?", sortOrder).Where("poll_id = ? AND label = ?", created.ID, label).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"Freitag", "Montag", "Mittwoch"}

	single, err := repo.ListOptions(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertLabels(t, "ListOptions", optionLabels(single), want)

	polls, err := repo.ListPolls(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(polls) != 1 {
		t.Fatalf("ListPolls returned %d polls, want 1", len(polls))
	}
	assertLabels(t, "ListPolls", optionLabels(polls[0].Options), want)

	ids := map[string]int64{}
	for _, opt := range single {
		ids[opt.Label] = opt.ID
	}
	vote := &Vote{PollID: created.ID, PlayerID: p.ID, Choice: "Mittwoch, Freitag", Choices: []string{"Mittwoch", "Freitag"}, VotedAt: now}
	if err := repo.UpsertVoteWithChoices(ctx, vote, map[string]int64{"Mittwoch": ids["Mittwoch"], "Freitag": ids["Freitag"]}, created.Title); err != nil {
		t.Fatal(err)
	}
	// Reorder the stored choices against insertion order: Freitag, Mittwoch.
	for label, sortOrder := range map[string]int{"Freitag": 1, "Mittwoch": 2} {
		if _, err := bdb.DB.NewUpdate().Model((*VoteChoice)(nil)).Set("sort_order = ?", sortOrder).Where("vote_id = ? AND label = ?", vote.ID, label).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	choices, err := repo.ListVoteChoices(ctx, vote.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertLabels(t, "ListVoteChoices", choices, []string{"Freitag", "Mittwoch"})

	votes, err := repo.ListVotes(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(votes) != 1 {
		t.Fatalf("ListVotes returned %d votes, want 1", len(votes))
	}
	assertLabels(t, "ListVotes", votes[0].Choices, []string{"Freitag", "Mittwoch"})
}

func optionLabels(opts []Option) []string {
	labels := make([]string, 0, len(opts))
	for _, opt := range opts {
		labels = append(labels, opt.Label)
	}
	return labels
}

func assertLabels(t *testing.T, source string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", source, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", source, got, want)
		}
	}
}
