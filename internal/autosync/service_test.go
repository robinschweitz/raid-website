package autosync

import (
	"context"
	"errors"
	"sync"
	"testing"

	"raid-website/internal/legacyimport"
	"raid-website/internal/player"
	"raid-website/internal/validation"
	"raid-website/internal/websheetsync"
)

type fakeSyncer struct {
	sheets      []string
	listErr     error
	previews    map[string]legacyimport.Report
	previewErrs map[string]error
	mergeErrs   map[string]error
	merges      []mergeCall
	block       <-chan struct{}
	started     chan struct{}
	startOnce   sync.Once
	mu          sync.Mutex
}

type mergeCall struct {
	actorID               int64
	sheetID, hash, reason string
}

func (f *fakeSyncer) ListSyncSheetIDs(_ context.Context, _ string) ([]string, error) {
	return f.sheets, f.listErr
}

func (f *fakeSyncer) DryRunSync(_ context.Context, _ string, sheetID string) (legacyimport.Report, error) {
	if f.started != nil {
		f.startOnce.Do(func() { close(f.started) })
	}
	if f.block != nil {
		<-f.block
	}
	return f.previews[sheetID], f.previewErrs[sheetID]
}

func (f *fakeSyncer) MergeSync(_ context.Context, actorID int64, _ string, sheetID, hash, reason string) (legacyimport.Report, error) {
	f.mu.Lock()
	f.merges = append(f.merges, mergeCall{actorID: actorID, sheetID: sheetID, hash: hash, reason: reason})
	f.mu.Unlock()
	return legacyimport.Report{}, f.mergeErrs[sheetID]
}

type fakeAdmins struct {
	admin *player.Player
	err   error
}

type fakeDelivery struct {
	result websheetsync.FlushResult
	err    error
}

func (f fakeDelivery) Flush(context.Context) (websheetsync.FlushResult, error) {
	return f.result, f.err
}

func (f fakeAdmins) FindActiveAdmin(context.Context) (*player.Player, error) { return f.admin, f.err }

func TestRunMergesOnlyErrorFreePreviewsWithAdminAndFixedReason(t *testing.T) {
	syncer := &fakeSyncer{sheets: []string{"valid", "invalid", "fetch-failure"}, previews: map[string]legacyimport.Report{"valid": {RawHash: "hash"}, "invalid": {RawHash: "bad", Errors: []string{"sync_unmapped_raid"}}}, previewErrs: map[string]error{"fetch-failure": errors.New("unavailable")}}
	service := NewService(syncer, fakeAdmins{admin: &player.Player{ID: 42, Role: validation.RoleAdmin, Active: true}}, nil)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(syncer.merges) != 1 {
		t.Fatalf("merges = %+v", syncer.merges)
	}
	call := syncer.merges[0]
	if call.actorID != 42 || call.sheetID != "valid" || call.hash != "hash" || call.reason != AutomatedReason {
		t.Fatalf("merge = %+v", call)
	}
}

func TestRunContinuesAfterMergeFailure(t *testing.T) {
	syncer := &fakeSyncer{sheets: []string{"first", "second"}, previews: map[string]legacyimport.Report{"first": {RawHash: "one"}, "second": {RawHash: "two"}}, mergeErrs: map[string]error{"first": errors.New("changed")}}
	service := NewService(syncer, fakeAdmins{admin: &player.Player{ID: 7, Role: validation.RoleAdmin, Active: true}}, nil)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(syncer.merges) != 2 || syncer.merges[1].sheetID != "second" {
		t.Fatalf("merges = %+v", syncer.merges)
	}
}

func TestRunSkipsOverlap(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	syncer := &fakeSyncer{sheets: []string{"sheet"}, previews: map[string]legacyimport.Report{"sheet": {RawHash: "hash"}}, block: block, started: started}
	service := NewService(syncer, fakeAdmins{admin: &player.Player{ID: 1, Role: validation.RoleAdmin, Active: true}}, nil)
	done := make(chan error, 1)
	go func() { done <- service.Run(context.Background()) }()
	<-started
	if err := service.Run(context.Background()); !errors.Is(err, ErrRunInProgress) {
		t.Fatalf("overlap error = %v", err)
	}
	close(block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRunDoesNotPullSheetAfterDeliveryFailure(t *testing.T) {
	syncer := &fakeSyncer{sheets: []string{"sheet"}, previews: map[string]legacyimport.Report{"sheet": {RawHash: "hash"}}}
	service := NewService(syncer, fakeAdmins{admin: &player.Player{ID: 1, Role: validation.RoleAdmin, Active: true}}, fakeDelivery{result: websheetsync.FlushResult{Failed: 1}})
	if err := service.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded despite failed website delivery")
	}
	if len(syncer.merges) != 0 {
		t.Fatalf("merges = %+v", syncer.merges)
	}
}
