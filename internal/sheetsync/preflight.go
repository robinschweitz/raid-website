package sheetsync

import (
	"context"
	"fmt"
	"raid-website/internal/legacyimport"
)

// PreflightReport describes readiness without persisting any domain changes.
type PreflightReport struct {
	Ready    bool                         `json:"ready"`
	SheetID  string                       `json:"sheet_id"`
	Import   legacyimport.Report          `json:"import"`
	Setup    legacyimport.SetupSyncReport `json:"setup"`
	Blockers []string                     `json:"blockers"`
}

// Preflight uses the same transaction and archival checks as RunOnce, rolled back.
func (s *Service) Preflight(ctx context.Context) (PreflightReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := PreflightReport{SheetID: s.sheetID, Blockers: []string{}}
	verifier, ok := s.writer.(interface {
		VerifyWorkingCopy(context.Context, string) error
	})
	if !ok {
		report.Blockers = append(report.Blockers, "working copy verifier required")
		return report, nil
	}
	if err := verifier.VerifyWorkingCopy(ctx, s.sheetID); err != nil {
		report.Blockers = append(report.Blockers, err.Error())
		return report, nil
	}
	var err error
	report.Import, report.Setup, err = s.importer.Reconcile(ctx, s.actorID, Source, s.sheetID, s.reason, true)
	if err != nil {
		report.Blockers = append(report.Blockers, err.Error())
		return report, nil
	}
	status, err := s.load(ctx)
	if err != nil {
		return report, err
	}
	if validator, ok := s.writer.(interface {
		Validate(context.Context, string, map[string][][]string, map[string]string) error
	}); ok {
		data, err := snapshots(ctx, s.db)
		if err != nil {
			return report, err
		}
		if err = validator.Validate(ctx, s.sheetID, data, status.ExportHashes); err != nil {
			report.Blockers = append(report.Blockers, fmt.Sprintf("snapshot conflict: %v", err))
			return report, nil
		}
	}
	report.Ready = true
	return report, nil
}
