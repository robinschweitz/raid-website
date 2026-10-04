package lootcatalog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"raid-website/internal/db"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func TestPinnedSourceFixtureOffline(t *testing.T) {
	content, err := os.ReadFile("testdata/marrowgar.catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var d lootcatalog.Dataset
	if err := json.Unmarshal(content, &d); err != nil {
		t.Fatal(err)
	}
	_, report := lootcatalog.Canonical(d)
	if len(report.Errors) != 0 || report.RawHash != "174a4d8ee52eb47ab89b457e8e778bb289370800f994c0d70bbe7c4849b80316" {
		t.Fatalf("source fixture: %+v", report)
	}
	if len(d.Instances) != 1 || d.Instances[0].ID != "631" || len(d.Bosses) != 1 || d.Bosses[0].ID != "36612" || len(d.Items) != 13 || len(d.Drops) != 13 {
		t.Fatal("fixture misrepresented source subset")
	}
	found := false
	for _, drop := range d.Drops {
		if drop.ItemID == "50759" && drop.BossID == "36612" && drop.RaidSize == 10 && strings.HasSuffix(drop.Difficulty, "source-lootmode-path:1/1") {
			found = true
		}
	}
	if !found {
		t.Fatal("reference loot relationship missing")
	}
	s := setup(t)
	ctx := context.Background()
	p, err := s.Preview(ctx, d, report.RawHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Merge(ctx, d, p.RawHash); err != nil {
		t.Fatal(err)
	}
	items, err := s.Items(ctx, lootcatalog.Filter{BossID: "36612", RaidSize: 10})
	if err != nil || len(items) != 13 {
		t.Fatalf("source fixture import: %v %d", err, len(items))
	}
}

func setup(t *testing.T) *lootcatalog.Service {
	t.Helper()
	ctx := context.Background()
	b, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	m, err := migrations.NewMigrator(b.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return lootcatalog.NewService(lootcatalog.NewRepo(b.DB))
}

// This synthetic domain fixture tests validation, not source catalog completeness.
func dataset() lootcatalog.Dataset {
	return lootcatalog.Dataset{
		Source:    lootcatalog.Source{ID: "test-source", Revision: "test-revision", URL: "https://example.invalid/source", License: "test-only", Attribution: "Synthetic test fixture"},
		Notices:   []string{"Synthetic; not game data"},
		Instances: []lootcatalog.Instance{{ID: "1", Name: "Test instance"}},
		Bosses:    []lootcatalog.Boss{{ID: "2", InstanceID: "1", Name: "Test boss"}},
		Items:     []lootcatalog.Item{{ItemID: "3", Name: "Test item"}},
		Drops:     []lootcatalog.Drop{{BossID: "2", ItemID: "3", RaidSize: 10, Difficulty: "source-mode:1", LootMode: 1}},
	}
}

func TestReviewedMergeNoOverwrite(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	d := dataset()
	_, canonical := lootcatalog.Canonical(d)
	if _, err := s.Merge(ctx, d, canonical.RawHash); !errors.Is(err, lootcatalog.ErrPreviewRequired) {
		t.Fatalf("unreviewed merge: %v", err)
	}
	preview, err := s.Preview(ctx, d, "")
	if err != nil || preview.Inserted != 4 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	c, err := s.List(ctx)
	if err != nil || len(c.Instances) != 0 {
		t.Fatalf("preview wrote domain rows: %+v %v", c, err)
	}
	report, err := s.Merge(ctx, d, preview.RawHash)
	if err != nil || !report.Merged || report.Inserted != 4 {
		t.Fatalf("merge: %+v %v", report, err)
	}
	report, err = s.Merge(ctx, d, preview.RawHash)
	if err != nil || report.Inserted != 0 || report.Skipped != 4 {
		t.Fatalf("repeat merge: %+v %v", report, err)
	}
	d.Items[0].Name = "Changed"
	if _, err := s.Merge(ctx, d, preview.RawHash); !errors.Is(err, lootcatalog.ErrHashMismatch) {
		t.Fatalf("changed reviewed payload: %v", err)
	}
	report, err = s.Preview(ctx, d, "")
	if !errors.Is(err, lootcatalog.ErrConflict) || len(report.Conflicts) == 0 {
		t.Fatalf("overwrite preview: %+v %v", report, err)
	}
	c, err = s.List(ctx)
	if err != nil || c.Items[0].Name != "Test item" {
		t.Fatalf("overwrote item: %+v %v", c, err)
	}
	if c.Sources[0].RawHash != preview.RawHash {
		t.Fatal("provenance lost reviewed hash")
	}
}

func TestValidationRejectsIDsLinksDuplicatesAndVariants(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*lootcatalog.Dataset)
	}{
		{"duplicate", func(d *lootcatalog.Dataset) { d.Items = append(d.Items, d.Items[0]) }},
		{"link", func(d *lootcatalog.Dataset) { d.Bosses[0].InstanceID = "99" }},
		{"item-link", func(d *lootcatalog.Dataset) { d.Drops[0].ItemID = "99" }},
		{"boss-link", func(d *lootcatalog.Dataset) { d.Drops[0].BossID = "99" }},
		{"id", func(d *lootcatalog.Dataset) { d.Items[0].ItemID = "03" }},
		{"size", func(d *lootcatalog.Dataset) { d.Drops[0].RaidSize = 40 }},
		{"mode", func(d *lootcatalog.Dataset) { d.Drops[0].LootMode = 0 }},
		{"difficulty", func(d *lootcatalog.Dataset) { d.Drops[0].Difficulty = " " }},
		{"duplicate-drop", func(d *lootcatalog.Dataset) { d.Drops = append(d.Drops, d.Drops[0]) }},
		{"source", func(d *lootcatalog.Dataset) { d.Source.License = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := dataset()
			tc.change(&d)
			_, report := lootcatalog.Canonical(d)
			if len(report.Errors) == 0 {
				t.Fatal("accepted invalid dataset")
			}
		})
	}
}

func TestSourceConflictsAndWholeMergeRollback(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	d := dataset()
	p, err := s.Preview(ctx, d, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Merge(ctx, d, p.RawHash); err != nil {
		t.Fatal(err)
	}
	d.Source.ID = "other-source"
	if _, err := s.Preview(ctx, d, ""); !errors.Is(err, lootcatalog.ErrConflict) {
		t.Fatalf("accepted foreign ownership: %v", err)
	}
	d = dataset()
	d.Source.Revision = "new-revision"
	d.Items = append(d.Items, lootcatalog.Item{ItemID: "4", Name: "New item"})
	d.Drops = append(d.Drops, lootcatalog.Drop{BossID: "2", ItemID: "4", RaidSize: 25, Difficulty: "mixed-source-mode:3", LootMode: 3})
	p, err = s.Preview(ctx, d, "")
	if err != nil {
		t.Fatal(err)
	}
	d.Items[0].Name = "Conflicting"
	_, canonical := lootcatalog.Canonical(d)
	if _, err := s.Merge(ctx, d, canonical.RawHash); !errors.Is(err, lootcatalog.ErrPreviewRequired) {
		t.Fatalf("accepted different evidence: %v", err)
	}
	c, err := s.List(ctx)
	if err != nil || len(c.Items) != 1 {
		t.Fatalf("partial writes: %+v %v", c, err)
	}
	// A previously valid preview must be rechecked against changes committed afterwards.
	d = dataset()
	d.Source.Revision = "later"
	d.Items = append(d.Items, lootcatalog.Item{ItemID: "4", Name: "A"})
	d.Drops = append(d.Drops, lootcatalog.Drop{BossID: "2", ItemID: "4", RaidSize: 10, Difficulty: "source-mode:1", LootMode: 1})
	p, err = s.Preview(ctx, d, "")
	if err != nil {
		t.Fatal(err)
	}
	other := d
	other.Source.Revision = "intervening"
	other.Items = append([]lootcatalog.Item{}, d.Items...)
	other.Items[1].Name = "B"
	op, err := s.Preview(ctx, other, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Merge(ctx, other, op.RawHash); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Merge(ctx, d, p.RawHash); !errors.Is(err, lootcatalog.ErrConflict) {
		t.Fatalf("stale preview conflict not rechecked: %v", err)
	}
}

func TestFiltersAndAssociationRequireDatasetSize(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	d := dataset()
	p, err := s.Preview(ctx, d, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Merge(ctx, d, p.RawHash); err != nil {
		t.Fatal(err)
	}
	instance, size := "1", 10
	if err := s.ValidateAssociation(ctx, &instance, &size); err != nil {
		t.Fatal(err)
	}
	size = 25
	if err := s.ValidateAssociation(ctx, &instance, &size); !errors.Is(err, lootcatalog.ErrInvalid) {
		t.Fatal("guessed 25-player variant")
	}
	if err := s.ValidateAssociation(ctx, nil, &size); !errors.Is(err, lootcatalog.ErrInvalid) {
		t.Fatal("size without instance")
	}
	items, err := s.Items(ctx, lootcatalog.Filter{BossID: "2", RaidSize: 10, Difficulty: "source-mode:1"})
	if err != nil || len(items) != 1 || items[0].LootMode != 1 {
		t.Fatalf("filter: %+v %v", items, err)
	}
	items, err = s.Items(ctx, lootcatalog.Filter{RaidSize: 25})
	if err != nil || len(items) != 0 || items == nil {
		t.Fatal("missing filter result must be empty array")
	}
}

func TestCatalogHTTPPermissionsAndArrays(t *testing.T) {
	s := setup(t)
	mux := http.NewServeMux()
	v1.CatalogHandlers{Catalog: s}.Register(mux)
	request := func(method, path string, role validation.Role, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		if role != "" {
			r = r.WithContext(v1.WithActor(r.Context(), &player.Player{ID: 1, Role: role}))
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v1/loot-catalog", "/api/v1/loot-catalog/instances", "/api/v1/loot-catalog/bosses", "/api/v1/loot-catalog/items"} {
		if w := request("GET", path, "", nil); w.Code != 401 {
			t.Fatalf("unauthenticated read: %d", w.Code)
		}
		w := request("GET", path, validation.RoleGuest, nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "null") {
			t.Fatalf("empty arrays: %d %s", w.Code, w.Body)
		}
	}
	for _, role := range []validation.Role{validation.RoleGuest, validation.RoleMember, validation.RoleRaidassistent, validation.RoleRaidleader} {
		for _, path := range []string{"/api/v1/loot-catalog/preview", "/api/v1/loot-catalog/merge"} {
			if w := request("POST", path, role, []byte(`{}`)); w.Code != 403 {
				t.Fatalf("%s write permission: %d", role, w.Code)
			}
		}
	}
	for _, path := range []string{"/api/v1/loot-catalog/preview", "/api/v1/loot-catalog/merge"} {
		if w := request("POST", path, "", []byte(`{}`)); w.Code != 401 {
			t.Fatalf("unauthenticated catalog write: %d", w.Code)
		}
	}
	body, _ := json.Marshal(map[string]any{"catalog": dataset()})
	w := request("POST", "/api/v1/loot-catalog/preview", validation.RoleAdmin, body)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	var envelope struct {
		Data lootcatalog.Report `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]any{"catalog": dataset(), "raw_hash": envelope.Data.RawHash})
	w = request("POST", "/api/v1/loot-catalog/merge", validation.RoleAdmin, body)
	if w.Code != 200 {
		t.Fatalf("merge: %d %s", w.Code, w.Body)
	}
	w = request("GET", "/api/v1/loot-catalog/items?boss_id=2&raid_size=10", validation.RoleGuest, nil)
	var itemEnvelope struct {
		Data []lootcatalog.ItemDrop `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &itemEnvelope); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(itemEnvelope.Data) != 1 || itemEnvelope.Data[0].ItemID != "3" || itemEnvelope.Data[0].BossID != "2" {
		t.Fatalf("item identity missing in JSON: %s", w.Body)
	}
	w = request("POST", "/api/v1/loot-catalog/preview", validation.RoleAdmin, []byte(`{"catalog":{},"surprise":1}`))
	if w.Code != 400 {
		t.Fatal("unknown request fields accepted")
	}
	w = request("GET", "/api/v1/loot-catalog/items?raid_size=0", validation.RoleGuest, nil)
	if w.Code != 422 {
		t.Fatal("explicit invalid size accepted")
	}
}
