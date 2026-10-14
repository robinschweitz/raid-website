package googlesheets

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// TestLiveRead is deliberately opt-in because it contacts the guild's private
// test spreadsheet. It verifies that the configured service account has access.
func TestLiveRead(t *testing.T) {
	credentialFile := os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE")
	sheetID := os.Getenv("GOOGLE_SHEETS_TEST_ID")
	if credentialFile == "" || sheetID == "" {
		t.Skip("GOOGLE_SERVICE_ACCOUNT_FILE and GOOGLE_SHEETS_TEST_ID are required")
	}
	client, err := NewFromFile(credentialFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, tab := range []string{"Raid Archive", "Loot Archive"} {
		values, err := client.Values(context.Background(), sheetID, "'"+tab+"'")
		if err != nil {
			t.Fatalf("read %s: %v", tab, err)
		}
		if len(values) == 0 {
			t.Fatalf("%s has no header row", tab)
		}
		t.Logf("%s headers: %s", tab, strings.Join(values[0], " | "))
	}
}

// TestLiveSetupSyncColumns establishes the website-owned columns once for the
// private integration sheet. It is opt-in because it mutates that test sheet.
func TestLiveSetupSyncColumns(t *testing.T) {
	if os.Getenv("GOOGLE_SHEETS_SETUP") != "1" {
		t.Skip("GOOGLE_SHEETS_SETUP=1 is required")
	}
	credentialFile := os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE")
	sheetID := os.Getenv("GOOGLE_SHEETS_TEST_ID")
	if credentialFile == "" || sheetID == "" {
		t.Fatal("GOOGLE_SERVICE_ACCOUNT_FILE and GOOGLE_SHEETS_TEST_ID are required")
	}
	client, err := NewFromFile(credentialFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.UpdateValues(context.Background(), sheetID, "'Loot Archive'!G1:K1", [][]string{{"Import ID", "Website Award ID", "Website Sync ID", "Website Updated At", "Website Sync Status"}}); err != nil {
		t.Fatal(err)
	}
}

// TestLiveSetupAuditTab establishes the append-only audit tab once for the
// private integration sheet. It is opt-in because it mutates that test sheet.
func TestLiveSetupAuditTab(t *testing.T) {
	if os.Getenv("GOOGLE_SHEETS_SETUP") != "1" {
		t.Skip("GOOGLE_SHEETS_SETUP=1 is required")
	}
	credentialFile := os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE")
	sheetID := os.Getenv("GOOGLE_SHEETS_TEST_ID")
	if credentialFile == "" || sheetID == "" {
		t.Fatal("GOOGLE_SERVICE_ACCOUNT_FILE and GOOGLE_SHEETS_TEST_ID are required")
	}
	client, err := NewFromFile(credentialFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.AddSheet(context.Background(), sheetID, "Website Sync Audit"); err != nil && !errors.Is(err, ErrSheetExists) {
		t.Fatal(err)
	}
	if err := client.UpdateValues(context.Background(), sheetID, "'Website Sync Audit'!A1:F1", [][]string{{"Website Sync ID", "Website Award ID", "Action", "Status", "Website Updated At", "Detail"}}); err != nil {
		t.Fatal(err)
	}
}
