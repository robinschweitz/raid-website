// Package autosync runs the adopted Google Sheets synchronization on a fixed interval.
package autosync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"raid-website/internal/legacyimport"
	"raid-website/internal/logger"
	"raid-website/internal/player"
	"raid-website/internal/websheetsync"
)

const (
	GoogleSheetsSource = "google_sheets"
	AutomatedReason    = "Automatische Google-Sheets-Synchronisierung"
	Interval           = time.Minute
)

var ErrRunInProgress = errors.New("automatic sheet synchronization is already running")

type syncService interface {
	ListSyncSheetIDs(context.Context, string) ([]string, error)
	DryRunSync(context.Context, string, string) (legacyimport.Report, error)
	MergeSync(context.Context, int64, string, string, string, string) (legacyimport.Report, error)
}

type adminFinder interface {
	FindActiveAdmin(context.Context) (*player.Player, error)
}

type deliveryService interface {
	Flush(context.Context) (websheetsync.FlushResult, error)
}

// Service serializes scheduled synchronization attempts.
type Service struct {
	syncer   syncService
	admins   adminFinder
	delivery deliveryService
	mu       sync.Mutex
}

func NewService(syncer syncService, admins adminFinder, delivery deliveryService) *Service {
	return &Service{syncer: syncer, admins: admins, delivery: delivery}
}

// Start runs until ctx is cancelled. It intentionally does not block HTTP startup.
func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Run(ctx); err != nil && !errors.Is(err, ErrRunInProgress) {
					logger.Error("automatic Google Sheets synchronization failed err=%v", err)
				}
			}
		}
	}()
}

// Run synchronizes every distinct adopted Google Sheets sheet once.
func (s *Service) Run(ctx context.Context) error {
	if !s.mu.TryLock() {
		logger.Warn("automatic Google Sheets synchronization skipped: previous run is still active")
		return ErrRunInProgress
	}
	defer s.mu.Unlock()
	if s.delivery != nil {
		result, err := s.delivery.Flush(ctx)
		if err != nil {
			return fmt.Errorf("flush website changes: %w", err)
		}
		if result.Failed > 0 {
			return fmt.Errorf("flush website changes: %d delivery failures", result.Failed)
		}
	}

	actor, err := s.admins.FindActiveAdmin(ctx)
	if err != nil {
		return err
	}
	sheetIDs, err := s.syncer.ListSyncSheetIDs(ctx, GoogleSheetsSource)
	if err != nil {
		return err
	}
	if len(sheetIDs) == 0 {
		logger.Info("automatic Google Sheets synchronization skipped: no adopted sheets")
		return nil
	}
	for _, sheetID := range sheetIDs {
		preview, err := s.syncer.DryRunSync(ctx, GoogleSheetsSource, sheetID)
		if err != nil {
			logger.Error("automatic Google Sheets synchronization preview failed sheet=%s err=%v", sheetID, err)
			continue
		}
		if len(preview.Errors) > 0 {
			logger.Warn("automatic Google Sheets synchronization skipped sheet=%s preview_errors=%d", sheetID, len(preview.Errors))
			continue
		}
		merged, err := s.syncer.MergeSync(ctx, actor.ID, GoogleSheetsSource, sheetID, preview.RawHash, AutomatedReason)
		if err != nil {
			logger.Error("automatic Google Sheets synchronization merge failed sheet=%s err=%v", sheetID, err)
			continue
		}
		if len(merged.Errors) > 0 {
			logger.Warn("automatic Google Sheets synchronization skipped sheet=%s merge_errors=%d", sheetID, len(merged.Errors))
			continue
		}
		if merged.Skipped {
			logger.Info("automatic Google Sheets synchronization unchanged sheet=%s", sheetID)
			continue
		}
		logger.Info("automatic Google Sheets synchronization merged sheet=%s", sheetID)
	}
	return nil
}
