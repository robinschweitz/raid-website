package legacyimport

import (
	"context"
	"strings"
	"testing"
)

func TestContinuousSourceChangesAreConflicts(t *testing.T) {
	for _, change := range []string{"points", "character_spec", "character_removed", "historical_roster", "historical_removed"} {
		t.Run(change, func(t *testing.T) {
			s, actor := importFixture(t)
			ctx := context.Background()
			data := sampleSheet()
			s.fetcher = fakeFetcher{data: data}
			preview := previewImport(t, s, "source", "sheet")
			if _, err := s.Merge(ctx, actor, "source", "sheet", preview.RawHash, "initial import"); err != nil {
				t.Fatal(err)
			}
			s.strictSource = true
			switch change {
			case "points":
				data.Points = []byte(strings.ReplaceAll(string(data.Points), "170", "190"))
			case "character_spec":
				data.Character = []byte(strings.ReplaceAll(string(data.Character), "Disziplin", "Schatten"))
			case "character_removed":
				data.Character = []byte("Charakter,Spieler,Klasse,Main Spec,Second Spec\nNylama,Nylama,Todesritter,,\n")
			case "historical_roster":
				data.Raid = []byte(strings.ReplaceAll(string(data.Raid), "Disziplin", "Schatten"))
			case "historical_removed":
				data.Raid = []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\n")
			}
			data.Hash = "changed-" + change
			s.fetcher = fakeFetcher{data: data}
			report, err := s.DryRun(ctx, "source", "sheet")
			if err != nil || len(report.Errors) == 0 || report.Created != (Counts{}) {
				t.Fatalf("source edit must be blocked: %+v %v", report, err)
			}
		})
	}
}
