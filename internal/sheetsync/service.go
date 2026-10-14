package sheetsync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/legacyimport"
)

const Source = "google_sheets"

// Status is persisted so restarts retain the last good export fingerprint.
// A failure is visible to admins and never silently treated as a successful sync.
type Status struct {
	Enabled      bool                          `json:"enabled"`
	Automatic    bool                          `json:"automatic"`
	SheetID      string                        `json:"sheet_id"`
	LastRun      *time.Time                    `json:"last_run,omitempty"`
	LastSuccess  *time.Time                    `json:"last_success,omitempty"`
	LastError    string                        `json:"last_error,omitempty"`
	LastImport   *legacyimport.Report          `json:"last_import,omitempty"`
	CurrentSetup *legacyimport.SetupSyncReport `json:"current_setup,omitempty"`
	ExportHashes map[string]string             `json:"-"`
}

type Service struct {
	mu        sync.Mutex
	db        *bun.DB
	importer  *legacyimport.Service
	writer    MirrorWriter
	sheetID   string
	actorID   int64
	reason    string
	interval  time.Duration
	automatic bool
}

func New(db *bun.DB, importer *legacyimport.Service, writer MirrorWriter, sheetID string, actorID int64, reason string, interval time.Duration) (*Service, error) {
	if strings.TrimSpace(sheetID) != "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8" {
		return nil, errors.New("sync must target the explicitly authorized working copy")
	}
	if db == nil || importer == nil || writer == nil || strings.TrimSpace(sheetID) == "" || actorID <= 0 || strings.TrimSpace(reason) == "" || interval < time.Minute {
		return nil, errors.New("sheet sync needs a database, importer, writer, sheet ID, actor, reason and interval >= 1m")
	}
	return &Service{db: db, importer: importer, writer: writer, sheetID: sheetID, actorID: actorID, reason: reason, interval: interval}, nil
}

func (s *Service) GetStatus(ctx context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(ctx)
}

// RunOnce adopts Sheet archive outcomes and additive history atomically, then
// imports known current-raid participants and publishes website snapshots.
// Import conflicts prevent the export, preserving evidence for an administrator.
func (s *Service) RunOnce(ctx context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.load(ctx)
	if err != nil {
		return Status{}, err
	}
	now := time.Now().UTC()
	status.LastRun = &now
	status.LastError = ""
	// No import or export writes until the authorized working copy is verified.
	var backupVerifier, ok = s.writer.(interface {
		VerifyWorkingCopy(context.Context, string) error
	})
	var backupErr error
	if !ok {
		backupErr = errors.New("sheet writer must verify the backup before any sync")
	} else {
		backupErr = backupVerifier.VerifyWorkingCopy(ctx, s.sheetID)
	}
	// Detect an already edited mirror before committing any imported domain data.
	if backupErr == nil {
		if validator, ok := s.writer.(interface {
			Validate(context.Context, string, map[string][][]string, map[string]string) error
		}); ok {
			current, readErr := snapshots(ctx, s.db)
			if readErr != nil {
				backupErr = readErr
			} else {
				backupErr = validator.Validate(ctx, s.sheetID, current, status.ExportHashes)
			}
		}
	}
	if backupErr != nil {
		err = fmt.Errorf("working copy prerequisite: %w", backupErr)
	} else {
		merged, setup, reconcileErr := s.importer.Reconcile(ctx, s.actorID, Source, s.sheetID, s.reason, false)
		err = reconcileErr
		if err == nil {
			status.LastImport = &merged
			status.CurrentSetup = &setup
		}
	}
	if err == nil {
		var data map[string][][]string
		data, err = snapshots(ctx, s.db)
		if err == nil {
			var hashes map[string]string
			hashes, err = s.writer.Publish(ctx, s.sheetID, data, status.ExportHashes)
			if err == nil {
				status.ExportHashes = hashes
			}
		}
	}
	if err != nil {
		status.LastError = err.Error()
	} else {
		status.LastSuccess = &now
	}
	if saveErr := s.save(ctx, status); saveErr != nil {
		return status, fmt.Errorf("persist sync status: %w", saveErr)
	}
	return status, err
}

// Start runs at startup and then at the configured interval. Only one process
// should run against a SQLite database; RunOnce serializes HTTP and timer runs.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.automatic = true
	s.mu.Unlock()
	go func() {
		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
			defer cancel()
			if _, err := s.RunOnce(runCtx); err != nil {
				log.Printf("google sheet sync: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
