package legacyimport

import (
    "context"
    "strings"
    "testing"
)

const setupSyncCSV = `"Position","Charakter","Spieler","Klasse","Spec","Rolle"
"1","Nylama","Nylama","Todesritter","Blut (Tank)","Tank"
`

func TestSyncCurrentSetupRejectsSourceChangeWithoutWrites(t *testing.T) {
    _, service, _, _, _, _ := setup(t)
    data := sampleSheet()
    data.Setup = []byte(setupSyncCSV)
    service.fetcher = fakeFetcher{data: data}

    report, err := service.SyncCurrentSetup(context.Background(), 1, "google_sheets", "test", "different-hash")
    if err != nil { t.Fatal(err) }
    if report.Added != 0 || len(report.Conflicts) != 1 || !strings.Contains(report.Conflicts[0], "verändert") {
        t.Fatalf("expected source-change conflict without writes, got %+v", report)
    }
}

func TestSyncCurrentSetupNeverBootstrapsMissingSeries(t *testing.T) {
    _, service, _, _, _, _ := setup(t)
    data := sampleSheet()
    data.Setup = []byte(setupSyncCSV)
    service.fetcher = fakeFetcher{data: data}

    report, err := service.SyncCurrentSetup(context.Background(), 1, "google_sheets", "test", data.Hash)
    if err != nil { t.Fatal(err) }
    if report.Added != 0 || report.RaidID != 0 || len(report.Warnings) == 0 {
        t.Fatalf("current setup must require an explicit first import, got %+v", report)
    }
}

func TestSyncCurrentSetupRejectsMalformedRows(t *testing.T) {
    _, service, _, _, _, _ := setup(t)
    data := sampleSheet()
    data.Setup = []byte(`"Position","Charakter","Spieler","Klasse","Spec","Rolle"
"1","Nylama","","Todesritter","Blut (Tank)","Tank"
`)
    service.fetcher = fakeFetcher{data: data}
    report, err := service.SyncCurrentSetup(context.Background(), 1, "google_sheets", "test", data.Hash)
    if err != nil { t.Fatal(err) }
    if report.Added != 0 || len(report.Conflicts) == 0 {
        t.Fatalf("missing player must reject entire setup, got %+v", report)
    }
}
