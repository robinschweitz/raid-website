package registration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRisingGodsSearchUsesAllianceLevel80FilterAndCache(t *testing.T) {
	var searches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("profiles") != "eu.rising-gods" {
			t.Fatalf("unexpected profiles query: %s", r.URL.RawQuery)
		}
		filter := r.URL.Query().Get("filter")
		if !strings.Contains(filter, "si=1") || !strings.Contains(filter, "minle=80") || !strings.Contains(filter, "maxle=80") || !strings.Contains(filter, "na=Serv") {
			t.Fatalf("unexpected filter: %q", filter)
		}
		atomic.AddInt32(&searches, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":9596079,"name":"Servenaja","classs":5,"level":80,"talenttree1":0,"talenttree2":51,"talenttree3":20}],"parent":{}}`))
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	for i := 0; i < 2; i++ {
		rows, err := client.SearchCharacters(context.Background(), "Serv")
		if err != nil {
			t.Fatalf("search characters: %v", err)
		}
		if len(rows) != 1 || rows[0].Name != "Servenaja" || rows[0].Faction != "Alliance" || rows[0].Level != 80 {
			t.Fatalf("unexpected rows: %#v", rows)
		}
	}
	if got := atomic.LoadInt32(&searches); got != 1 {
		t.Fatalf("expected cached second search, got %d upstream searches", got)
	}
}

func TestRisingGodsSearchUsesTargetedQueryForSubstringMatches(t *testing.T) {
	var searchRequests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if filter != "si=1;minle=80;maxle=80;na=klein" {
			t.Fatalf("unexpected filter: %q", filter)
		}
		atomic.AddInt32(&searchRequests, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":244821959,"name":"Klein","classs":8,"level":80,"talenttree1":17,"talenttree2":0,"talenttree3":53}],"parent":{}}`))
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	for i := 0; i < 2; i++ {
		rows, err := client.SearchCharacters(context.Background(), "klein")
		if err != nil {
			t.Fatalf("search substring characters: %v", err)
		}
		if len(rows) != 1 || rows[0].Name != "Klein" {
			t.Fatalf("unexpected rows: %#v", rows)
		}
	}
	if got := atomic.LoadInt32(&searchRequests); got != 1 {
		t.Fatalf("expected cached second search, got %d upstream searches", got)
	}
}

func TestRisingGodsSearchReusesCompletePrefixCache(t *testing.T) {
	var searches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filter := r.URL.Query().Get("filter"); filter != "si=1;minle=80;maxle=80;na=Klein" {
			t.Fatalf("unexpected filter: %q", filter)
		}
		atomic.AddInt32(&searches, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Klein","classs":8,"level":80,"talenttree1":17,"talenttree2":0,"talenttree3":53},{"id":2,"name":"Kleine","classs":5,"level":80,"talenttree1":8,"talenttree2":12,"talenttree3":0}],"parent":{}}`))
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	if _, err := client.SearchCharacters(context.Background(), "Klein"); err != nil {
		t.Fatalf("search prefix: %v", err)
	}
	rows, err := client.SearchCharacters(context.Background(), "Kleine")
	if err != nil {
		t.Fatalf("search extended prefix: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "Kleine" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if got := atomic.LoadInt32(&searches); got != 1 {
		t.Fatalf("expected extended query to reuse complete prefix cache, got %d upstream searches", got)
	}
}

func TestRisingGodsSearchDoesNotReuseCappedPrefixCache(t *testing.T) {
	var searches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		atomic.AddInt32(&searches, 1)
		if filter == "si=1;minle=80;maxle=80;na=Klein" {
			objects := make([]string, 0, risingGodsListCap)
			for i := 0; i < risingGodsListCap; i++ {
				objects = append(objects, `{"id":`+strconv.Itoa(i+1)+`,"name":"Klein`+strconv.Itoa(i)+`","classs":8,"level":80,"talenttree1":17,"talenttree2":0,"talenttree3":53}`)
			}
			_, _ = w.Write([]byte(`{"data":[` + strings.Join(objects, ",") + `],"parent":{}}`))
			return
		}
		if filter == "si=1;minle=80;maxle=80;na=Kleine" {
			_, _ = w.Write([]byte(`{"data":[{"id":999,"name":"Kleine","classs":5,"level":80,"talenttree1":8,"talenttree2":12,"talenttree3":0}],"parent":{}}`))
			return
		}
		t.Fatalf("unexpected filter: %q", filter)
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	if _, err := client.SearchCharacters(context.Background(), "Klein"); err != nil {
		t.Fatalf("search capped prefix: %v", err)
	}
	rows, err := client.SearchCharacters(context.Background(), "Kleine")
	if err != nil {
		t.Fatalf("search extended capped prefix: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "Kleine" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if got := atomic.LoadInt32(&searches); got != 2 {
		t.Fatalf("expected capped prefix not to be reused, got %d upstream searches", got)
	}
}

func TestRisingGodsSearchResolvesPastedProfileURL(t *testing.T) {
	var slugLoads int32
	var profileLoads int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("profile") == "eu.rising-gods.servenaja":
			atomic.AddInt32(&slugLoads, 1)
			_, _ = w.Write([]byte(`profilah.initialize('profilah-generic', { id: 9596079 });`))
		case r.URL.Query().Get("profile") == "load" && r.URL.Query().Get("id") == "9596079":
			atomic.AddInt32(&profileLoads, 1)
			_, _ = w.Write([]byte(profileBody("9596079", "Servenaja", 5, 1, 1, 80)))
		default:
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	rows, err := client.SearchCharacters(context.Background(), "https://db.rising-gods.de/?profile=eu.rising-gods.servenaja")
	if err != nil {
		t.Fatalf("search pasted profile URL: %v", err)
	}
	if len(rows) != 1 || rows[0].ProfileID != "9596079" || rows[0].Name != "Servenaja" || rows[0].Faction != "Alliance" || rows[0].Level != 80 {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if atomic.LoadInt32(&slugLoads) != 1 || atomic.LoadInt32(&profileLoads) != 1 {
		t.Fatalf("unexpected upstream counts: slug=%d profile=%d", slugLoads, profileLoads)
	}
}

func TestRisingGodsLoadCharacterRejectsNonAllianceOrNonLevel80(t *testing.T) {
	for _, tc := range []struct {
		name    string
		level   int
		faction int
	}{
		{name: "level 79", level: 79, faction: 1},
		{name: "horde", level: 80, faction: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(profileBody("9596079", "Servenaja", 5, 1, tc.faction, tc.level)))
			}))
			defer srv.Close()

			client := testRisingGodsClient(srv.URL)
			if _, _, err := client.LoadCharacter(context.Background(), "9596079"); err == nil {
				t.Fatal("expected invalid profile error")
			}
		})
	}
}

func testRisingGodsClient(base string) RisingGodsClient {
	return RisingGodsClient{HTTPClient: &http.Client{Transport: rewriteTransport{base: base}}, cache: newRisingGodsCache()}
}

type rewriteTransport struct {
	base string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	baseURL, err := url.Parse(t.base)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = baseURL.Scheme
	clone.URL.Host = baseURL.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func profileBody(id, name string, classID, raceID, factionID, level int) string {
	return `$WowheadProfiler.registerProfile({"id":` + id + `,"name":"` + name + `","classs":` + strconv.Itoa(classID) + `,"race":` + strconv.Itoa(raceID) + `,"faction":` + strconv.Itoa(factionID) + `,"level":` + strconv.Itoa(level) + `,"talents":"000000000000000000000000000000000000000000000000000111111111"});`
}
