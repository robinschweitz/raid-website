package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	ErrExternalNotFound    = errors.New("rising-gods character not found")
	ErrExternalInvalid     = errors.New("rising-gods profile invalid")
	ErrExternalUnavailable = errors.New("rising-gods unavailable")

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
	risingGodsItemAdd = regexp.MustCompile(`g_items\.add\(\d+,\s*`)

	defaultRisingGodsCache = newRisingGodsCache()
)

// Upstream profile list searches take 3-5 seconds regardless of the filter, so
// search results are kept for a long time and served stale while they are
// refreshed in the background. Profile loads stay short-lived because they are
// the authoritative check before anything is stored.
const (
	risingGodsSearchTTL        = 12 * time.Hour
	risingGodsSearchStaleMax   = 7 * 24 * time.Hour
	risingGodsSearchMaxEntries = 2000
	risingGodsProfileTTL       = 10 * time.Minute
	risingGodsSlugTTL          = 24 * time.Hour
	risingGodsListCap          = 300
	risingGodsFetchTimeout     = 20 * time.Second
)

type ExternalClient interface {
	SearchCharacters(ctx context.Context, query string) ([]ExternalCharacter, error)
	LoadCharacter(ctx context.Context, profileID string) (ExternalCharacter, string, error)
}

type RisingGodsClient struct {
	HTTPClient *http.Client
	ProxyURL   string
	ProxyToken string
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
	return &http.Client{Timeout: risingGodsFetchTimeout}
}

func (c RisingGodsClient) upstreamURL() string {
	if c.ProxyURL != "" {
		return strings.TrimRight(c.ProxyURL, "/")
	}
	return risingGodsBaseURL
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
	cache := c.sharedCache()
	cacheKey := "search:" + strings.ToLower(q)
	if rows, fresh, ok := cache.getSearch(cacheKey); ok {
		if !fresh {
			cache.start(cacheKey, c.refreshSearch(ctx, cacheKey, q))
		}
		return rows, nil
	}
	if rows, ok := cache.getCompletePrefixSearch(q); ok {
		return rows, nil
	}
	rowsAny, err := cache.do(ctx, cacheKey, c.refreshSearch(ctx, cacheKey, q))
	if err != nil {
		return nil, err
	}
	return rowsAny.([]ExternalCharacter), nil
}

// WarmSearch makes sure the cache holds a fresh result for query. Unlike
// SearchCharacters it waits for the upstream fetch instead of serving a stale
// entry, so the caller controls how many upstream requests run at once.
func (c RisingGodsClient) WarmSearch(ctx context.Context, query string) error {
	q := strings.TrimSpace(query)
	if len([]rune(q)) < 3 {
		return ErrExternalInvalid
	}
	cacheKey := "search:" + strings.ToLower(q)
	if _, fresh, ok := c.sharedCache().getSearch(cacheKey); ok && fresh {
		return nil
	}
	_, err := c.sharedCache().do(ctx, cacheKey, c.refreshSearch(ctx, cacheKey, q))
	return err
}

func (c RisingGodsClient) refreshSearch(ctx context.Context, cacheKey, q string) func() (any, error) {
	return func() (any, error) {
		if rows, fresh, ok := c.sharedCache().getSearch(cacheKey); ok && fresh {
			return rows, nil
		}
		fetchCtx, cancel := detachedFetchContext(ctx)
		defer cancel()
		rows, err := c.searchCharactersLive(fetchCtx, q)
		if err != nil {
			return nil, err
		}
		c.sharedCache().setSearch(cacheKey, rows, len(rows) < risingGodsListCap, risingGodsSearchTTL)
		return rows, nil
	}
}

// detachedFetchContext keeps the caller's values but not its cancellation:
// coalesced callers and background refreshes share one upstream fetch, which
// must not fail just because the first caller's request went away.
func detachedFetchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), risingGodsFetchTimeout)
}

func (c RisingGodsClient) searchCharactersLive(ctx context.Context, q string) ([]ExternalCharacter, error) {
	return c.searchCharactersByFilter(ctx, "si=1;minle=80;maxle=80;na="+q)
}

func (c RisingGodsClient) searchCharactersByFilter(ctx context.Context, filter string) ([]ExternalCharacter, error) {
	u := c.upstreamURL() + "/?profiles=eu.rising-gods&filter=" + url.QueryEscape(filter)
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
	profileAny, err := c.sharedCache().do(ctx, cacheKey, func() (any, error) {
		if profile, raw, ok := c.sharedCache().getProfile(cacheKey); ok {
			return cachedProfile{profile: profile, raw: raw}, nil
		}
		fetchCtx, cancel := detachedFetchContext(ctx)
		defer cancel()
		profile, raw, err := c.loadCharacterLive(fetchCtx, id)
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
	body, err := c.fetch(ctx, c.upstreamURL()+"/?profile=load&id="+url.QueryEscape(id))
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
		if row, ok := c.sharedCache().findSearchRow(id); ok {
			mainSpec = row.MainSpec
		}
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
	profile := ExternalCharacter{ProfileID: id, Name: name, Class: class, MainSpec: mainSpec, SecondSpec: secondSpec, GearScore: calculateGearScoreFromProfileBody(body, class), Level: level, Race: raceName(raceID), Faction: faction, ProfileURL: profileURL(id)}
	return profile, raw, nil
}

type risingGodsProfileItem struct {
	Quality   int `json:"quality"`
	JSONEquip struct {
		Class    int     `json:"classs"`
		Subclass int     `json:"subclass"`
		Level    float64 `json:"level"`
		Slot     int     `json:"slot"`
		SlotBak  int     `json:"slotbak"`
	} `json:"jsonequip"`
}

type gearScoreItem struct {
	inventorySlot int
	equipLoc      string
	score         int
}

type gearScoreItemType struct {
	slotMod  float64
	itemSlot int
}

var gearScoreItemTypes = map[string]gearScoreItemType{
	"INVTYPE_RELIC":          {slotMod: 0.3164, itemSlot: 18},
	"INVTYPE_TRINKET":        {slotMod: 0.5625, itemSlot: 33},
	"INVTYPE_2HWEAPON":       {slotMod: 2.0000, itemSlot: 16},
	"INVTYPE_WEAPONMAINHAND": {slotMod: 1.0000, itemSlot: 16},
	"INVTYPE_WEAPONOFFHAND":  {slotMod: 1.0000, itemSlot: 17},
	"INVTYPE_RANGED":         {slotMod: 0.3164, itemSlot: 18},
	"INVTYPE_THROWN":         {slotMod: 0.3164, itemSlot: 18},
	"INVTYPE_RANGEDRIGHT":    {slotMod: 0.3164, itemSlot: 18},
	"INVTYPE_SHIELD":         {slotMod: 1.0000, itemSlot: 17},
	"INVTYPE_WEAPON":         {slotMod: 1.0000, itemSlot: 36},
	"INVTYPE_HOLDABLE":       {slotMod: 1.0000, itemSlot: 17},
	"INVTYPE_HEAD":           {slotMod: 1.0000, itemSlot: 1},
	"INVTYPE_NECK":           {slotMod: 0.5625, itemSlot: 2},
	"INVTYPE_SHOULDER":       {slotMod: 0.7500, itemSlot: 3},
	"INVTYPE_CHEST":          {slotMod: 1.0000, itemSlot: 5},
	"INVTYPE_ROBE":           {slotMod: 1.0000, itemSlot: 5},
	"INVTYPE_WAIST":          {slotMod: 0.7500, itemSlot: 6},
	"INVTYPE_LEGS":           {slotMod: 1.0000, itemSlot: 7},
	"INVTYPE_FEET":           {slotMod: 0.7500, itemSlot: 8},
	"INVTYPE_WRIST":          {slotMod: 0.5625, itemSlot: 9},
	"INVTYPE_HAND":           {slotMod: 0.7500, itemSlot: 10},
	"INVTYPE_FINGER":         {slotMod: 0.5625, itemSlot: 31},
	"INVTYPE_CLOAK":          {slotMod: 0.5625, itemSlot: 15},
}

func calculateGearScoreFromProfileBody(body, class string) *int {
	items := gearScoreItemsFromProfileBody(body)
	if len(items) == 0 {
		return nil
	}
	titanGrip := 1.0
	hasMain := false
	hasOff := false
	for _, item := range items {
		if item.inventorySlot == 16 {
			hasMain = true
		}
		if item.inventorySlot == 17 {
			hasOff = true
		}
	}
	if hasMain && hasOff {
		for _, item := range items {
			if (item.inventorySlot == 16 || item.inventorySlot == 17) && item.equipLoc == "INVTYPE_2HWEAPON" {
				titanGrip = 0.5
				break
			}
		}
	}
	total := 0.0
	for _, item := range items {
		score := float64(item.score)
		if class == "Jäger" {
			if item.inventorySlot == 16 || item.inventorySlot == 17 {
				score *= 0.3164
			}
			if item.inventorySlot == 18 {
				score *= 5.3224
			}
		}
		if item.inventorySlot == 16 || item.inventorySlot == 17 {
			score *= titanGrip
		}
		total += score
	}
	if total <= 0 {
		return nil
	}
	gearScore := int(math.Floor(total))
	return &gearScore
}

func gearScoreItemsFromProfileBody(body string) []gearScoreItem {
	matches := risingGodsItemAdd.FindAllStringIndex(body, -1)
	items := make([]gearScoreItem, 0, len(matches))
	for _, match := range matches {
		object := extractJSONObjectAt(body, match[1])
		if object == "" {
			continue
		}
		var parsed risingGodsProfileItem
		if err := json.Unmarshal([]byte(object), &parsed); err != nil {
			continue
		}
		item, ok := gearScoreItemFromProfileItem(parsed)
		if ok {
			items = append(items, item)
		}
	}
	return items
}

func gearScoreItemFromProfileItem(item risingGodsProfileItem) (gearScoreItem, bool) {
	equipLoc, inventorySlot := risingGodsEquipLoc(item)
	if equipLoc == "" || inventorySlot == 4 {
		return gearScoreItem{}, false
	}
	score := gearScoreItemScore(item.JSONEquip.Level, item.Quality, equipLoc)
	if score < 0 {
		return gearScoreItem{}, false
	}
	return gearScoreItem{inventorySlot: inventorySlot, equipLoc: equipLoc, score: score}, true
}

func risingGodsEquipLoc(item risingGodsProfileItem) (string, int) {
	slot := item.JSONEquip.Slot
	switch slot {
	case 1:
		return "INVTYPE_HEAD", 1
	case 2:
		return "INVTYPE_NECK", 2
	case 3:
		return "INVTYPE_SHOULDER", 3
	case 4:
		return "INVTYPE_BODY", 4
	case 5, 20:
		return "INVTYPE_CHEST", 5
	case 6:
		return "INVTYPE_WAIST", 6
	case 7:
		return "INVTYPE_LEGS", 7
	case 8:
		return "INVTYPE_FEET", 8
	case 9:
		return "INVTYPE_WRIST", 9
	case 10:
		return "INVTYPE_HAND", 10
	case 11:
		return "INVTYPE_FINGER", 11
	case 12:
		return "INVTYPE_TRINKET", 13
	case 13:
		return "INVTYPE_WEAPON", 16
	case 15, 26:
		return "INVTYPE_RANGEDRIGHT", 18
	case 16:
		return "INVTYPE_CLOAK", 15
	case 17:
		return "INVTYPE_2HWEAPON", 16
	case 21:
		return "INVTYPE_WEAPONMAINHAND", 16
	case 22:
		return "INVTYPE_WEAPONOFFHAND", 17
	case 23:
		return "INVTYPE_HOLDABLE", 17
	case 25:
		return "INVTYPE_THROWN", 18
	case 28:
		return "INVTYPE_RELIC", 18
	}
	return "", 0
}

func gearScoreItemScore(itemLevel float64, itemRarity int, equipLoc string) int {
	itemType, ok := gearScoreItemTypes[equipLoc]
	if !ok {
		return -1
	}
	qualityScale := 1.0
	if itemRarity == 5 {
		qualityScale = 1.3
		itemRarity = 4
	} else if itemRarity == 1 || itemRarity == 0 {
		qualityScale = 0.005
		itemRarity = 2
	}
	if itemRarity == 7 {
		itemRarity = 3
		itemLevel = 187.05
	}
	formula, ok := gearScoreFormula(itemLevel, itemRarity)
	if !ok {
		return -1
	}
	score := math.Floor(((itemLevel - formula.a) / formula.b) * itemType.slotMod * 1.8618 * qualityScale)
	if score < 0 {
		return 0
	}
	return int(score)
}

type gearScoreFormulaRow struct {
	a float64
	b float64
}

func gearScoreFormula(itemLevel float64, rarity int) (gearScoreFormulaRow, bool) {
	if itemLevel > 120 {
		switch rarity {
		case 4:
			return gearScoreFormulaRow{a: 91.4500, b: 0.6500}, true
		case 3:
			return gearScoreFormulaRow{a: 81.3750, b: 0.8125}, true
		case 2:
			return gearScoreFormulaRow{a: 73.0000, b: 1.0000}, true
		}
	} else {
		switch rarity {
		case 4:
			return gearScoreFormulaRow{a: 26.0000, b: 1.2000}, true
		case 3:
			return gearScoreFormulaRow{a: 0.7500, b: 1.8000}, true
		case 2:
			return gearScoreFormulaRow{a: 8.0000, b: 2.0000}, true
		case 1:
			return gearScoreFormulaRow{a: 0.0000, b: 2.2500}, true
		}
	}
	return gearScoreFormulaRow{}, false
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
	slugURL, err := c.normalizedRisingGodsProfileURL(ref)
	if err != nil {
		return "", err
	}
	cacheKey := "slug:" + strings.ToLower(slugURL)
	if id, ok := c.sharedCache().getSlug(cacheKey); ok {
		return id, nil
	}
	idAny, err := c.sharedCache().do(ctx, cacheKey, func() (any, error) {
		if id, ok := c.sharedCache().getSlug(cacheKey); ok {
			return id, nil
		}
		fetchCtx, cancel := detachedFetchContext(ctx)
		defer cancel()
		body, err := c.fetch(fetchCtx, slugURL)
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
	if c.ProxyToken != "" {
		req.Header.Set("X-Rising-Gods-Proxy-Token", c.ProxyToken)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrExternalUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrExternalNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: status %d", ErrExternalUnavailable, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", fmt.Errorf("%w: read response: %w", ErrExternalUnavailable, err)
	}
	return string(b), nil
}

// getSearch returns a cached search. fresh is false when the entry is past its
// TTL but still young enough to be served while a refresh runs.
func (c *risingGodsCache) getSearch(key string) (rows []ExternalCharacter, fresh bool, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.searches[key]
	now := time.Now()
	if !ok || now.After(cached.expiresAt.Add(risingGodsSearchStaleMax)) {
		return nil, false, false
	}
	rows = append([]ExternalCharacter(nil), cached.rows...)
	return rows, !now.After(cached.expiresAt), true
}

// findSearchRow looks up a profile in any cached search, stale ones included.
func (c *risingGodsCache) findSearchRow(profileID string) (ExternalCharacter, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-risingGodsSearchStaleMax)
	for _, cached := range c.searches {
		if cached.expiresAt.Before(cutoff) {
			continue
		}
		for _, row := range cached.rows {
			if row.ProfileID == profileID && row.MainSpec != "" {
				return row, true
			}
		}
	}
	return ExternalCharacter{}, false
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
	now := time.Now()
	if _, exists := c.searches[key]; !exists {
		c.pruneSearchesLocked(now)
	}
	c.searches[key] = cachedSearch{rows: append([]ExternalCharacter(nil), rows...), expiresAt: now.Add(ttl), complete: complete}
}

// pruneSearchesLocked drops searches too old to serve and, if the cache is
// still full, the entry closest to expiry. The search endpoint is public, so
// the number of cached queries must stay bounded.
func (c *risingGodsCache) pruneSearchesLocked(now time.Time) {
	cutoff := now.Add(-risingGodsSearchStaleMax)
	for key, cached := range c.searches {
		if cached.expiresAt.Before(cutoff) {
			delete(c.searches, key)
		}
	}
	for len(c.searches) >= risingGodsSearchMaxEntries {
		oldestKey := ""
		var oldest time.Time
		for key, cached := range c.searches {
			if oldestKey == "" || cached.expiresAt.Before(oldest) {
				oldestKey, oldest = key, cached.expiresAt
			}
		}
		delete(c.searches, oldestKey)
	}
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

// do runs fn once per key across concurrent callers. Each caller stops waiting
// when its own ctx is done; the shared call keeps running for the others.
func (c *risingGodsCache) do(ctx context.Context, key string, fn func() (any, error)) (any, error) {
	call := c.start(key, fn)
	select {
	case <-call.done:
		return call.val, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// start launches fn for key in the background unless a call for key is
// already running, and returns the running call.
func (c *risingGodsCache) start(key string, fn func() (any, error)) *risingGodsCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	if call, ok := c.inflight[key]; ok {
		return call
	}
	call := &risingGodsCall{done: make(chan struct{})}
	c.inflight[key] = call
	go func() {
		val, err := fn()
		c.mu.Lock()
		call.val, call.err = val, err
		delete(c.inflight, key)
		c.mu.Unlock()
		close(call.done)
	}()
	return call
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

func (c RisingGodsClient) normalizedRisingGodsProfileURL(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if strings.HasPrefix(strings.ToLower(trimmed), "eu.rising-gods.") {
		return c.upstreamURL() + "/?profile=" + url.QueryEscape(trimmed), nil
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
	return c.upstreamURL() + "/?profile=" + url.QueryEscape(profile), nil
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

func extractJSONObjectAt(body string, start int) string {
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

// factionName maps the profiler faction field, which Rising-Gods encodes as
// 0 for Alliance and 1 for Horde (checked against a Gnome and a Tauren
// profile on 2026-10-11).
func factionName(id int) string {
	if id == 0 {
		return "Alliance"
	}
	if id == 1 {
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
