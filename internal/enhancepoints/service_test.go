package enhancepoints

import "testing"

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
