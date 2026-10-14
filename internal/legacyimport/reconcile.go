package legacyimport

import (
	"context"
	"fmt"
	"strings"
)

type snapshotFetcher struct {
	sheetID string
	data    SheetData
}

func (f snapshotFetcher) Fetch(_ context.Context, sheetID string) (SheetData, error) {
	if sheetID != f.sheetID {
		return SheetData{}, fmt.Errorf("snapshot sheet mismatch")
	}
	return f.data, nil
}

// Reconcile evaluates one fetched source version and applies domain changes in
// one transaction. Preview rolls back the same path, including import receipts.
func (s *Service) Reconcile(ctx context.Context, actorID int64, source, sheetID, reason string, previewOnly bool, guard func(Report) error) (Report, SetupSyncReport, error) {
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return Report{}, SetupSyncReport{}, err
	}
	var report Report
	var setup SetupSyncReport
	err = s.transaction(ctx, !previewOnly, func(bound *Service) error {
		bound.fetcher = snapshotFetcher{sheetID, data}
		bound.strictSource = true
		bound.external = nil
		preview, err := bound.DryRun(ctx, source, sheetID)
		report = preview
		if err != nil {
			return err
		}
		if len(preview.Errors) > 0 {
			return fmt.Errorf("historical sheet conflicts: %s", strings.Join(preview.Errors, "; "))
		}
		if err = guard(preview); err != nil {
			return err
		}
		report, err = bound.Merge(ctx, actorID, source, sheetID, preview.RawHash, reason)
		if err != nil {
			return err
		}
		if len(report.Errors) > 0 {
			return fmt.Errorf("historical sheet merge conflicts: %s", strings.Join(report.Errors, "; "))
		}
		setup, err = bound.SyncCurrentSetup(ctx, actorID, source, sheetID, data.Hash)
		if err != nil {
			return err
		}
		if len(setup.Conflicts) > 0 {
			return fmt.Errorf("current raid setup conflicts: %s", strings.Join(setup.Conflicts, "; "))
		}
		if report.Skipped {
			report.Created = Counts{}
			report.ArchivedRaids = 0
		}
		return nil
	})
	return report, setup, err
}
