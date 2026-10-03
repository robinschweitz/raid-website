package enhancepoints

import (
	"testing"

	"raid-website/internal/validation"
)

func TestShouldAwardNoMainSpec(t *testing.T) {
	if ShouldAwardNoMainSpec(RaidState{Status: validation.RaidStatusCompleted}, true) != true {
		t.Fatal("want true")
	}
	if ShouldAwardNoMainSpec(RaidState{Status: validation.RaidStatusCompleted}, false) != false {
		t.Fatal("want false")
	}
	if ShouldAwardNoMainSpec(RaidState{Status: validation.RaidStatusCancelled}, true) != false {
		t.Fatal("cancelled -> no")
	}
	if ShouldAwardNoMainSpec(RaidState{Status: validation.RaidStatusOpen}, true) != false {
		t.Fatal("open -> no")
	}
}
