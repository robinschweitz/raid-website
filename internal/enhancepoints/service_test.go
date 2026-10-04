package enhancepoints

import "testing"

func TestComputeBalance(t *testing.T) {
	if got := ComputeBalance([]Transaction{{Amount: 10}, {Amount: -40}, {Amount: 5}}); got != -25 {
		t.Fatalf("want -25 got %d", got)
	}
}

func TestRollCost(t *testing.T) {
	for _, c := range []struct {
		balance, cost int
		ok            bool
	}{{-10, 0, false}, {0, 0, false}, {30, 30, true}, {50, 50, true}, {70, 50, true}, {150, 50, true}} {
		cost, err := RollCost(c.balance)
		if (err == nil) != c.ok || cost != c.cost {
			t.Fatalf("balance %d: cost %d error %v", c.balance, cost, err)
		}
	}
}

func TestRollRange(t *testing.T) {
	for _, c := range []struct{ cost, min, max int }{{-10, 1, 100}, {0, 1, 100}, {30, 31, 130}, {50, 51, 150}} {
		min, max := RollRange(c.cost)
		if min != c.min || max != c.max {
			t.Fatalf("cost %d: got %d-%d", c.cost, min, max)
		}
	}
}
