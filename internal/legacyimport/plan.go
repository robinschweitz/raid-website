package legacyimport

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/tokens"
	"raid-website/internal/validation"
)

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
	Key     string
	Date    *time.Time
	Entries []AttendancePlan
}

type LootPlan struct {
	RaidKey    string
	PlayerName string
	Item       string
	ItemID     string
	Priority   string
	IsToken    bool
	IsMainSpec bool
}

type Plan struct {
	Players    []PlayerPlan
	Characters []CharacterPlan
	Raids      []RaidPlan
	Loot       []LootPlan
	Warnings   []string
	Errors     []string
}

// BuildPlan converts the parsed sheet tabs into a deterministic merge plan.
// It performs no database access.
func BuildPlan(raidRows []RaidArchiveRow, lootRows []LootArchiveRow, charRows []CharacterRow, pointsRows []PointsRow) Plan {
	p := Plan{}

	players := map[string]*PlayerPlan{}
	characters := map[string]*CharacterPlan{}
	raidMap := map[string]*RaidPlan{}

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
		key := strings.TrimSpace(row.RaidID)
		if key == "" {
			continue
		}
		rp := raidMap[key]
		if rp == nil {
			rp = &RaidPlan{Key: key}
			raidMap[key] = rp
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
		rp.Entries = append(rp.Entries, AttendancePlan{
			Character: character,
			Player:    c.PlayerName,
			Role:      role,
			Class:     c.Class,
			Spec:      strings.TrimSpace(row.Spec),
			Position:  pos,
		})
	}

	// 4. Loot Archive -> loot awards.
	for i, row := range lootRows {
		player := strings.TrimSpace(row.Player)
		item := strings.TrimSpace(row.Loot)
		if player == "" || item == "" {
			continue
		}
		ensurePlayer(player)
		key := strings.TrimSpace(row.RaidID)
		if key == "" {
			key = "LEGACY-LOOT"
		}
		if _, ok := raidMap[key]; !ok {
			rp := &RaidPlan{Key: key}
			if d, ok := parseSheetDate(row.TimestampRaw); ok {
				rp.Date = &d
			}
			raidMap[key] = rp
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: raid %q only present in Loot Archive", i+2, key))
		}
		priority := strings.TrimSpace(row.Priority)
		isMain := false
		switch strings.ToLower(priority) {
		case "main":
			isMain = true
		case "second", "token", "":
		default:
			p.Warnings = append(p.Warnings, fmt.Sprintf("loot row %d: unknown priority %q for item %q", i+2, priority, item))
		}
		p.Loot = append(p.Loot, LootPlan{
			RaidKey:    key,
			PlayerName: player,
			Item:       item,
			ItemID:     strings.TrimSpace(row.ItemID),
			Priority:   priority,
			IsToken:    tokens.IsTokenAward(item) || strings.EqualFold(priority, "token"),
			IsMainSpec: isMain,
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
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
