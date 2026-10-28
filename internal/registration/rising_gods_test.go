package registration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestRisingGodsProxyUsesBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Rising-Gods-Proxy-Token"); got != "proxy-secret" {
			t.Fatalf("X-Rising-Gods-Proxy-Token = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != risingGodsUserAgent {
			t.Fatalf("User-Agent = %q", got)
		}
		if r.URL.Query().Get("profiles") != "eu.rising-gods" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[],"parent":{}}`))
	}))
	defer srv.Close()

	client := RisingGodsClient{HTTPClient: srv.Client(), ProxyURL: srv.URL, ProxyToken: "proxy-secret", cache: newRisingGodsCache()}
	rows, err := client.SearchCharacters(context.Background(), "Test")
	if err != nil {
		t.Fatalf("search through proxy: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("unexpected rows: %#v", rows)
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
			_, _ = w.Write([]byte(profileBody("9596079", "Servenaja", 5, 1, 0, 80)))
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
		{name: "level 79", level: 79, faction: 0},
		{name: "horde", level: 80, faction: 1},
		{name: "unknown faction", level: 80, faction: 2},
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

func TestRisingGodsLoadCharacterCalculatesAddonGearScore(t *testing.T) {
	body := `g_items.add(49998, {"quality":4,"jsonequip":{"id":49998,"classs":4,"subclass":1,"slot":5,"level":264,"gearscore":1}});` +
		`g_items.add(51335, {"quality":4,"jsonequip":{"id":51335,"classs":4,"subclass":-3,"slot":2,"level":264,"gearscore":9999}});` +
		profileBody("9596079", "Servenaja", 5, 1, 0, 80)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	profile, _, err := client.LoadCharacter(context.Background(), "9596079")
	if err != nil {
		t.Fatalf("load character: %v", err)
	}
	expected := gearScoreItemScore(264, 4, "INVTYPE_CHEST") + gearScoreItemScore(264, 4, "INVTYPE_NECK")
	if profile.GearScore == nil || *profile.GearScore != expected {
		t.Fatalf("expected addon gearscore %d, got %#v", expected, profile.GearScore)
	}
}

func TestGearScoreCalculationAppliesHunterWeaponWeights(t *testing.T) {
	body := `g_items.add(51406, {"quality":4,"jsonequip":{"id":51406,"classs":2,"subclass":15,"slot":21,"level":264}});` +
		`g_items.add(51531, {"quality":4,"jsonequip":{"id":51531,"classs":2,"subclass":19,"slot":15,"level":264}});`
	gearScore := calculateGearScoreFromProfileBody(body, "Jäger")
	mainHand := float64(gearScoreItemScore(264, 4, "INVTYPE_WEAPONMAINHAND")) * 0.3164
	ranged := float64(gearScoreItemScore(264, 4, "INVTYPE_RANGEDRIGHT")) * 5.3224
	expected := int(mainHand + ranged)
	if gearScore == nil || *gearScore != expected {
		t.Fatalf("expected hunter gearscore %d, got %#v", expected, gearScore)
	}
}

func TestRisingGodsSearchServesStaleWhileRefreshing(t *testing.T) {
	var searches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&searches, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":2,"name":"Servenaja","classs":5,"level":80,"talenttree1":0,"talenttree2":51,"talenttree3":20}],"parent":{}}`))
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	client.cache.setSearch("search:serv", []ExternalCharacter{{ProfileID: "1", Name: "Servold", Class: "Priester", MainSpec: "Heilig", Level: 80}}, true, -time.Second)

	rows, err := client.SearchCharacters(context.Background(), "Serv")
	if err != nil {
		t.Fatalf("search characters: %v", err)
	}
	if len(rows) != 1 || rows[0].ProfileID != "1" {
		t.Fatalf("expected stale rows to be served immediately, got %#v", rows)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if rows, fresh, ok := client.cache.getSearch("search:serv"); ok && fresh && len(rows) == 1 && rows[0].ProfileID == "2" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale search was not refreshed in the background")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&searches); got != 1 {
		t.Fatalf("expected one background refresh, got %d upstream searches", got)
	}
}

func TestRisingGodsSearchCancelledCallerDoesNotFailWaiters(t *testing.T) {
	var searches int32
	arrived := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&searches, 1) == 1 {
			close(arrived)
		}
		<-release
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Servenaja","classs":5,"level":80,"talenttree1":0,"talenttree2":51,"talenttree3":20}],"parent":{}}`))
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := client.SearchCharacters(firstCtx, "Serv")
		firstErr <- err
	}()
	<-arrived
	secondRows := make(chan []ExternalCharacter, 1)
	secondErr := make(chan error, 1)
	go func() {
		rows, err := client.SearchCharacters(context.Background(), "Serv")
		secondRows <- rows
		secondErr <- err
	}()
	cancelFirst()
	if err := <-firstErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled first caller, got %v", err)
	}
	close(release)
	if err := <-secondErr; err != nil {
		t.Fatalf("second caller failed: %v", err)
	}
	if rows := <-secondRows; len(rows) != 1 || rows[0].Name != "Servenaja" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if got := atomic.LoadInt32(&searches); got != 1 {
		t.Fatalf("expected one coalesced upstream search, got %d", got)
	}
}

func TestRisingGodsLoadCharacterUsesCachedSearchSpec(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("profile") != "load" {
			t.Errorf("unexpected upstream request: %s", r.URL.String())
			return
		}
		_, _ = w.Write([]byte(`$WowheadProfiler.registerProfile({"id":7,"name":"Servenaja","classs":5,"race":1,"faction":0,"level":80});`))
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	client.cache.setSearch("search:servenaja", []ExternalCharacter{{ProfileID: "7", Name: "Servenaja", Class: "Priester", MainSpec: "Heilig", Level: 80}}, true, -time.Hour)

	profile, _, err := client.LoadCharacter(context.Background(), "7")
	if err != nil {
		t.Fatalf("load character: %v", err)
	}
	if profile.MainSpec != "Heilig" {
		t.Fatalf("expected spec from cached search row, got %q", profile.MainSpec)
	}
}

func TestRisingGodsSearchCacheStaysBounded(t *testing.T) {
	cache := newRisingGodsCache()
	for i := 0; i <= risingGodsSearchMaxEntries; i++ {
		cache.setSearch("search:name"+strconv.Itoa(i), nil, true, time.Duration(i)*time.Second)
	}
	if got := len(cache.searches); got != risingGodsSearchMaxEntries {
		t.Fatalf("expected %d cached searches, got %d", risingGodsSearchMaxEntries, got)
	}
	if _, _, ok := cache.getSearch("search:name0"); ok {
		t.Fatal("expected the entry closest to expiry to be evicted")
	}
}

func TestRisingGodsSearchClassifiesUpstreamRejectionAsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Rising-Gods-Proxy-Upstream-Status", "502")
		w.Header().Set("X-Rising-Gods-Proxy-Request-ID", "proxy-request-1")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := testRisingGodsClient(srv.URL)
	_, err := client.SearchCharacters(context.Background(), "Servenaja")
	if !errors.Is(err, ErrExternalUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if !strings.Contains(err.Error(), "proxy upstream status 502") || !strings.Contains(err.Error(), "proxy request proxy-request-1") {
		t.Fatalf("expected proxy diagnostics, got %v", err)
	}
}

func TestRisingGodsSearchDoesNotCacheFailedResponses(t *testing.T) {
	for _, tc := range []struct {
		name        string
		firstStatus int
		firstBody   string
	}{
		{name: "upstream error", firstStatus: http.StatusBadGateway, firstBody: "Bad Gateway"},
		{name: "missing search data", firstStatus: http.StatusOK, firstBody: "<html>Temporary upstream error</html>"},
		{name: "incomplete search data", firstStatus: http.StatusOK, firstBody: `{"data":[{"id":1}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt32(&requests, 1) == 1 {
					w.WriteHeader(tc.firstStatus)
					_, _ = w.Write([]byte(tc.firstBody))
					return
				}
				_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Servenaja","classs":5,"level":80,"talenttree1":0,"talenttree2":51,"talenttree3":20}],"parent":{}}`))
			}))
			defer srv.Close()

			client := testRisingGodsClient(srv.URL)
			if _, err := client.SearchCharacters(context.Background(), "Serv"); !errors.Is(err, ErrExternalUnavailable) {
				t.Fatalf("expected upstream failure, got %v", err)
			}
			if _, _, ok := client.cache.getSearch("search:serv"); ok {
				t.Fatal("failed search should not be cached")
			}
			for i := 0; i < 2; i++ {
				rows, err := client.SearchCharacters(context.Background(), "Serv")
				if err != nil || len(rows) != 1 || rows[0].Name != "Servenaja" {
					t.Fatalf("expected successful retry, got rows=%#v err=%v", rows, err)
				}
			}
			if got := atomic.LoadInt32(&requests); got != 2 {
				t.Fatalf("expected failure and one successful cached retry, got %d requests", got)
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
