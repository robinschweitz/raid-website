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
func (s *Service) Reconcile(ctx context.Context, actorID int64, source, sheetID, reason string, previewOnly bool) (Report, SetupSyncReport, error) {
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
		if err := bound.prepareSheetArchives(ctx, actorID, source, sheetID, reason, data); err != nil {
			return err
		}
		preview, err := bound.DryRun(ctx, source, sheetID)
		report = preview
		if err != nil {
			return err
		}
		if len(preview.Errors) > 0 {
			return fmt.Errorf("historical sheet conflicts: %s", strings.Join(preview.Errors, "; "))
		}
		report, err = bound.Merge(ctx, actorID, source, sheetID, preview.RawHash, reason)
		if err != nil {
			return err
		}
		if len(report.Errors) > 0 {
			return fmt.Errorf("historical sheet merge conflicts: %s", strings.Join(report.Errors, "; "))
		}
		if report.Skipped {
			report.Created = Counts{}
			report.ArchivedRaids = 0
		}
		if err = bound.describeSheetArchives(ctx, &report); err != nil {
			return err
		}
		if err = bound.applySheetArchives(ctx, actorID, source, sheetID, reason, &report); err != nil {
			return err
		}
		if len(bound.sheetArchives.raids) > 0 {
			report.Skipped = false
		}
		if err = bound.imports.saveSourceBaseline(ctx, source, sheetID, data); err != nil {
			return err
		}
		if err = bound.savePointBaselines(ctx, source, sheetID, data); err != nil {
			return err
		}
		setup, err = bound.SyncCurrentSetup(ctx, actorID, source, sheetID, data.Hash)
		if err != nil {
			return err
		}
		if len(setup.Conflicts) > 0 {
			return fmt.Errorf("current raid setup conflicts: %s", strings.Join(setup.Conflicts, "; "))
		}
		return bound.recordSheetArchiveReport(ctx, actorID, sheetID, reason, report)
	})
	return report, setup, err
}
