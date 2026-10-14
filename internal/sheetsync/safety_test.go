package sheetsync

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/uptrace/bun"
	"io"
	"net/http"
	"path/filepath"
	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/legacyimport"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"strings"
	"testing"
	"time"
)

const validationCopy = "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8"

type validationFetcher struct{ data legacyimport.SheetData }

func (f *validationFetcher) Fetch(_ context.Context, id string) (legacyimport.SheetData, error) {
	if id != validationCopy {
		return legacyimport.SheetData{}, errors.New("forbidden source")
	}
	return f.data, nil
}

type validationWriter struct {
	calls int
	tabs  map[string][][]string
}

func (w *validationWriter) VerifyWorkingCopy(_ context.Context, id string) error {
	if id != validationCopy {
		return errors.New("copy only")
	}
	return nil
}
func (w *validationWriter) Publish(_ context.Context, id string, tabs map[string][][]string, prev map[string]string) (map[string]string, error) {
	if id != validationCopy {
		return nil, errors.New("copy only")
	}
	w.calls++
	w.tabs = tabs
	out := map[string]string{}
	for k, v := range tabs {
		out[k] = rowsHash(v)
	}
	return out, nil
}
func validationDB(t *testing.T) (*bun.DB, *legacyimport.Service, *validationFetcher, int64) {
	t.Helper()
	ctx := context.Background()
	b, e := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "pr13.sqlite")+"?_pragma=foreign_keys(1)")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { b.Close() })
	m, e := migrations.NewMigrator(b.DB)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = lootcatalog.NewService(lootcatalog.NewRepo(b.DB)).EnsureBundled(ctx); e != nil {
		t.Fatal(e)
	}
	p := player.NewRepo(b.DB)
	now := time.Now().UTC()
	a := &player.Player{Name: "PR13 isolated admin", Role: "ADMIN", Active: true, CreatedAt: now, UpdatedAt: now}
	if e = p.Create(ctx, a); e != nil {
		t.Fatal(e)
	}
	f := &validationFetcher{data: legacyimport.SheetData{Raid: []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\n"), Loot: []byte("Raid ID,Timestamp,Spieler,Loot,Priority,ItemId,Import ID\n"), Character: []byte("Charakter,Spieler,Klasse,Main Spec,Second Spec\nHero,Hero,Paladin,Vergeltung,\n"), Points: []byte("Spieler,Punkte,Inaktiv\nHero,170,Nein\n"), Setup: []byte(",Charakter,Spieler,Klasse,Spec,Rolle\n1,Hero,Hero,Paladin,Vergeltung,Mdd\n"), Hash: "fixture-v1"}}
	s := legacyimport.NewService(legacyimport.NewRepo(b.DB), p, character.NewRepo(b.DB), raid.NewRepo(b.DB), attendance.NewRepo(b.DB), loot.NewRepo(b.DB), enhancepoints.NewRepo(b.DB), raidseries.NewRepo(b.DB), f)
	return b.DB, s, f, a.ID
}

func TestSafetyRunOnceRepeatAndAdoptNewSheetHistory(t *testing.T) {
	b, imp, f, a := validationDB(t)
	w := &validationWriter{}
	s, e := New(b, imp, w, validationCopy, a, "PR13 isolation", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	r, e := s.RunOnce(ctx)
	if e != nil || w.calls != 1 {
		t.Fatal(e, r)
	}
	r, e = s.RunOnce(ctx)
	if e != nil || !r.LastImport.Skipped || w.calls != 2 {
		t.Fatal(e, r)
	}
	var count int
	if err := b.NewSelect().TableExpr("enhance_point_transactions").ColumnExpr("COUNT(*)").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate EP: %d", count)
	}
	f.data.Raid = append(f.data.Raid, []byte("ICC25-99,14.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")...)
	f.data.Character = append(f.data.Character, []byte("MustNotImport,MustNotImport,Paladin,Vergeltung,\n")...)
	f.data.Hash = "new-history"
	r, e = s.RunOnce(ctx)
	if e != nil || w.calls != 3 {
		t.Fatal("new Sheet archive was not adopted", e, r)
	}
	for table, want := range map[string]int{"raids": 1, "characters": 2, "enhance_point_transactions": 1} {
		var n int
		if err := b.NewSelect().TableExpr(table).ColumnExpr("COUNT(*)").Scan(ctx, &n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("adoption had unexpected %s count: %d", table, n)
		}
	}
}
func TestSafetyRunOnceAdoptsHistoricalArchiveTransition(t *testing.T) {
	b, imp, f, a := validationDB(t)
	ctx := context.Background()
	f.data.Raid = append(f.data.Raid, []byte("ICC25-99,14.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")...)
	p, e := imp.DryRun(ctx, Source, validationCopy)
	if e != nil {
		t.Fatal(e)
	}
	m, e := imp.Merge(ctx, a, Source, validationCopy, p.RawHash, "Test history fixture")
	if e != nil || len(m.Errors) > 0 {
		t.Fatal(e, m.Errors)
	}
	if _, e = b.ExecContext(ctx, "UPDATE raids SET archived_at=NULL WHERE name='ICC25-99'"); e != nil {
		t.Fatal(e)
	}
	w := &validationWriter{}
	s, e := New(b, imp, w, validationCopy, a, "test", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.RunOnce(ctx)
	if e != nil || w.calls != 1 {
		t.Fatal("archive transition failed", e)
	}
	var n int
	if err := b.NewSelect().TableExpr("raids").ColumnExpr("COUNT(*)").Where("archived_at IS NOT NULL").Scan(ctx, &n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("historical archive transition missing")
	}
}

type validationTransport struct {
	tabs         map[string][][]string
	writes       int
	failBatch    bool
	paths        []string
	ids          map[string]int64
	loseResponse bool
}

func (v *validationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	v.paths = append(v.paths, r.URL.String())
	var out any = map[string]any{}
	code := 200
	if v.ids == nil {
		v.ids = map[string]int64{}
		var next int64 = 1
		for title := range v.tabs {
			v.ids[title] = next
			next++
		}
	}
	if r.URL.Host == "oauth2.googleapis.com" {
		out = map[string]string{"access_token": "TEST-ONLY"}
	} else if strings.Contains(r.URL.Path, "/values/") {
		if r.Method != "GET" {
			return nil, fmt.Errorf("non-atomic values write")
		}
		part := strings.Split(r.URL.Path, "/values/")[1]
		title := strings.Split(strings.TrimPrefix(part, "'"), "'!")[0]
		out = map[string]any{"values": v.tabs[title]}
	} else if r.Method == "GET" {
		sheets := []any{}
		for title, id := range v.ids {
			sheets = append(sheets, map[string]any{"properties": map[string]any{"title": title, "sheetId": id, "gridProperties": map[string]int{"rowCount": 1000, "columnCount": 26}}})
		}
		out = map[string]any{"sheets": sheets}
	} else if r.Method == "POST" && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
		v.writes++
		if v.failBatch {
			v.failBatch = false
			code = 503
			out = map[string]string{"error": "injected atomic batch failure"}
		} else {
			var payload struct {
				Requests []struct {
					Add *struct {
						Properties struct {
							Title string `json:"title"`
							ID    int64  `json:"sheetId"`
						} `json:"properties"`
					} `json:"addSheet"`
					Update *struct {
						Range struct {
							ID int64 `json:"sheetId"`
						} `json:"range"`
						Rows []struct {
							Values []struct {
								Value struct {
									Text string `json:"stringValue"`
								} `json:"userEnteredValue"`
							} `json:"values"`
						} `json:"rows"`
					} `json:"updateCells"`
				} `json:"requests"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				return nil, err
			}
			for _, request := range payload.Requests {
				if request.Add != nil {
					v.ids[request.Add.Properties.Title] = request.Add.Properties.ID
					v.tabs[request.Add.Properties.Title] = [][]string{}
				}
				if request.Update != nil {
					title := ""
					for k, id := range v.ids {
						if id == request.Update.Range.ID {
							title = k
						}
					}
					if title == "" {
						return nil, fmt.Errorf("missing atomic target")
					}
					rows := [][]string{}
					for _, row := range request.Update.Rows {
						cells := []string{}
						for _, cell := range row.Values {
							cells = append(cells, cell.Value.Text)
						}
						rows = append(rows, cells)
					}
					v.tabs[title] = rows
				}
			}
			if v.loseResponse {
				v.loseResponse = false
				return nil, io.ErrUnexpectedEOF
			}
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
}
func validationGoogle(t *testing.T, v *validationTransport) *GoogleWriter {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	return &GoogleWriter{email: "test@example.invalid", key: key, client: &http.Client{Transport: v}}
}
func TestSafetyGoogleMirrorConflictAndRepeat(t *testing.T) {
	v := &validationTransport{tabs: map[string][][]string{"Website Charaktere": {{"ID", "Name"}, {"1", "Hero"}}, "Charaktere": {{"untouched"}}}}
	g := validationGoogle(t, v)
	ctx := context.Background()
	data := map[string][][]string{"Website Charaktere": v.tabs["Website Charaktere"]}
	h := map[string]string{"Website Charaktere": rowsHash(data["Website Charaktere"])}
	if _, e := g.Publish(ctx, validationCopy, data, h); e != nil || v.writes != 0 {
		t.Fatal(e, v.writes)
	}
	v.tabs["Website Charaktere"][1][1] = "Manual edit"
	data["Website Charaktere"] = [][]string{{"ID", "Name"}, {"1", "Hero"}}
	_, e := g.Publish(ctx, validationCopy, data, h)
	if e == nil || v.writes != 0 {
		t.Fatal("mirror conflict not protected")
	}
}
func TestSafetyGoogleExportRetryAfterFailure(t *testing.T) {
	old := [][]string{{"ID", "Name"}, {"1", "Hero"}}
	v := &validationTransport{tabs: map[string][][]string{"Website Charaktere": old}, failBatch: true}
	g := validationGoogle(t, v)
	h := map[string]string{"Website Charaktere": rowsHash(old)}
	data := map[string][][]string{"Website Charaktere": {{"ID", "Name"}, {"1", "Hero"}, {"2", "New"}}}
	_, e1 := g.Publish(context.Background(), validationCopy, data, h)
	_, e2 := g.Publish(context.Background(), validationCopy, data, h)
	if e1 == nil {
		t.Fatal("failure injection failed")
	}
	if e2 != nil {
		t.Fatalf("retry cannot recover from own cleared tab: %v", e2)
	}
}

func TestSafetyAtomicExportLostResponseAndShorterSnapshot(t *testing.T) {
	old := [][]string{{"ID", "Name"}, {"1", "Hero"}, {"2", "Gone"}}
	v := &validationTransport{tabs: map[string][][]string{"Website Charaktere": old, "Website Punkte": {{"Points"}, {"10"}}}, loseResponse: true}
	g := validationGoogle(t, v)
	desired := map[string][][]string{"Website Charaktere": {{"ID", "Name"}, {"1", "Hero"}}, "Website Punkte": {{"Points"}, {"20"}}}
	previous := map[string]string{}
	for title, rows := range v.tabs {
		previous[title] = rowsHash(rows)
	}
	if _, err := g.Publish(t.Context(), validationCopy, desired, previous); err == nil {
		t.Fatal("lost response was not simulated")
	}
	if v.writes != 1 {
		t.Fatal("export was not one batch")
	}
	for title, rows := range desired {
		if rowsHash(v.tabs[title]) != rowsHash(rows) {
			t.Fatal("batch did not replace whole snapshots", title)
		}
	}
	if _, err := g.Publish(t.Context(), validationCopy, desired, previous); err != nil {
		t.Fatal("lost response retry failed", err)
	}
	if v.writes != 1 {
		t.Fatal("identical retry wrote again")
	}
}

func TestSafetyGoogleRejectsSourceTabsAndOtherSheetIDsBeforeNetwork(t *testing.T) {
	v := &validationTransport{tabs: map[string][][]string{}}
	g := validationGoogle(t, v)
	if _, err := g.Publish(t.Context(), validationCopy, map[string][][]string{"Charaktere": {{"bad"}}}, nil); err == nil {
		t.Fatal("source tab accepted")
	}
	if _, err := g.Publish(t.Context(), "other-sheet", map[string][][]string{}, nil); err == nil {
		t.Fatal("other target accepted")
	}
	if len(v.paths) != 0 {
		t.Fatal("rejected export contacted Google")
	}
}

func TestSafetyDeletedMirrorIsConflict(t *testing.T) {
	v := &validationTransport{tabs: map[string][][]string{}}
	g := validationGoogle(t, v)
	desired := map[string][][]string{"Website Charaktere": {{"Name"}, {"Hero"}}}
	if _, err := g.Publish(t.Context(), validationCopy, desired, map[string]string{"Website Charaktere": rowsHash(desired["Website Charaktere"])}); err == nil {
		t.Fatal("deleted mirror silently recreated")
	}
	if v.writes != 0 {
		t.Fatal("deleted mirror conflict caused a write")
	}
}
