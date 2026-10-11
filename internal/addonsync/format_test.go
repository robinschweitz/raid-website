package addonsync

import (
	"errors"
	"testing"
	"time"

	"raid-website/internal/validation"
)

func TestBuildInviteExport(t *testing.T) {
	size := 25
	got := BuildInviteExport(7, "ICC25 | Donnerstag", &size, []InviteMember{
		{Name: "Shanna", Role: validation.RoleHeal, Class: "Priester"},
		{Name: "Thorgrim", Role: validation.RoleTank, Class: "Krieger", IsBench: true},
	})
	want := "RS1|INV|7|ICC25 / Donnerstag|25\nShanna|HEAL|Priester|0\nThorgrim|TANK|Krieger|1"
	if got != want {
		t.Fatalf("export:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseLootExport(t *testing.T) {
	text := "\r\n RS1|LOOT|7|Rüdiger|1760000000\r\n1760000100|36612|50415|Shanna\n\n1760000200|-|50274|-\nkaputt\n1760000300|x|50415|Shanna\n"
	export, err := ParseLootExport(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if export.RaidID != 7 || export.Recorder != "Rüdiger" || !export.StartedAt.Equal(time.Unix(1760000000, 0)) {
		t.Fatalf("header: %+v", export)
	}
	if len(export.Lines) != 4 {
		t.Fatalf("lines: %+v", export.Lines)
	}
	first := export.Lines[0]
	if first.NPCID != "36612" || first.ItemID != "50415" || first.RecipientName != "Shanna" || first.Error != "" || !first.LootedAt.Equal(time.Unix(1760000100, 0)) {
		t.Fatalf("first line: %+v", first)
	}
	if second := export.Lines[1]; second.NPCID != "" || second.RecipientName != "" || second.Error != "" {
		t.Fatalf("unassigned line: %+v", second)
	}
	if export.Lines[2].Error == "" || export.Lines[3].Error == "" {
		t.Fatalf("broken lines must be reported: %+v", export.Lines[2:])
	}
}

func TestParseLootExportRejectsOtherText(t *testing.T) {
	for _, text := range []string{"", "   \n", "RS1|INV|7|Raid|25", "RS2|LOOT|7|A|1", "RS1|LOOT|x|A|1", "Shanna:BD-AAAA"} {
		if _, err := ParseLootExport(text); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("%q: expected ErrInvalidPayload, got %v", text, err)
		}
	}
}

func TestLineKeysAreStableAndCountDuplicates(t *testing.T) {
	export, err := ParseLootExport("RS1|LOOT|7|Rüdiger|1\n5|-|50415|Shanna\n5|-|50415|Shanna\n6|-|50274|Shanna")
	if err != nil {
		t.Fatal(err)
	}
	keys := lineKeys(7, export)
	again := lineKeys(7, export)
	if keys[0] == keys[1] || keys[0] == keys[2] || keys[1] == keys[2] {
		t.Fatalf("keys must differ: %v", keys)
	}
	for i := range keys {
		if keys[i] != again[i] {
			t.Fatalf("keys must be stable: %v %v", keys, again)
		}
	}
	if other := lineKeys(8, export); other[0] == keys[0] {
		t.Fatal("keys must depend on the raid")
	}
}
