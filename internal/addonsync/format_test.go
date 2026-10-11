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
	text := "\r\n RS1|LOOT|7|Rüdiger|1760000000\r\n1|1760000100|36612|50415|Shanna|given\n\n2|1760000200|-|50274|-|-\n3|1760000250|-|50274|Rüdiger|held\nkaputt\n5|1760000300|x|50415|Shanna|given\n1|1760000400|-|50415|Shanna|manual\n7|1760000500|-|50415|-|given\n8|1760000600|-|50415|Shanna|lost\n9|1760000700|-|50415|Shanna\n"
	export, err := ParseLootExport(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if export.RaidID != 7 || export.Recorder != "Rüdiger" || !export.StartedAt.Equal(time.Unix(1760000000, 0)) {
		t.Fatalf("header: %+v", export)
	}
	if len(export.Lines) != 9 {
		t.Fatalf("lines: %+v", export.Lines)
	}
	first := export.Lines[0]
	if first.EntryID != "1" || first.NPCID != "36612" || first.ItemID != "50415" || first.RecipientName != "Shanna" || first.State != LootGiven || first.Error != "" || !first.LootedAt.Equal(time.Unix(1760000100, 0)) {
		t.Fatalf("first line: %+v", first)
	}
	if second := export.Lines[1]; second.NPCID != "" || second.RecipientName != "" || second.State != "" || second.Error != "" {
		t.Fatalf("unassigned line: %+v", second)
	}
	if held := export.Lines[2]; held.State != LootHeld || held.Error != "" {
		t.Fatalf("held line: %+v", held)
	}
	// broken text, bad NPC, duplicate entry ID, state without recipient, unknown state, old four-column line
	for _, broken := range export.Lines[3:] {
		if broken.Error == "" {
			t.Fatalf("broken line must be reported: %+v", broken)
		}
	}
}

func TestParseLootExportRejectsOtherText(t *testing.T) {
	for _, text := range []string{"", "   \n", "RS1|INV|7|Raid|25", "RS1|ROSTER|7|A|1", "RS2|LOOT|7|A|1", "RS1|LOOT|x|A|1", "Shanna:BD-AAAA"} {
		if _, err := ParseLootExport(text); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("%q: expected ErrInvalidPayload, got %v", text, err)
		}
	}
}

func TestLineKeysFollowTheEntryNotTheRecipient(t *testing.T) {
	parse := func(text string) *LootExport {
		t.Helper()
		export, err := ParseLootExport(text)
		if err != nil {
			t.Fatal(err)
		}
		return export
	}
	held := parse("RS1|LOOT|7|Rüdiger|1\n5|10|-|50415|Rüdiger|held\n6|10|-|50415|Rüdiger|held\nkaputt\nkaputt")
	traded := parse("RS1|LOOT|7|Rüdiger|1\n5|10|-|50415|Shanna|given")
	keys := lineKeys(7, held)
	if keys[0] == keys[1] || keys[2] == keys[3] {
		t.Fatalf("keys must differ per entry and per broken line: %v", keys)
	}
	if lineKeys(7, traded)[0] != keys[0] {
		t.Fatal("a trade must not change the key of the drop")
	}
	if other := lineKeys(8, held); other[0] == keys[0] {
		t.Fatal("keys must depend on the raid")
	}
	if restarted := lineKeys(7, parse("RS1|LOOT|7|Rüdiger|2\n5|10|-|50415|Rüdiger|held")); restarted[0] == keys[0] {
		t.Fatal("a new recording must not reuse keys of an earlier one")
	}
}

func TestParseRosterExport(t *testing.T) {
	export, err := ParseRosterExport("RS1|ROSTER|7|Rüdiger|1760007200\nShanna|PRIEST|1760000000\nWeltchat|-|1760003600\n|MAGE|1\nkaputt\nZeit|MAGE|x")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if export.RaidID != 7 || export.Recorder != "Rüdiger" || !export.ExportedAt.Equal(time.Unix(1760007200, 0)) || len(export.Members) != 5 {
		t.Fatalf("export: %+v", export)
	}
	if m := export.Members[0]; m.Name != "Shanna" || m.ClassToken != "PRIEST" || !m.FirstSeen.Equal(time.Unix(1760000000, 0)) || m.Error != "" {
		t.Fatalf("member: %+v", m)
	}
	if m := export.Members[1]; m.ClassToken != "" || m.Error != "" {
		t.Fatalf("member without class: %+v", m)
	}
	for _, broken := range export.Members[2:] {
		if broken.Error == "" {
			t.Fatalf("broken line must be reported: %+v", broken)
		}
	}
	for _, text := range []string{"", "RS1|LOOT|7|A|1", "RS1|ROSTER|x|A|1", "RS1|ROSTER|7|A"} {
		if _, err := ParseRosterExport(text); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("%q: expected ErrInvalidPayload, got %v", text, err)
		}
	}
}
