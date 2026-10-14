package sheetsync

import (
	"strings"
	"testing"

	"raid-website/internal/legacyimport"
)

func TestEnsureWebsiteOnlyArchival(t *testing.T) {
	allowed := []legacyimport.Report{
		{},
		{Created: legacyimport.Counts{Characters: 2, Loot: 3, Points: 1}},
	}
	for _, preview := range allowed {
		if err := ensureWebsiteOnlyArchival(preview); err != nil {
			t.Fatalf("unexpected archive guard failure: %v", err)
		}
	}
	blocked := []legacyimport.Report{
		{Created: legacyimport.Counts{Raids: 1}},
		{ArchivedRaids: 1},
	}
	for _, preview := range blocked {
		err := ensureWebsiteOnlyArchival(preview)
		if err == nil || !strings.Contains(err.Error(), "website") {
			t.Fatalf("expected website-only archival error, got: %v", err)
		}
	}
}
