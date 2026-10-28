package validation

import "testing"

func TestNormalizeClassNameWarlockAliases(t *testing.T) {
	// "Hexenmeister" is the official German client name; the guild data uses "Hexer".
	for _, value := range []string{"Hexer", "hexer", "Hexenmeister", " hexenmeister ", "Warlock"} {
		if got := NormalizeClassName(value); got != "Hexer" {
			t.Errorf("NormalizeClassName(%q) = %q, want Hexer", value, got)
		}
	}
	if _, ok := ClassSpecs[NormalizeClassName("Hexenmeister")]; !ok {
		t.Error("Hexenmeister must resolve to a class with specs")
	}
}
