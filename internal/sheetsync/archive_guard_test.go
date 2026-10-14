package sheetsync

import (
	"strings"
	"testing"

	"raid-website/internal/legacyimport"
)

func TestEnsureWebsiteOnlyArchival(t *testing.T) {
	cases := []struct {
		name    string
		preview legacyimport.Report
		want    string
	}{
		{name: "existing raids can sync", preview: legacyimport.Report{}},
		{
			name:    "new characters and loot can sync",
			preview: legacyimport.Report{Created: legacyimport.Counts{Characters: 2, Loot: 3, Points: 1}},
		},
		{
			name:    "new archived raid from sheet is forbidden",
			preview: legacyimport.Report{Created: legacyimport.Counts{Raids: 1}},
			want:    "website",
		},
		{
			name:    "reclassifying existing completed raid is forbidden",
			preview: legacyimport.Report{ArchivedRaids: 1},
			want: "website",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ensureWebsiteOnlyArchival(tc.preview)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected guard failure: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected guard error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}
