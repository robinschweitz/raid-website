package legacyimport

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Sheet dates require Europe/Berlin even without host zoneinfo.

	"raid-website/internal/attendance"
	"raid-website/internal/loot"
	"raid-website/internal/validation"
)

const icecrownCitadelInstanceID = "631"

var sheetRaidKey = regexp.MustCompile(`^ICC(\d{2})-\d{2,}$`)

type PlayerPlan struct {
	Name      string
	Active    bool
	Points    int
	HasPoints bool
}

type CharacterPlan struct {
	Name       string
	PlayerName string
	Class      string
	MainSpec   string
	SecondSpec string
}

type AttendancePlan struct {
	Character string
	Player    string
	Role      validation.AttendanceRole
	Class     string
	Spec      string
	Position  int
}

type RaidPlan struct {
	ExistingID        int64
	Key               string
	Date              *time.Time
	CatalogInstanceID string
	RaidSize          int
	Entries           []AttendancePlan
}

type LootPlan struct {
	Date          *time.Time
	ExternalID    string
	Identity      string
	Fingerprint   string
	RaidKey       string
	PlayerName    string
	CharacterName string
	Item          string
	ItemID        string
	Priority      string
	Method        loot.Method
	IsToken       bool
	Boss          string
	CatalogBossID string
}

// IsMainSpec reports whether the award blocks the +10 EP archive reward. An armor token
// never blocks it, whatever priority the sheet recorded.
func (l LootPlan) IsMainSpec() bool {
	if l.IsToken {
		return false
	}
	return l.Method == loot.Main || l.Method == loot.Enhanced
}

type Plan struct {
	Setup          *SetupPlan
	RequiresReason bool
	Players        []PlayerPlan
	Characters     []CharacterPlan
	Raids          []RaidPlan
	Loot           []LootPlan
	Warnings       []string
	Errors         []string
}

// BuildPlan converts the parsed sheet tabs into a deterministic merge plan.
// It performs no database access.
func BuildPlan(raidRows []RaidArchiveRow, lootRows []LootArchiveRow, charRows []CharacterRow, pointsRows []PointsRow) Plan {
	p := Plan{Warnings: []string{}, Errors: []string{}}

	players := map[string]*PlayerPlan{}
	characters := map[string]*CharacterPlan{}
	raidMap := map[string]*RaidPlan{}

	newRaidPlan := func(key string) *RaidPlan {
		rp := &RaidPlan{Key: key}
		if instance, size, ok := catalogContext(key); ok {
			rp.CatalogInstanceID = instance
			rp.RaidSize = size
		} else {
			p.Warnings = append(p.Warnings, fmt.Sprintf("raid %q: sheet key does not encode a proven catalog instance; raid is imported without catalog context", key))
		}
		raidMap[key] = rp
		return rp
	}

	ensurePlayer := func(name string) *PlayerPlan {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		if existing, ok := players[name]; ok {
			return existing
		}
		pl := &PlayerPlan{Name: name, Active: true}
		players[name] = pl
		return pl
	}

	// 1. Punkte -> players with opening balances.
	for i, row := range pointsRows {
		name := strings.TrimSpace(row.Player)
		if name == "" {
			continue
		}
		pl := ensurePlayer(name)
		if row.Inactive != "" {
			switch strings.ToLower(strings.TrimSpace(row.Inactive)) {
			case "ja", "yes", "true", "1", "x":
				pl.Active = false
			}
		}
		if strings.TrimSpace(row.Points) != "" {
			pts, err := strconv.Atoi(strings.TrimSpace(row.Points))
			if err != nil {
				p.Warnings = append(p.Warnings, fmt.Sprintf("punkte row %d: invalid points %q for %q", i+2, row.Points, name))
			} else {
				if pl.HasPoints && pl.Points != pts {
					p.Warnings = append(p.Warnings, fmt.Sprintf("punkte row %d: conflicting points for %q (%d vs %d), keeping max", i+2, name, pl.Points, pts))
				}
				if !pl.HasPoints || pts > pl.Points {
					pl.Points = pts
					pl.HasPoints = true
				}
			}
		}
	}

	// 2. Charaktere -> characters.
	for i, row := range charRows {
		name := strings.TrimSpace(row.Character)
		if name == "" {
			continue
		}
		playerName := strings.TrimSpace(row.Player)
		if playerName == "" {
			playerName = name
			p.Warnings = append(p.Warnings, fmt.Sprintf("charaktere row %d: %q without player, using character name", i+2, name))
		}
		ensurePlayer(playerName)
		c := characters[name]
		if c == nil {
			c = &CharacterPlan{Name: name, PlayerName: playerName}
			characters[name] = c
		}
		if c.PlayerName != playerName {
			p.Errors = append(p.Errors, fmt.Sprintf("character %q has conflicting owners", name))
		}
		if c.Class == "" {
			c.Class = validation.NormalizeClassName(row.Class)
		}
		if c.MainSpec == "" {
			c.MainSpec = strings.TrimSpace(row.MainSpec)
		}
		if c.SecondSpec == "" {
			c.SecondSpec = strings.TrimSpace(row.SecondSpec)
		}
	}

	// 3. Raid Archive -> raids with attendance.
	for i, row := range raidRows {
		if _, ok := parseSheetDate(row.TimestampRaw); !ok {
			p.Warnings = append(p.Warnings, fmt.Sprintf("raid row %d: missing or invalid date %q; using import time when no raid date is available", i+2, row.TimestampRaw))
		}
		key := strings.TrimSpace(row.RaidID)
		if key == "" {
			continue
		}
		rp := raidMap[key]
		if rp == nil {
			rp = newRaidPlan(key)
		}
		if rp.Date == nil {
			if d, ok := parseSheetDate(row.TimestampRaw); ok {
				rp.Date = &d
			}
		}
		character := strings.TrimSpace(row.Character)
		if character == "" {
			continue
		}
		c := characters[character]
		if c == nil {
			c = &CharacterPlan{Name: character, PlayerName: character, Class: validation.NormalizeClassName(row.Class)}
			characters[character] = c
			ensurePlayer(character)
			p.Warnings = append(p.Warnings, fmt.Sprintf("raid row %d: character %q not in Charaktere, created with own player", i+2, character))
		}
		role, ok := mapRole(row.Role)
		if !ok {
			role = validation.RoleDD
			p.Warnings = append(p.Warnings, fmt.Sprintf("raid row %d: unknown role %q for %q, defaulting to DD", i+2, row.Role, character))
		}
		pos := 0
		if v, err := strconv.Atoi(strings.TrimSpace(row.Position)); err == nil {
			pos = v
		}
		class := attendance.NormalizeClass(row.Class)
		if _, valid := validation.ClassSpecs[class]; !valid {
			p.Warnings = append(p.Warnings, fmt.Sprintf("raid row %d: missing or unknown roster class %q for %q; source value preserved, not replaced from Charaktere", i+2, row.Class, character))
		}
		if strings.TrimSpace(row.Spec) == "" {
			p.Warnings = append(p.Warnings, fmt.Sprintf("raid row %d: missing roster spec for %q; historical spec remains unknown", i+2, character))
		}
		rp.Entries = append(rp.Entries, AttendancePlan{
			Character: character,
			Player:    c.PlayerName,
			Role:      role,
			Class:     class,
			Spec:      strings.TrimSpace(row.Spec),
			Position:  pos,
		})
	}

	// IDs are checked even on incomplete rows; invalid rows cannot hide duplicates.
	seenIDs := map[string]bool{}
	occurrences := map[string]int{}
	// 4. Loot Archive -> loot awards.
	for i, row := range lootRows {
		var date *time.Time
		if parsed, ok := parseSheetDate(row.TimestampRaw); ok {
			date = &parsed
		} else {
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: missing or invalid date %q; using import time for award", i+2, row.TimestampRaw))
		}
		externalID := strings.TrimSpace(row.ImportID)
		if externalID != "" {
			if seenIDs[externalID] {
				p.Errors = append(p.Errors, fmt.Sprintf("loot row %d: duplicate Import ID %q", i+2, externalID))
			}
			seenIDs[externalID] = true
		}
		characterName := strings.TrimSpace(row.Player)
		item := strings.TrimSpace(row.Loot)
		if characterName == "" || item == "" {
			if characterName != "" || item != "" || externalID != "" {
				p.Errors = append(p.Errors, fmt.Sprintf("loot row %d: missing character or item", i+2))
			}
			continue
		}
		c := characters[characterName]
		if c == nil {
			c = &CharacterPlan{Name: characterName, PlayerName: characterName}
			characters[characterName] = c
			ensurePlayer(characterName)
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: character %q not in Charaktere or Raid Archive, created with own player", i+2, characterName))
		}
		key := strings.TrimSpace(row.RaidID)
		if key == "" {
			key = "LEGACY-LOOT"
		}
		if _, ok := raidMap[key]; !ok {
			rp := newRaidPlan(key)
			if d, ok := parseSheetDate(row.TimestampRaw); ok {
				rp.Date = &d
			}
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: raid %q only present in Loot Archive", i+2, key))
		}
		priority := strings.TrimSpace(row.Priority)
		method, known := sheetPriority(priority)
		if !known {
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: unknown priority %q for item %q; recorded as SECOND, which never blocks +10 EP", i+2, priority, item))
		}
		isToken := loot.IsTokenAward(item)
		if isToken && method != loot.Second {
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: armor token %q recorded with priority %q; tokens never block +10 EP, recorded as SECOND", i+2, item, priority))
			method = loot.Second
		}
		if !isToken && strings.EqualFold(priority, "token") {
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: priority Token but %q is not an armor token; no token recorded", i+2, item))
		}
		// Keep source fingerprints stable across recipient resolution changes.
		content, _ := json.Marshal([]string{row.RaidID, row.TimestampRaw, characterName, item, row.ItemID, priority})
		fingerprint := fmt.Sprintf("%x", sha256.Sum256(content))
		var identity string
		if externalID != "" {
			identity = fmt.Sprintf("id:%x", sha256.Sum256([]byte(externalID)))
		} else {
			occurrences[fingerprint]++
			identity = fmt.Sprintf("content:%s:%d", fingerprint, occurrences[fingerprint])
		}
		p.Loot = append(p.Loot, LootPlan{
			Date:       date,
			ExternalID: externalID, Identity: identity, Fingerprint: fingerprint,
			RaidKey:       key,
			PlayerName:    c.PlayerName,
			CharacterName: characterName,
			Item:          item,
			ItemID:        strings.TrimSpace(row.ItemID),
			Priority:      priority,
			IsToken:       isToken,
			Method:        method,
		})
	}

	// Materialize players.
	for _, pl := range players {
		p.Players = append(p.Players, *pl)
	}
	sort.Slice(p.Players, func(i, j int) bool { return p.Players[i].Name < p.Players[j].Name })

	for _, c := range characters {
		p.Characters = append(p.Characters, *c)
	}
	sort.Slice(p.Characters, func(i, j int) bool { return p.Characters[i].Name < p.Characters[j].Name })

	for _, rp := range raidMap {
		sort.Slice(rp.Entries, func(i, j int) bool { return rp.Entries[i].Position < rp.Entries[j].Position })
		p.Raids = append(p.Raids, *rp)
	}
	sort.Slice(p.Raids, func(i, j int) bool { return p.Raids[i].Key < p.Raids[j].Key })

	return p
}

// sheetPriority maps the sheet's Priority column onto a recorded award method. Only the
// sheet values that exist are translated; an unknown value is reported by the caller and
// recorded as SECOND so it never silently counts as main-spec loot.
func sheetPriority(priority string) (loot.Method, bool) {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "main":
		return loot.Main, true
	case "second":
		return loot.Second, true
	case "enhanced":
		return loot.Enhanced, true
	case "boe":
		return loot.BOE, true
	case "token":
		return loot.Second, true
	case "":
		return loot.Second, true
	default:
		return loot.Second, false
	}
}

// catalogContext decodes a sheet raid key into the bundled catalog context. The key
// carries the instance and the raid size ("ICC25-07"). Only that proven shape is decoded;
// any other key leaves the raid unassociated instead of inventing a context.
func catalogContext(key string) (string, int, bool) {
	match := sheetRaidKey.FindStringSubmatch(strings.TrimSpace(key))
	if match == nil {
		return "", 0, false
	}
	size, err := strconv.Atoi(match[1])
	if err != nil || size != 10 && size != 25 {
		return "", 0, false
	}
	return icecrownCitadelInstanceID, size, true
}

func mapRole(s string) (validation.AttendanceRole, bool) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "tank", "tanks":
		return validation.RoleTank, true
	case "heiler", "heal", "healer", "heals", "hiler":
		return validation.RoleHeal, true
	case "dd", "dps", "damage", "dmg", "mdd", "rdd":
		return validation.RoleDD, true
	}
	if strings.Contains(v, "dd") {
		return validation.RoleDD, true
	}
	return "", false
}

func parseSheetDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{"02.01.2006", "2.1.2006", "02.01.06", "2006-01-02"}
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.Time{}, false
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, location); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
