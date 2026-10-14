package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/legacyimport"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

type setupFetcher struct {
	calls int
	data  legacyimport.SheetData
}

func (f *setupFetcher) Fetch(context.Context, string) (legacyimport.SheetData, error) {
	f.calls++
	return f.data, nil
}

func TestImportSetupAndArchivedAdditionsAreAdminOnly(t *testing.T) {
	bdb, _ := middlewareAuthDB(t)
	ctx := context.Background()
	if _, err := lootcatalog.NewService(lootcatalog.NewRepo(bdb.DB)).EnsureBundled(ctx); err != nil {
		t.Fatal(err)
	}
	players := player.NewRepo(bdb.DB)
	now := time.Now().UTC()
	actor := &player.Player{Name: "Importer", Active: true, Role: validation.RoleAdmin, CreatedAt: now, UpdatedAt: now}
	if err := players.Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	fetcher := &setupFetcher{data: legacyimport.SheetData{
		Raid:      []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\nICC25-01,04.10.2026,1,Oxora,DD,Paladin,Vergeltung\n"),
		Loot:      []byte("Raid ID,Timestamp,Spieler,Loot,Priority,ItemID,Import ID\n"),
		Character: []byte("Charakter,Spieler,Klasse,Main Spec,Second Spec\nOxora,Oxora,Paladin,Vergeltung,\n"),
		Points:    []byte("Spieler,Punkte\nOxora,20\n"),
		Setup:     []byte(",Charakter,Spieler,Klasse,Spec,Rolle\n1,Oxora,Oxora,Paladin,Vergeltung,Mdd\n"), Hash: "first",
	}}
	svc := legacyimport.NewService(legacyimport.NewRepo(bdb.DB), players, character.NewRepo(bdb.DB), raid.NewRepo(bdb.DB), attendance.NewRepo(bdb.DB), loot.NewRepo(bdb.DB), enhancepoints.NewRepo(bdb.DB), raidseries.NewRepo(bdb.DB), fetcher)
	mux := http.NewServeMux()
	v1.NewAPI(v1.Deps{Imports: svc}).Register(mux)
	config := &legacyimport.SetupConfig{Name: "ICC25", Leader: "Oxora", FirstDate: "2026-10-05", StartTime: "19:00", Timezone: "Europe/Berlin", Weekdays: []int{1}, IntervalWeeks: 1}
	request := func(path string, role validation.Role, hash, reason string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{"sheet_id": "sheet", "raw_hash": hash, "reason": reason, "setup": config})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		caller := *actor
		caller.Role = role
		r = r.WithContext(v1.WithActor(r.Context(), &caller))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, role := range []validation.Role{validation.RoleGuest, validation.RoleMember, validation.RoleRaidassistent, validation.RoleRaidleader} {
		for _, path := range []string{"/api/v1/imports/dry-run", "/api/v1/imports", "/api/v1/sync/dry-run", "/api/v1/sync"} {
			w := request(path, role, "", "")
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s %s: %d %s", role, path, w.Code, w.Body.String())
			}
		}
	}
	if fetcher.calls != 0 {
		t.Fatal("unauthorized request fetched source")
	}
	syncRequest := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{"sheet_id": "sheet"})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		r = r.WithContext(v1.WithActor(r.Context(), actor))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	syncPreview := syncRequest("/api/v1/sync/dry-run")
	if syncPreview.Code != http.StatusOK || !bytes.Contains(syncPreview.Body.Bytes(), []byte(`"sync_unmapped_raid: ICC25-01"`)) {
		t.Fatalf("sync preview: %d %s", syncPreview.Code, syncPreview.Body.String())
	}
	setupRejected := request("/api/v1/sync/dry-run", validation.RoleAdmin, "", "")
	if setupRejected.Code != http.StatusBadRequest {
		t.Fatalf("sync setup accepted: %d %s", setupRejected.Code, setupRejected.Body.String())
	}
	decode := func(w *httptest.ResponseRecorder) legacyimport.Report {
		t.Helper()
		var response struct {
			Data legacyimport.Report `json:"data"`
		}
		if w.Code != http.StatusOK {
			t.Fatalf("response: %d %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	preview := decode(request("/api/v1/imports/dry-run", validation.RoleAdmin, "", ""))
	if len(preview.Errors) != 0 || preview.Setup == nil {
		t.Fatalf("preview: %+v", preview)
	}
	merged := decode(request("/api/v1/imports", validation.RoleAdmin, preview.RawHash, ""))
	if len(merged.Errors) != 0 || merged.Created.Series != 1 || merged.Created.Raids != 2 {
		t.Fatalf("merge: %+v", merged)
	}
	fetcher.data.Hash = "second"
	fetcher.data.Loot = append(fetcher.data.Loot, []byte("ICC25-01,04.10.2026,Oxora,Historical loot,Main,,stable-1\n")...)
	preview = decode(request("/api/v1/imports/dry-run", validation.RoleAdmin, "", ""))
	if len(preview.Errors) != 0 {
		t.Fatalf("archived preview: %+v", preview)
	}
	missingReason := decode(request("/api/v1/imports", validation.RoleAdmin, preview.RawHash, ""))
	if len(missingReason.Errors) == 0 || missingReason.Created != (legacyimport.Counts{}) {
		t.Fatalf("missing reason: %+v", missingReason)
	}
	merged = decode(request("/api/v1/imports", validation.RoleAdmin, preview.RawHash, "Missing historical drop"))
	if len(merged.Errors) != 0 || merged.Created.Loot != 1 || merged.Created.Series != 0 || merged.Created.Raids != 0 {
		t.Fatalf("archived merge: %+v", merged)
	}
}
