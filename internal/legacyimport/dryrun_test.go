package legacyimport

import (
	"strings"
	"testing"
)

const sampleCSV = "Klasse,Spec,,Boss,Item,ItemId,,Raid Buff,Anzahl,,Tanks,Heiler,,Current Raid Player,Tokens gesammelt\n" +
	"Magier,Feuer,,Der Lichkönig,Weiheabzeichen des Eroberers,52027,,3% Crit,4,,,Infamous,,Valily,2\n" +
	"Krieger,Waffen,,Fauldarm,Blutfall,50042,,,2,,,Dertotmacher,,Florian,0\n"

func TestParseAndDryRun(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader(sampleCSV))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 got %d", len(rows))
	}
	if rows[0].Item != "Weiheabzeichen des Eroberers" {
		t.Fatalf("item col misaligned: %q", rows[0].Item)
	}
	if rows[0].CurrentRaidPlayer != "Valily" {
		t.Fatalf("player col misaligned: %q", rows[0].CurrentRaidPlayer)
	}
	rep := BuildDryRun(rows)
	if rep.TokenItems != 1 {
		t.Fatalf("want 1 token got %d", rep.TokenItems)
	}
}
