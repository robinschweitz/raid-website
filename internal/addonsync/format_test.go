package addonsync

import (
	"errors"
	"strings"
	"testing"
	"time"

	"raid-website/internal/validation"
)

func TestBuildInviteExport(t *testing.T) {
	size := 25
	got := BuildInviteExport(7, "ICC25 | Donnerstag; Gruppe 2", &size, nil, []InviteMember{
		{Name: "Shanna", Role: validation.RoleHeal, Class: "Priester"},
		{Name: "Thorgrim", Role: validation.RoleTank, Class: "Krieger", IsBench: true},
	})
	want := "RS2;INV;7;ICC25 / Donnerstag, Gruppe 2;25\nShanna;HEAL;Priester;0\nThorgrim;TANK;Krieger;1"
	if got != want {
		t.Fatalf("export:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildInviteExportWithRealmStart(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.November, 5, 19, 0, 0, 0, time.UTC).In(berlin)
	got := BuildInviteExport(7, "ICC25", nil, &start, []InviteMember{{Name: "Shanna", Role: validation.RoleHeal, Class: "Priester"}})
	want := "RS2;INV;7;ICC25;;2026-11-05;20:00\nShanna;HEAL;Priester;0"
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
	v2, err := ParseRosterExport("RS2;ROSTER;7;Rüdiger;1760007200\nShanna;PRIEST;1760000000\nWeltchat;-;1760003600\nkaputt|x|1")
	if err != nil || v2.RaidID != 7 || len(v2.Members) != 3 || v2.Members[0].Name != "Shanna" || v2.Members[0].ClassToken != "PRIEST" || v2.Members[1].ClassToken != "" || v2.Members[2].Error == "" {
		t.Fatalf("RS2 roster: %+v %v", v2, err)
	}
	for _, text := range []string{"", "RS1|LOOT|7|A|1", "RS1|ROSTER|x|A|1", "RS1|ROSTER|7|A", "RS2;ROSTER;7;A", "RS2;ROSTER;7;A;1;2", "RS2;INV;7;A;1"} {
		if _, err := ParseRosterExport(text); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("%q: expected ErrInvalidPayload, got %v", text, err)
		}
	}
}

func TestParseLootExportV2(t *testing.T) {
	text := strings.Join([]string{
		"RS2;LOOT;7;Rüdiger;1760000000;1760009000",
		"1;1760000100;36612;50415;Shanna;given;87;ms;1-100;Shanna",
		"2;1760000200;-;50274;Rüdiger;held;131;ms;31-130;Thorgrim",
		"3;1760000250;-;50274;Rüdiger;held;-;-;-;-",
		"4;1760000300;-;50415;Thorgrim;parked;-;-;-;-",
		"5;1760000400;-;50415;Shanna;deleted;44;os;1-99;Shanna",
		"6;1760000500;-;50415;Shanna;manual;-;-;-;-",
		"7;1760000600;-;-;Shanna;manual;-;-;-;-",
		"8;1760000700;-;50415;Shanna;lost;-;-;-;-",
		"9;1760000800;-;50415;-;given;-;-;-;-",
		"10;1760000900;-;50415;Shanna;given;151;ms;51-150;Shanna",
		"11;1760001000;-;50415;Shanna;given;90;ds;1-100;Shanna",
		"12;1760001100;-;50415;Shanna;given;90;ms;100-1;Shanna",
		"13;1760001200;-;50415;Shanna;given;90;ms;1-100",
		"14;1760001300;-;50415;Rüdiger;disenchanted;-;-;-;-",
		"15;1760001400;-;50415;Bankchar;bank;-;-;-;-",
		"16;1760001500;-;50415;-;bank;-;-;-;-",
	}, "\n")
	export, err := ParseLootExport(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if export.Version != "RS2" || export.RaidID != 7 || export.Recorder != "Rüdiger" || !export.StartedAt.Equal(time.Unix(1760000000, 0)) || len(export.Lines) != 16 {
		t.Fatalf("export: %+v", export)
	}
	if l := export.Lines[0]; l.Error != "" || l.State != LootGiven || l.Roll != 87 || l.Spec != SpecMain || l.RollLow != 1 || l.RollHigh != 100 || l.WinnerName != "Shanna" || l.EnhancedPoints() != 0 {
		t.Fatalf("given line: %+v", l)
	}
	if l := export.Lines[1]; l.Error != "" || l.State != LootHeld || l.WinnerName != "Thorgrim" || l.EnhancedPoints() != 30 {
		t.Fatalf("held enhanced line: %+v", l)
	}
	if l := export.Lines[2]; l.Error != "" || l.Roll != 0 || l.Spec != "" || l.WinnerName != "" {
		t.Fatalf("open held line: %+v", l)
	}
	if l := export.Lines[3]; l.Error != "" || l.State != LootParked || l.RecipientName != "Thorgrim" {
		t.Fatalf("parked line: %+v", l)
	}
	if l := export.Lines[4]; l.Error != "" || l.State != LootDeleted || l.Spec != SpecOff || l.EnhancedPoints() != 0 {
		t.Fatalf("deleted line: %+v", l)
	}
	if l := export.Lines[5]; l.Error != "" || l.State != LootManual {
		t.Fatalf("manual line: %+v", l)
	}
	if l := export.Lines[13]; l.Error != "" || l.State != LootDisenchanted || l.RecipientName != "Rüdiger" || l.State.Importable() {
		t.Fatalf("disenchanted line: %+v", l)
	}
	if l := export.Lines[14]; l.Error != "" || l.State != LootBank || l.State.Importable() {
		t.Fatalf("bank line: %+v", l)
	}
	// no item, unknown state, missing recipient, roll above 150, unknown spec, reversed range, nine fields, bank without holder
	for _, broken := range append(export.Lines[6:13:13], export.Lines[15]) {
		if broken.Error == "" {
			t.Fatalf("broken line must be reported: %+v", broken)
		}
	}
	for _, text := range []string{"RS2;LOOT;7;A;1", "RS2;LOOT;7;A;1;x", "RS2;ROSTER;7;A;1;2", "RS2;LOOT;x;A;1;2"} {
		if _, err := ParseLootExport(text); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("%q: expected ErrInvalidPayload, got %v", text, err)
		}
	}
	if _, err := ParseRosterExport("RS2;LOOT;7;A;1;2"); !errors.Is(err, ErrInvalidPayload) {
		t.Fatal("RS2 is only a loot format")
	}
}

func TestEnhancedPointsFollowTheMinBalanceRule(t *testing.T) {
	for _, tc := range []struct{ low, high, want int }{
		{1, 100, 0}, {1, 99, 0}, {2, 101, 1}, {31, 130, 30}, {51, 150, 50}, {52, 151, 0}, {31, 129, 0},
	} {
		if got := (LootLine{RollLow: tc.low, RollHigh: tc.high}).EnhancedPoints(); got != tc.want {
			t.Fatalf("%d-%d: %d, want %d", tc.low, tc.high, got, tc.want)
		}
	}
}
