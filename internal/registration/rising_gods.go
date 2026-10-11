package registration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"raid-website/internal/validation"
)

const risingGodsBaseURL = "https://db.rising-gods.de"

var (
	ErrExternalNotFound = errors.New("rising-gods character not found")
	ErrExternalInvalid  = errors.New("rising-gods profile invalid")

	listviewProfileID = regexp.MustCompile(`"id"\s*:\s*(\d+)`)
	listviewName      = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
	listviewClass     = regexp.MustCompile(`"classs"\s*:\s*(\d+)`)
	listviewLevel     = regexp.MustCompile(`"level"\s*:\s*(\d+)`)
	listviewTree1     = regexp.MustCompile(`"talenttree1"\s*:\s*(\d+)`)
	listviewTree2     = regexp.MustCompile(`"talenttree2"\s*:\s*(\d+)`)
	listviewTree3     = regexp.MustCompile(`"talenttree3"\s*:\s*(\d+)`)
	profileIDField    = regexp.MustCompile(`"id"\s*:\s*(\d+)`)
	profileLevel      = regexp.MustCompile(`"level"\s*:\s*(\d+)`)
	profileClass      = regexp.MustCompile(`"classs"\s*:\s*(\d+)`)
	profileRace       = regexp.MustCompile(`"race"\s*:\s*(\d+)`)
	profileFaction    = regexp.MustCompile(`"faction"\s*:\s*(\d+)`)
	profileName       = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
	profileTalent     = regexp.MustCompile(`"talents"\s*:\s*"([0-9]+)"`)
	profilePageID     = regexp.MustCompile(`profilah\.initialize\([^)]*\{\s*id\s*:\s*(\d+)`)

	defaultRisingGodsCache = newRisingGodsCache()
)

const (
	risingGodsSearchTTL  = 10 * time.Minute
	risingGodsProfileTTL = 10 * time.Minute
	risingGodsSlugTTL    = 24 * time.Hour
	risingGodsListCap    = 300
)

type ExternalClient interface {
	SearchCharacters(ctx context.Context, query string) ([]ExternalCharacter, error)
	LoadCharacter(ctx context.Context, profileID string) (ExternalCharacter, string, error)
}

type RisingGodsClient struct {
	HTTPClient *http.Client
	cache      *risingGodsCache
}

type risingGodsCache struct {
	mu       sync.Mutex
	searches map[string]cachedSearch
	profiles map[string]cachedProfile
	slugs    map[string]cachedSlug
	inflight map[string]*risingGodsCall
}

type cachedSearch struct {
	rows      []ExternalCharacter
	expiresAt time.Time
	complete  bool
}

type cachedProfile struct {
	profile   ExternalCharacter
	raw       string
	expiresAt time.Time
}

type cachedSlug struct {
	id        string
	expiresAt time.Time
}

type risingGodsCall struct {
	done chan struct{}
	val  any
	err  error
}

func newRisingGodsCache() *risingGodsCache {
	return &risingGodsCache{searches: map[string]cachedSearch{}, profiles: map[string]cachedProfile{}, slugs: map[string]cachedSlug{}, inflight: map[string]*risingGodsCall{}}
}

func (c RisingGodsClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c RisingGodsClient) sharedCache() *risingGodsCache {
	if c.cache != nil {
		return c.cache
	}
	return defaultRisingGodsCache
}

func (c RisingGodsClient) SearchCharacters(ctx context.Context, query string) ([]ExternalCharacter, error) {
	q := strings.TrimSpace(query)
	if len([]rune(q)) < 3 {
		return nil, ErrExternalInvalid
	}
	if profileRef, ok := risingGodsProfileReference(q); ok {
		profile, _, err := c.loadProfileReference(ctx, profileRef)
		if err != nil {
			return nil, err
		}
		return []ExternalCharacter{profile}, nil
	}
	cacheKey := "search:" + strings.ToLower(q)
	if rows, ok := c.sharedCache().getSearch(cacheKey); ok {
		return rows, nil
	}
	if rows, ok := c.sharedCache().getCompletePrefixSearch(q); ok {
		return rows, nil
	}
	rowsAny, err := c.sharedCache().do(cacheKey, func() (any, error) {
		if rows, ok := c.sharedCache().getSearch(cacheKey); ok {
			return rows, nil
		}
		if rows, ok := c.sharedCache().getCompletePrefixSearch(q); ok {
			return rows, nil
		}
		rows, err := c.searchCharactersLive(ctx, q)
		if err != nil {
			return nil, err
		}
		c.sharedCache().setSearch(cacheKey, rows, len(rows) < risingGodsListCap, risingGodsSearchTTL)
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return rowsAny.([]ExternalCharacter), nil
}

func (c RisingGodsClient) searchCharactersLive(ctx context.Context, q string) ([]ExternalCharacter, error) {
	return c.searchCharactersByFilter(ctx, "si=1;minle=80;maxle=80;na="+q)
}

func (c RisingGodsClient) searchCharactersByFilter(ctx context.Context, filter string) ([]ExternalCharacter, error) {
	u := risingGodsBaseURL + "/?profiles=eu.rising-gods&filter=" + url.QueryEscape(filter)
	body, err := c.fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	dataStart := strings.Index(body, `"data":[`)
	if dataStart < 0 {
		return []ExternalCharacter{}, nil
	}
	dataEnd := strings.Index(body[dataStart:], `,"parent"`)
	if dataEnd < 0 {
		return []ExternalCharacter{}, nil
	}
	data := body[dataStart : dataStart+dataEnd]
	objects := splitJSObjects(data)
	results := make([]ExternalCharacter, 0, len(objects))
	for _, raw := range objects {
		id := firstSubmatch(listviewProfileID, raw)
		name := firstSubmatch(listviewName, raw)
		classID, _ := strconv.Atoi(firstSubmatch(listviewClass, raw))
		level, _ := strconv.Atoi(firstSubmatch(listviewLevel, raw))
		tree1, _ := strconv.Atoi(firstSubmatch(listviewTree1, raw))
		tree2, _ := strconv.Atoi(firstSubmatch(listviewTree2, raw))
		tree3, _ := strconv.Atoi(firstSubmatch(listviewTree3, raw))
		class := className(classID)
		if id == "" || name == "" || class == "" || level != 80 {
			continue
		}
		results = append(results, ExternalCharacter{ProfileID: id, Name: name, Class: class, MainSpec: specFromTreeTotals(class, [3]int{tree1, tree2, tree3}), Level: level, Faction: "Alliance", ProfileURL: profileURL(id)})
	}
	return results, nil
}

func (c RisingGodsClient) LoadCharacter(ctx context.Context, profileID string) (ExternalCharacter, string, error) {
	id := strings.TrimSpace(profileID)
	if id == "" {
		return ExternalCharacter{}, "", ErrExternalInvalid
	}
	cacheKey := "profile:" + id
	if profile, raw, ok := c.sharedCache().getProfile(cacheKey); ok {
		return profile, raw, nil
	}
	profileAny, err := c.sharedCache().do(cacheKey, func() (any, error) {
		if profile, raw, ok := c.sharedCache().getProfile(cacheKey); ok {
			return cachedProfile{profile: profile, raw: raw}, nil
		}
		profile, raw, err := c.loadCharacterLive(ctx, id)
		if err != nil {
			return nil, err
		}
		c.sharedCache().setProfile(cacheKey, profile, raw, risingGodsProfileTTL)
		return cachedProfile{profile: profile, raw: raw}, nil
	})
	if err != nil {
		return ExternalCharacter{}, "", err
	}
	cached := profileAny.(cachedProfile)
	return cached.profile, cached.raw, nil
}

func (c RisingGodsClient) loadCharacterLive(ctx context.Context, id string) (ExternalCharacter, string, error) {
	body, err := c.fetch(ctx, risingGodsBaseURL+"/?profile=load&id="+url.QueryEscape(id))
	if err != nil {
		return ExternalCharacter{}, "", err
	}
	raw := extractRegisterProfileObject(body)
	if raw == "" {
		return ExternalCharacter{}, "", ErrExternalNotFound
	}
	loadedID := firstSubmatch(profileIDField, raw)
	level, _ := strconv.Atoi(firstSubmatch(profileLevel, raw))
	classID, _ := strconv.Atoi(firstSubmatch(profileClass, raw))
	raceID, _ := strconv.Atoi(firstSubmatch(profileRace, raw))
	factionID, _ := strconv.Atoi(firstSubmatch(profileFaction, raw))
	name := firstSubmatch(profileName, raw)
	class := className(classID)
	faction := factionName(factionID)
	if loadedID != id || name == "" || class == "" || level != 80 || faction != "Alliance" {
		return ExternalCharacter{}, "", ErrExternalInvalid
	}
	talentMatches := profileTalent.FindAllStringSubmatch(raw, 2)
	mainSpec := ""
	secondSpec := ""
	if len(talentMatches) > 0 {
		mainSpec = deriveSpec(class, talentMatches[0][1])
	}
	if len(talentMatches) > 1 {
		secondSpec = deriveSpec(class, talentMatches[1][1])
	}
	if mainSpec == "" {
		rows, err := c.SearchCharacters(ctx, name)
		if err == nil {
			for _, row := range rows {
				if row.ProfileID == id && row.MainSpec != "" {
					mainSpec = row.MainSpec
					break
				}
			}
		}
	}
	if mainSpec == "" {
		return ExternalCharacter{}, "", ErrExternalInvalid
	}
	profile := ExternalCharacter{ProfileID: id, Name: name, Class: class, MainSpec: mainSpec, SecondSpec: secondSpec, Level: level, Race: raceName(raceID), Faction: faction, ProfileURL: profileURL(id)}
	return profile, raw, nil
}

func (c RisingGodsClient) loadProfileReference(ctx context.Context, ref string) (ExternalCharacter, string, error) {
	if id := profileLoadID(ref); id != "" {
		return c.LoadCharacter(ctx, id)
	}
	id, err := c.resolveProfileSlug(ctx, ref)
	if err != nil {
		return ExternalCharacter{}, "", err
	}
	return c.LoadCharacter(ctx, id)
}

func (c RisingGodsClient) resolveProfileSlug(ctx context.Context, ref string) (string, error) {
	slugURL, err := normalizedRisingGodsProfileURL(ref)
	if err != nil {
		return "", err
	}
	cacheKey := "slug:" + strings.ToLower(slugURL)
	if id, ok := c.sharedCache().getSlug(cacheKey); ok {
		return id, nil
	}
	idAny, err := c.sharedCache().do(cacheKey, func() (any, error) {
		if id, ok := c.sharedCache().getSlug(cacheKey); ok {
			return id, nil
		}
		body, err := c.fetch(ctx, slugURL)
		if err != nil {
			return "", err
		}
		id := firstSubmatch(profilePageID, body)
		if id == "" {
			return "", ErrExternalInvalid
		}
		c.sharedCache().setSlug(cacheKey, id, risingGodsSlugTTL)
		return id, nil
	})
	if err != nil {
		return "", err
	}
	return idAny.(string), nil
}

func (c RisingGodsClient) fetch(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrExternalNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("rising-gods status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *risingGodsCache) getSearch(key string) ([]ExternalCharacter, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.searches[key]
	if !ok || time.Now().After(cached.expiresAt) {
		return nil, false
	}
	rows := append([]ExternalCharacter(nil), cached.rows...)
	return rows, true
}

func (c *risingGodsCache) getCompletePrefixSearch(query string) ([]ExternalCharacter, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	q := strings.ToLower(strings.TrimSpace(query))
	var best cachedSearch
	bestPrefix := ""
	for key, cached := range c.searches {
		if !cached.complete || now.After(cached.expiresAt) || !strings.HasPrefix(key, "search:") {
			continue
		}
		prefix := strings.TrimPrefix(key, "search:")
		if len(prefix) < 3 || len(prefix) >= len(q) || !strings.HasPrefix(q, prefix) || len(prefix) <= len(bestPrefix) {
			continue
		}
		best = cached
		bestPrefix = prefix
	}
	if bestPrefix == "" {
		return nil, false
	}
	return filterRowsByName(best.rows, q), true
}

func (c *risingGodsCache) setSearch(key string, rows []ExternalCharacter, complete bool, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.searches[key] = cachedSearch{rows: append([]ExternalCharacter(nil), rows...), expiresAt: time.Now().Add(ttl), complete: complete}
}

func (c *risingGodsCache) getProfile(key string) (ExternalCharacter, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.profiles[key]
	if !ok || time.Now().After(cached.expiresAt) {
		return ExternalCharacter{}, "", false
	}
	return cached.profile, cached.raw, true
}

func (c *risingGodsCache) setProfile(key string, profile ExternalCharacter, raw string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profiles[key] = cachedProfile{profile: profile, raw: raw, expiresAt: time.Now().Add(ttl)}
}

func (c *risingGodsCache) getSlug(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.slugs[key]
	if !ok || time.Now().After(cached.expiresAt) {
		return "", false
	}
	return cached.id, true
}

func (c *risingGodsCache) setSlug(key string, id string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slugs[key] = cachedSlug{id: id, expiresAt: time.Now().Add(ttl)}
}

func (c *risingGodsCache) do(key string, fn func() (any, error)) (any, error) {
	c.mu.Lock()
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.val, call.err
	}
	call := &risingGodsCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	call.val, call.err = fn()

	c.mu.Lock()
	delete(c.inflight, key)
	close(call.done)
	c.mu.Unlock()
	return call.val, call.err
}

func risingGodsProfileReference(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "https://db.rising-gods.de/") || strings.HasPrefix(strings.ToLower(trimmed), "http://db.rising-gods.de/") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", false
		}
		profile := strings.TrimSpace(u.Query().Get("profile"))
		if profile == "" {
			return "", false
		}
		return trimmed, true
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "eu.rising-gods.") {
		return trimmed, true
	}
	return "", false
}

func profileLoadID(ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if u.Query().Get("profile") != "load" {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("id"))
}

func normalizedRisingGodsProfileURL(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if strings.HasPrefix(strings.ToLower(trimmed), "eu.rising-gods.") {
		return risingGodsBaseURL + "/?profile=" + url.QueryEscape(trimmed), nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(u.Host, "db.rising-gods.de") {
		return "", ErrExternalInvalid
	}
	profile := strings.TrimSpace(u.Query().Get("profile"))
	if profile == "" || profile == "load" {
		return "", ErrExternalInvalid
	}
	return risingGodsBaseURL + "/?profile=" + url.QueryEscape(profile), nil
}

func filterRowsByName(rows []ExternalCharacter, query string) []ExternalCharacter {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []ExternalCharacter{}
	}
	matches := make([]ExternalCharacter, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(strings.TrimSpace(row.Name)), q) {
			matches = append(matches, row)
		}
	}
	return matches
}

func splitJSObjects(data string) []string {
	start := strings.IndexByte(data, '[')
	if start < 0 {
		return nil
	}
	var out []string
	depth := 0
	objectStart := -1
	inString := false
	escaped := false
	for i := start + 1; i < len(data); i++ {
		ch := data[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		switch ch {
		case '{':
			if depth == 0 {
				objectStart = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && objectStart >= 0 {
				out = append(out, data[objectStart:i+1])
				objectStart = -1
			}
		}
	}
	return out
}

func extractRegisterProfileObject(body string) string {
	marker := "$WowheadProfiler.registerProfile("
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	if start >= len(body) || body[start] != '{' {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(body); i++ {
		ch := body[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return body[start : i+1]
			}
		}
	}
	return ""
}

func firstSubmatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func className(id int) string {
	switch id {
	case 1:
		return "Krieger"
	case 2:
		return "Paladin"
	case 3:
		return "Jäger"
	case 4:
		return "Schurke"
	case 5:
		return "Priester"
	case 6:
		return "Todesritter"
	case 7:
		return "Schamane"
	case 8:
		return "Magier"
	case 9:
		return "Hexer"
	case 11:
		return "Druide"
	default:
		return ""
	}
}

func raceName(id int) string {
	switch id {
	case 1:
		return "Mensch"
	case 2:
		return "Orc"
	case 3:
		return "Zwerg"
	case 4:
		return "Nachtelf"
	case 5:
		return "Untoter"
	case 6:
		return "Taure"
	case 7:
		return "Gnom"
	case 8:
		return "Troll"
	case 10:
		return "Blutelf"
	case 11:
		return "Draenei"
	default:
		return ""
	}
}

func factionName(id int) string {
	if id == 1 {
		return "Alliance"
	}
	if id == 2 {
		return "Horde"
	}
	return ""
}

func profileURL(id string) string {
	return risingGodsBaseURL + "/?profile=" + url.QueryEscape(id)
}

func deriveSpec(class, talents string) string {
	if talents == "" || len(talents)%3 != 0 {
		return ""
	}
	section := len(talents) / 3
	sums := [3]int{}
	for tree := 0; tree < 3; tree++ {
		for _, ch := range talents[tree*section : (tree+1)*section] {
			if ch >= '0' && ch <= '9' {
				sums[tree] += int(ch - '0')
			}
		}
	}
	best := 0
	for i := 1; i < 3; i++ {
		if sums[i] > sums[best] {
			best = i
		}
	}
	if sums[best] == 0 {
		return ""
	}
	specs := validation.ClassSpecs[class]
	if len(specs) < 3 {
		return ""
	}
	if class == "Todesritter" && best == 0 {
		return "Blut (DD)"
	}
	if class == "Druide" && best == 1 {
		return "Wilder Kampf (Cat)"
	}
	return specs[best]
}

func specFromTreeTotals(class string, sums [3]int) string {
	best := 0
	for i := 1; i < 3; i++ {
		if sums[i] > sums[best] {
			best = i
		}
	}
	if sums[best] == 0 {
		return ""
	}
	specs := validation.ClassSpecs[class]
	if len(specs) < 3 {
		return ""
	}
	if class == "Todesritter" && best == 0 {
		return "Blut (DD)"
	}
	if class == "Druide" && best == 1 {
		return "Wilder Kampf (Cat)"
	}
	return specs[best]
}
