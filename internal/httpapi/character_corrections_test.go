package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/validation"
)

type characterCorrectionFixture struct {
	srv     *httptest.Server
	db      *bun.DB
	actorID int64
	token   string
	initial character.Character
}

func newCharacterCorrectionFixture(t *testing.T) characterCorrectionFixture {
	t.Helper()
	srv, db := setupServer(t)
	actorID := seedAccount(t, db, "character-admin", "password", validation.RoleAdmin)
	token := login(t, srv.Client(), srv, "character-admin", "password")
	now := time.Now().UTC()
	c := character.Character{PlayerID: actorID, Name: "Original", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(db).Create(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	// Use the stored timestamp precision for snapshot comparisons.
	stored, err := character.NewRepo(db).Get(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return characterCorrectionFixture{srv: srv, db: db, actorID: actorID, token: token, initial: *stored}
}

func characterSnapshotsEqual(first, second character.Character) bool {
	// SQLite stores microseconds; JSON retains nanoseconds and decodes UTC locations.
	for _, c := range []*character.Character{&first, &second} {
		c.CreatedAt = c.CreatedAt.UTC().Truncate(time.Microsecond)
		c.UpdatedAt = c.UpdatedAt.UTC().Truncate(time.Microsecond)
		if c.NameChangedAt != nil {
			changedAt := c.NameChangedAt.UTC().Truncate(time.Microsecond)
			c.NameChangedAt = &changedAt
		}
	}
	return reflect.DeepEqual(first, second)
}

func TestCharacterCorrectionsRequireReasonForEveryField(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	url := fmt.Sprintf("%s/api/v1/characters/%d", f.srv.URL, f.initial.ID)
	for field, value := range map[string]any{"name": "Renamed", "class": "Krieger", "main_spec": "Arkan", "second_spec": "", "is_main": true, "active": false} {
		for _, reason := range []string{"", " \t\r\n "} {
			t.Run(field+fmt.Sprintf("/%q", reason), func(t *testing.T) {
				status, body := do(t, f.srv.Client(), http.MethodPatch, url, f.token, map[string]any{field: value, "reason": reason})
				if status != http.StatusBadRequest {
					t.Fatalf("status %d: %s", status, body)
				}
				var out struct{ Error struct{ Code string } }
				if err := json.Unmarshal(body, &out); err != nil {
					t.Fatal(err)
				}
				if out.Error.Code != "reason_required" {
					t.Fatalf("error code %q", out.Error.Code)
				}
			})
		}
	}
	stored, err := character.NewRepo(f.db).Get(context.Background(), f.initial.ID)
	if err != nil || !reflect.DeepEqual(*stored, f.initial) {
		t.Fatalf("rejected correction changed character: %+v, %v", stored, err)
	}
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "character", fmt.Sprint(f.initial.ID))
	if err != nil || len(logs) != 0 {
		t.Fatalf("rejected correction wrote audit: %+v, %v", logs, err)
	}
}

func TestCharacterCorrectionsGuestAndInvalidActor(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	guestID := seedAccount(t, f.db, "character-guest", "password", validation.RoleGuest)
	token := login(t, f.srv.Client(), f.srv, "character-guest", "password")
	url := fmt.Sprintf("%s/api/v1/characters/%d", f.srv.URL, f.initial.ID)
	status, body := do(t, f.srv.Client(), http.MethodPatch, url, token, map[string]any{"name": "Denied", "reason": "rename"})
	if status != http.StatusForbidden {
		t.Fatalf("guest status %d: %s", status, body)
	}
	name := "Denied"
	service := character.NewService(character.NewRepo(f.db))
	for _, tc := range []struct {
		actor int64
		want  error
	}{{guestID, character.ErrForbidden}, {999999, character.ErrInvalidActor}, {0, character.ErrInvalidActor}} {
		if _, err := service.Correct(context.Background(), tc.actor, f.initial.ID, character.Patch{Name: &name, Reason: "rename"}); !errors.Is(err, tc.want) {
			t.Fatalf("actor %d: want %v, got %v", tc.actor, tc.want, err)
		}
	}
}

func TestCharacterCorrectionsPartialNormalizationAndNonuniqueMain(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	other := f.initial
	other.ID = 0
	other.Name = "Other"
	other.IsMain = true
	if err := character.NewRepo(f.db).Create(context.Background(), &other); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/characters/%d", f.srv.URL, f.initial.ID)
	status, body := do(t, f.srv.Client(), http.MethodPatch, url, f.token, map[string]any{"class": " warrior ", "is_main": true, "reason": " \tclass correction\n "})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	stored, err := character.NewRepo(f.db).Get(context.Background(), f.initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Class != "Krieger" || !stored.IsMain || stored.Name != f.initial.Name || stored.MainSpec != f.initial.MainSpec || stored.SecondSpec != f.initial.SecondSpec || !stored.Active {
		t.Fatalf("partial correction: %+v", stored)
	}
	second, err := character.NewRepo(f.db).Get(context.Background(), other.ID)
	if err != nil || !second.IsMain {
		t.Fatalf("other main changed: %+v, %v", second, err)
	}
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "character", fmt.Sprint(f.initial.ID))
	if err != nil || len(logs) != 1 || logs[0].Reason != "class correction" || logs[0].ActorID == nil || *logs[0].ActorID != f.actorID {
		t.Fatalf("audit: %+v, %v", logs, err)
	}
	for _, patch := range []map[string]any{{"class": "Barbar", "reason": "invalid"}, {"name": "", "reason": "invalid"}, {"player_id": f.actorID, "reason": "invalid"}} {
		status, body = do(t, f.srv.Client(), http.MethodPatch, url, f.token, patch)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid patch status %d: %s", status, body)
		}
	}
}

func TestCharacterCorrectionsRenameRollback(t *testing.T) {
	for _, table := range []string{"character_name_history", "audit_log"} {
		t.Run(table, func(t *testing.T) {
			f := newCharacterCorrectionFixture(t)
			ctx := context.Background()
			if _, err := f.db.ExecContext(ctx, "CREATE TRIGGER fail_character_correction BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT, 'history unavailable'); END"); err != nil {
				t.Fatal(err)
			}
			url := fmt.Sprintf("%s/api/v1/characters/%d", f.srv.URL, f.initial.ID)
			status, body := do(t, f.srv.Client(), http.MethodPatch, url, f.token, map[string]any{"name": "Renamed", "main_spec": "Arkan", "reason": "rename rollback"})
			if status != http.StatusInternalServerError {
				t.Fatalf("status %d: %s", status, body)
			}
			stored, err := character.NewRepo(f.db).Get(ctx, f.initial.ID)
			if err != nil || !reflect.DeepEqual(*stored, f.initial) {
				t.Fatalf("character escaped rollback: %+v, %v", stored, err)
			}
			history, err := character.NewRepo(f.db).ListNameHistory(ctx, f.initial.ID)
			if err != nil || len(history) != 0 {
				t.Fatalf("name history escaped rollback: %+v, %v", history, err)
			}
			logs, err := audit.NewRepo(f.db).ListByEntity(ctx, "character", fmt.Sprint(f.initial.ID))
			if err != nil || len(logs) != 0 {
				t.Fatalf("audit escaped rollback: %+v, %v", logs, err)
			}
			if _, err := f.db.ExecContext(ctx, "DROP TRIGGER fail_character_correction"); err != nil {
				t.Fatal(err)
			}
			status, body = do(t, f.srv.Client(), http.MethodPatch, url, f.token, map[string]any{"name": "Renamed", "reason": "  actual rename  "})
			if status != http.StatusOK {
				t.Fatalf("rename status %d: %s", status, body)
			}
			history, err = character.NewRepo(f.db).ListNameHistory(ctx, f.initial.ID)
			if err != nil || len(history) != 1 || history[0].OldName != f.initial.Name || history[0].NewName != "Renamed" || history[0].Reason != "actual rename" || history[0].ActorID != f.actorID {
				t.Fatalf("rename history: %+v, %v", history, err)
			}
		})
	}
}

func TestCharacterCorrectionsConcurrentFieldsAndBeforeSnapshots(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	ctx := context.Background()
	service := character.NewService(character.NewRepo(f.db))
	for i := 0; i < 10; i++ {
		name, spec := fmt.Sprintf("Renamed%d", i), fmt.Sprintf("Spec%d", i)
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, patch := range []character.Patch{{Name: &name, Reason: "rename"}, {MainSpec: &spec, Reason: "spec correction"}} {
			go func() {
				<-start
				_, err := service.Correct(ctx, f.actorID, f.initial.ID, patch)
				results <- err
			}()
		}
		close(start)
		for j := 0; j < 2; j++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		stored, err := character.NewRepo(f.db).Get(ctx, f.initial.ID)
		if err != nil || stored.Name != name || stored.MainSpec != spec {
			t.Fatalf("concurrent edit lost: %+v, %v", stored, err)
		}
	}
	logs, err := audit.NewRepo(f.db).ListByEntity(ctx, "character", fmt.Sprint(f.initial.ID))
	if err != nil || len(logs) != 20 {
		t.Fatalf("audit count %d: %v", len(logs), err)
	}
	previous := f.initial
	for _, log := range logs {
		var before, after character.Character
		if err := json.Unmarshal([]byte(log.BeforeJSON), &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(log.AfterJSON), &after); err != nil {
			t.Fatal(err)
		}
		if !characterSnapshotsEqual(before, previous) {
			t.Fatalf("stale before snapshot: got %+v, want %+v", before, previous)
		}
		if log.Reason == "rename" && (after.MainSpec != before.MainSpec || after.Name == before.Name || log.Action != "rename") {
			t.Fatalf("rename overwrote concurrent spec: %+v", log)
		}
		if log.Reason == "spec correction" && (after.Name != before.Name || after.MainSpec == before.MainSpec || log.Action != "update") {
			t.Fatalf("spec overwrote concurrent rename: %+v", log)
		}
		previous = after
	}
}
