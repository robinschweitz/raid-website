package enhancepoints

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
)

func TestConcurrentRollsDoNotOverspend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bdb, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "roll.db")+"?_pragma=foreign_keys(1)")
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
	p := &player.Player{Name: "Roller", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(bdb.DB).Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(bdb.DB)
	if err := repo.InsertTransaction(ctx, &Transaction{PlayerID: p.ID, Amount: 70, Type: "adjustment", Reason: "test balance", ActorID: p.ID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan RollResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.Roll(ctx, p.ID, p.ID)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	costs := map[int]int{}
	for result := range results {
		costs[result.Cost]++
		wantBalance := 0
		if result.Cost == 50 {
			wantBalance = 20
		}
		if result.Min != 1 || result.Max != 100+result.Cost || result.Roll < result.Min || result.Roll > result.Max || result.BalanceAfter != wantBalance {
			t.Errorf("invalid roll result: %+v", result)
		}
	}
	if costs[50] != 1 || costs[20] != 1 || len(costs) != 2 {
		t.Fatalf("want one cost 50 and one cost 20, got %v", costs)
	}
	if len(errs) != 6 {
		t.Fatalf("want six rejected rolls, got %d", len(errs))
	}
	for err := range errs {
		if !errors.Is(err, ErrInsufficientBalance) {
			t.Errorf("unexpected roll error: %v", err)
		}
	}
	balance, err := repo.Balance(ctx, p.ID)
	if err != nil || balance != 0 {
		t.Fatalf("balance: want 0, got %d, err %v", balance, err)
	}
	transactions, err := repo.ListByPlayer(ctx, p.ID)
	if err != nil || len(transactions) != 3 {
		t.Fatalf("want seed and two debits, got %v, err %v", transactions, err)
	}
	for _, transaction := range transactions[1:] {
		if transaction.Type != "enhance_roll" || transaction.ActorID != p.ID || transaction.Amount >= 0 {
			t.Errorf("invalid debit: %+v", transaction)
		}
	}
}

func TestComputeBalance(t *testing.T) {
	tx := []Transaction{{Amount: 10}, {Amount: -40}, {Amount: 5}}
	if got := ComputeBalance(tx); got != -25 {
		t.Fatalf("want -25 got %d", got)
	}
}

func TestRollCost(t *testing.T) {
	cases := []struct {
		bal int
		x   int
		ok  bool
	}{
		{0, 0, false},
		{40, 40, true},
		{50, 50, true},
		{70, 50, true},
		{150, 50, true},
	}
	for i, c := range cases {
		x, err := RollCost(c.bal)
		if (err == nil) != c.ok {
			t.Fatalf("%d: ok=%t err=%v", i, c.ok, err)
		}
		if err == nil && x != c.x {
			t.Fatalf("%d: want %d got %d", i, c.x, x)
		}
	}
}

func TestRollRange(t *testing.T) {
	min, max := RollRange(40)
	if min != 1 || max != 140 {
		t.Fatalf("want 1-140 got %d-%d", min, max)
	}
}
