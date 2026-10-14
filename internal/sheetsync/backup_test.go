package sheetsync

import "testing"

func TestMatchingBackupTabs(t *testing.T) {
	tabs := []sheetTab{
		{"Raid Archive", 1}, {"Loot Archive", 2}, {"Charaktere", 3},
		{"Punkte", 4}, {"ICC25 Raidsetup", 5}, {"ICC25", 6},
	}
	if err := matchingBackupTabs(append(append([]sheetTab{}, tabs...), sheetTab{"Website Raids", 7}), tabs); err != nil {
		t.Fatalf("new export tabs on original must not invalidate older backup: %v", err)
	}
	changed := append([]sheetTab{}, tabs...)
	changed[1].ID = 8
	if err := matchingBackupTabs(tabs, changed); err == nil {
		t.Fatal("replaced original tab must not pass backup check")
	}
	if err := matchingBackupTabs(tabs, tabs[:4]); err == nil {
		t.Fatal("incomplete copy must not pass backup check")
	}
}

func TestBackupIDCannotBeOriginal(t *testing.T) {
	g := &GoogleWriter{}
	if err := g.VerifyBackup(t.Context(), "same-id", "same-id"); err == nil {
		t.Fatal("backup must use a distinct spreadsheet")
	}
}
