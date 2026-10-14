package sheetsync

import (
	"reflect"
	"testing"
)

func TestRowsHashNormalizesGoogleSheetsTrailingEmptyCells(t *testing.T) {
	a := [][]string{{"ID", "Name", ""}, {"1", "Hero", ""}}
	b := [][]string{{"ID", "Name"}, {"1", "Hero"}}
	if rowsHash(a) != rowsHash(b) {
		t.Fatalf("trailing cells returned by Sheets API must not cause a false conflict")
	}
	if rowsHash(b) == rowsHash([][]string{{"ID", "Name"}, {"1", "Villain"}}) {
		t.Fatal("a real change must have a different hash")
	}
	before := [][]string{{"A", ""}, {"", ""}}
	after := normalizedRows(before)
	if !reflect.DeepEqual(after, [][]string{{"A"}}) {
		t.Fatalf("bad normalization: %#v", after)
	}
	if len(before[0]) != 2 {
		t.Fatal("normalization must not mutate source")
	}
}

func TestGoogleWriterRejectsInvalidCredentials(t *testing.T) {
	for _, raw := range []string{"", "{}", `{"client_email":"some@example.com","private_key":"invalid"}`} {
		if _, err := NewGoogleWriter(raw); err == nil {
			t.Fatalf("invalid credentials accepted: %q", raw)
		}
	}
}
