package sourceexport

import (
	"raid-website/internal/lootcatalog"
	"testing"
)

func TestEncounterGroupingPreservesSourcesAndRejectsConflicts(t *testing.T) {
	fixture := func() lootcatalog.Dataset {
		return lootcatalog.Dataset{Bosses: []lootcatalog.Boss{{ID: "1", InstanceID: "631", Name: "Captain"}, {ID: "2", InstanceID: "631", Name: "Other captain"}}, Drops: []lootcatalog.Drop{{BossID: "1", ItemID: "3", Difficulty: "source-creature:1"}, {BossID: "2", ItemID: "3", Difficulty: "source-creature:2"}}}
	}
	d := fixture()
	e := Encounter{ID: "encounter:631:gunship", InstanceID: "631", Name: "Luftschiffkampf", Participants: []string{"1", "2"}}
	if err := groupEncounters(&d, []Encounter{e}); err != nil {
		t.Fatal(err)
	}
	if len(d.Bosses) != 1 || len(d.Drops) != 2 || d.Drops[0].BossID != e.ID || d.Drops[1].BossID != e.ID || d.Drops[0].Difficulty == d.Drops[1].Difficulty {
		t.Fatalf("lost encounter/source identity: %+v", d)
	}
	d = fixture()
	other := e
	other.ID = "encounter:631:other"
	if err := groupEncounters(&d, []Encounter{e, other}); err == nil {
		t.Fatal("duplicate ownership accepted")
	}
	d = fixture()
	e.InstanceID = "603"
	e.ID = "encounter:603:wrong"
	if err := groupEncounters(&d, []Encounter{e}); err == nil {
		t.Fatal("cross-instance ownership accepted")
	}
}

func TestClientSpellArchiveChecksumRequired(t *testing.T) {
	err := verifyChestSpells([]Root{{SpellID: "71207"}}, func(string) (string, error) { return "untrusted archive", nil })
	if err == nil {
		t.Fatal("unverified client data accepted")
	}
}

func TestExplicitSpellColumns(t *testing.T) {
	var got map[string]string
	err := Parse("INSERT INTO `spell_dbc` (`ID`, `Effect_1`, `EffectMiscValue_1`) VALUES (71207,76,201959);", "spell_dbc", func(row map[string]string) error { got = row; return nil })
	if err != nil || got["id"] != "71207" || got["effectmiscvalue_1"] != "201959" {
		t.Fatalf("spell row: %v %v", got, err)
	}
	for _, sql := range []string{"INSERT INTO `spell_dbc` (`ID`,`ID`) VALUES (1,2);", "INSERT INTO `spell_dbc` (`ID`) VALUES (1,2);"} {
		if err := Parse(sql, "spell_dbc", func(map[string]string) error { return nil }); err == nil {
			t.Fatal("malformed row accepted")
		}
	}
}
