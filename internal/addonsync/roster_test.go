package addonsync

import (
	"testing"

	"raid-website/internal/character"
)

func TestKnownCharactersFindFoldsCaseAndPrefersExactMatch(t *testing.T) {
	known := knownCharacters{}
	for _, c := range []character.Character{{ID: 9, Name: "Shanna"}, {ID: 5, Name: "shanna"}, {ID: 7, Name: "Rüdiger"}} {
		known[toKey(c.Name)] = append(known[toKey(c.Name)], c)
	}
	for _, tc := range []struct {
		name string
		want int64
	}{{"Shanna", 9}, {"shanna", 5}, {"SHANNA", 5}, {"RÜDIGER", 7}, {"rüdiger", 7}} {
		if got := known.find(tc.name); got == nil || got.ID != tc.want {
			t.Fatalf("find(%q) = %+v, want id %d", tc.name, got, tc.want)
		}
	}
	if known.find("Weltchat") != nil {
		t.Fatal("unknown names must not match")
	}
}
