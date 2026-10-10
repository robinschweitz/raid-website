package lootcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	catalogdata "raid-website/scripts/data"
)

const (
	lootModePathToken = "source-lootmode-path:"
	hardmodeToken     = "hardmode:"
	// unconditionalLootPath is the only source path that proves plain loot. Every
	// other path is conditional and must stay labelled as such.
	unconditionalLootPath = "1"
)

var bundle struct {
	sync.Once
	catalog Catalog
	err     error
}

func validMode(mode Mode) bool {
	return mode == ModeNormal || mode == ModeHeroic || mode == ModeHardmode
}

// sourceGameMode normalizes one raw source relationship into a gameplay mode.
//
// The difficulty string is the only source of truth: normal and heroic come from
// the source raid difficulty, hardmode only from an explicit hardmode token. The
// loot-mode mask never implies heroic, because conditional creature loot exists
// in every difficulty. Conditional means "the source does not prove plain loot",
// never "heroic".
func sourceGameMode(drop Drop) (Mode, bool, error) {
	segments := strings.Split(drop.Difficulty, "/")
	var mode Mode
	switch strings.TrimSpace(segments[0]) {
	case "raid-difficulty:0", "raid-difficulty:1":
		mode = ModeNormal
	case "raid-difficulty:2", "raid-difficulty:3":
		mode = ModeHeroic
	default:
		return "", false, fmt.Errorf("%w: nicht unterstützte Quelldifficulty %q", ErrInvalid, drop.Difficulty)
	}
	conditional := drop.LootMode != 1
	hardmode := false
	for _, segment := range segments[1:] {
		key, _, structured := strings.Cut(strings.TrimSpace(segment), ":")
		switch {
		case strings.TrimSpace(segment) == hardmodeToken+"true":
			hardmode = true
		case strings.TrimSpace(segment) == hardmodeToken+"false":
		case strings.HasPrefix(strings.TrimSpace(segment), lootModePathToken):
			if strings.TrimPrefix(strings.TrimSpace(segment), lootModePathToken) != unconditionalLootPath {
				conditional = true
			}
		case structured && key != "source-creature" && key != "source-chest":
			// An unrecognized structured token could gate loot, so it stays conditional.
			conditional = true
		}
	}
	if hardmode {
		return ModeHardmode, true, nil
	}
	return mode, conditional, nil
}

// decodeBundled validates the compiled source file and hashes its exact bytes.
func decodeBundled(raw string) (Catalog, error) {
	var input Dataset
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		converted, convertErr := decodeLocalizedBundled(raw)
		if convertErr != nil {
			return Catalog{}, fmt.Errorf("gebündelter Katalog kann nicht gelesen werden: %w", err)
		}
		input = converted
	}
	canonical, report := Canonical(input)
	if len(report.Errors) != 0 {
		return Catalog{}, fmt.Errorf("%w: %v", ErrInvalid, report.Errors)
	}
	for _, drop := range canonical.Drops {
		if _, _, err := sourceGameMode(drop); err != nil {
			return Catalog{}, err
		}
	}
	hash := sha256.Sum256([]byte(raw))
	return Catalog{
		Sources:   []Provenance{{Source: canonical.Source, RawHash: hex.EncodeToString(hash[:]), Notices: canonical.Notices}},
		Instances: canonical.Instances,
		Bosses:    canonical.Bosses,
		Items:     canonical.Items,
		Drops:     canonical.Drops,
	}, nil
}

type localizedDataset struct {
	Meta      localizedMeta                `json:"meta"`
	Items     map[string]localizedItem     `json:"items"`
	Instances map[string]localizedInstance `json:"instances"`
}

type localizedMeta struct {
	AtlasLootRepo    string   `json:"atlasloot_repo"`
	AtlasLootBranch  string   `json:"atlasloot_branch"`
	DefaultLocale    string   `json:"default_locale"`
	Locales          []string `json:"locales"`
	GeneratedAt      string   `json:"generated_at"`
	SchemaVersion    int      `json:"schema_version"`
	ConfiguredSource []any    `json:"configured_instances"`
}

type localizedInstance struct {
	ID     string                   `json:"id"`
	Names  map[string]string        `json:"names"`
	Bosses map[string]localizedBoss `json:"bosses"`
}

type localizedBoss struct {
	ID    string                          `json:"id"`
	Names map[string]string               `json:"names"`
	Loot  map[string][]localizedLootEntry `json:"loot"`
}

type localizedLootEntry struct {
	ItemID  int    `json:"item_id"`
	TableID string `json:"table_id"`
}

type localizedItem struct {
	ID            int                          `json:"id"`
	Name          string                       `json:"name"`
	Binding       string                       `json:"binding"`
	Icon          string                       `json:"icon"`
	DisplayID     *int                         `json:"displayId"`
	Quality       *int                         `json:"quality"`
	ItemLevel     *int                         `json:"item_level"`
	RequiredLevel *int                         `json:"required_level"`
	Slot          string                       `json:"slot"`
	Type          string                       `json:"type"`
	Armor         *int                         `json:"armor"`
	Stats         map[string]int               `json:"stats"`
	Sockets       []string                     `json:"sockets"`
	SocketBonus   *SocketBonus                 `json:"socket_bonus"`
	Effects       []string                     `json:"effects"`
	Requirements  []string                     `json:"requirements"`
	Unique        bool                         `json:"unique"`
	Weapon        localizedWeapon              `json:"weapon"`
	Locales       map[string]localizedItemView `json:"locales"`
}

type localizedWeapon struct {
	MinDamage *float64 `json:"min_damage"`
	MaxDamage *float64 `json:"max_damage"`
	Speed     *float64 `json:"speed"`
	DPS       *float64 `json:"dps"`
}

type localizedItemView struct {
	Name           string `json:"name"`
	Icon           string `json:"icon"`
	TooltipHTML    string `json:"tooltip_html"`
	TooltipText    string `json:"tooltip_text"`
	FallbackLocale string `json:"fallback_locale"`
	FetchError     string `json:"fetch_error"`
}

func decodeLocalizedBundled(raw string) (Dataset, error) {
	var input localizedDataset
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(&input); err != nil {
		return Dataset{}, err
	}
	if len(input.Items) == 0 || len(input.Instances) == 0 {
		return Dataset{}, fmt.Errorf("localized catalog is empty")
	}
	defaultLocale := input.Meta.DefaultLocale
	if strings.TrimSpace(defaultLocale) == "" {
		defaultLocale = "enUS"
	}
	revision := input.Meta.GeneratedAt
	if strings.TrimSpace(revision) == "" {
		revision = input.Meta.AtlasLootBranch
	}
	if strings.TrimSpace(revision) == "" {
		revision = "unknown"
	}
	repo := input.Meta.AtlasLootRepo
	if strings.TrimSpace(repo) == "" {
		repo = "Gescht/AtlasLoot3.3.5a"
	}

	out := Dataset{
		Source: Source{
			ID:          "atlasloot-wowhead-wotlk",
			Revision:    revision,
			URL:         "https://github.com/" + repo,
			License:     "AtlasLoot + Wowhead tooltip data",
			Attribution: "AtlasLoot and Wowhead contributors",
		},
		Notices: []string{
			"Built from localized AtlasLoot/Wowhead JSON schema version " + fmt.Sprint(input.Meta.SchemaVersion),
			"Localized display locale: " + defaultLocale,
		},
	}

	for _, item := range input.Items {
		out.Items = append(out.Items, convertLocalizedItem(item, defaultLocale))
	}

	for _, instance := range input.Instances {
		instanceID := localizedInstanceID(instance.ID)
		out.Instances = append(out.Instances, Instance{ID: instanceID, Name: localizedName(instance.Names, defaultLocale, instance.ID)})
		for _, boss := range instance.Bosses {
			bossID := localizedBossID(instanceID, boss.ID)
			out.Bosses = append(out.Bosses, Boss{ID: bossID, InstanceID: instanceID, Name: localizedName(boss.Names, defaultLocale, boss.ID)})
			for difficulty, entries := range boss.Loot {
				raidSize, sourceDifficulty, ok := localizedDifficulty(difficulty)
				if !ok {
					return Dataset{}, fmt.Errorf("unsupported localized difficulty %q", difficulty)
				}
				for _, entry := range entries {
					out.Drops = append(out.Drops, Drop{
						BossID:     bossID,
						ItemID:     fmt.Sprint(entry.ItemID),
						RaidSize:   raidSize,
						Difficulty: sourceDifficulty + "/" + lootModePathToken + unconditionalLootPath,
						LootMode:   1,
					})
				}
			}
		}
	}
	return out, nil
}

func convertLocalizedItem(input localizedItem, defaultLocale string) Item {
	defaultBinding := 0
	item := Item{
		ItemID:        fmt.Sprint(input.ID),
		Name:          localizedItemName(input, defaultLocale),
		Icon:          input.Icon,
		DisplayID:     input.DisplayID,
		Binding:       &defaultBinding,
		BindingType:   input.Binding,
		Quality:       input.Quality,
		ItemLevel:     input.ItemLevel,
		RequiredLevel: input.RequiredLevel,
		Slot:          input.Slot,
		Type:          input.Type,
		Armor:         input.Armor,
		Stats:         cloneIntMap(input.Stats),
		Sockets:       append([]string{}, input.Sockets...),
		SocketBonus:   cloneSocketBonus(input.SocketBonus),
		Effects:       append([]string{}, input.Effects...),
		Requirements:  append([]string{}, input.Requirements...),
		Unique:        input.Unique,
		Locales:       convertLocalizedViews(input.Locales),
		MinDamage:     input.Weapon.MinDamage,
		MaxDamage:     input.Weapon.MaxDamage,
		DPS:           input.Weapon.DPS,
	}
	if input.Weapon.MinDamage != nil || input.Weapon.MaxDamage != nil || input.Weapon.Speed != nil || input.Weapon.DPS != nil {
		item.Weapon = &Weapon{MinDamage: input.Weapon.MinDamage, MaxDamage: input.Weapon.MaxDamage, Speed: input.Weapon.Speed, DPS: input.Weapon.DPS}
	}
	if value, ok := localizedBinding(input.Binding); ok {
		item.Binding = &value
	}
	if value, ok := input.Stats["stamina"]; ok {
		item.Stamina = &value
	}
	if value, ok := input.Stats["strength"]; ok {
		item.Strength = &value
	}
	if value, ok := input.Stats["agility"]; ok {
		item.Agility = &value
	}
	if value, ok := input.Stats["intellect"]; ok {
		item.Intellect = &value
	}
	if value, ok := input.Stats["spirit"]; ok {
		item.Spirit = &value
	}
	if value, ok := input.Stats["spell_power"]; ok {
		item.SpellPower = &value
	}
	return item
}

func convertLocalizedViews(input map[string]localizedItemView) map[string]LocalizedItemView {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]LocalizedItemView, len(input))
	for locale, view := range input {
		out[locale] = LocalizedItemView(view)
	}
	return out
}

func cloneIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneSocketBonus(input *SocketBonus) *SocketBonus {
	if input == nil {
		return nil
	}
	return &SocketBonus{Text: input.Text, Stats: cloneIntMap(input.Stats)}
}

func localizedItemName(item localizedItem, defaultLocale string) string {
	if view, ok := item.Locales[defaultLocale]; ok && strings.TrimSpace(view.Name) != "" {
		return view.Name
	}
	if view, ok := item.Locales["enUS"]; ok && strings.TrimSpace(view.Name) != "" {
		return view.Name
	}
	return item.Name
}

func localizedName(names map[string]string, defaultLocale string, fallback string) string {
	if value := strings.TrimSpace(names[defaultLocale]); value != "" {
		return value
	}
	if value := strings.TrimSpace(names["enUS"]); value != "" {
		return value
	}
	return fallback
}

func localizedBinding(binding string) (int, bool) {
	switch binding {
	case "bind_on_pickup":
		return 1, true
	case "bind_on_equip":
		return 2, true
	case "bind_to_account", "bind_to_bnet_account":
		return 4, true
	default:
		return 0, false
	}
}

func localizedDifficulty(label string) (int, string, bool) {
	switch label {
	case "10_normal":
		return 10, "raid-difficulty:0", true
	case "25_normal":
		return 25, "raid-difficulty:1", true
	case "10_heroic":
		return 10, "raid-difficulty:2", true
	case "25_heroic":
		return 25, "raid-difficulty:3", true
	default:
		return 0, "", false
	}
}

func localizedInstanceID(id string) string {
	switch id {
	case "IcecrownCitadel":
		return "631"
	default:
		return id
	}
}

func localizedBossID(instanceID string, id string) string {
	if instanceID == "631" {
		switch id {
		case "ICCLordMarrowgar":
			return "36612"
		case "ICCLadyDeathwhisper":
			return "36855"
		case "ICCGunshipBattle":
			return "encounter:631:gunship"
		case "ICCSaurfang":
			return "37813"
		case "ICCFestergut":
			return "36626"
		case "ICCRotface":
			return "36627"
		case "ICCPutricide":
			return "36678"
		case "ICCCouncil":
			return "encounter:631:blood-prince-council"
		case "ICCLanathel":
			return "37955"
		case "ICCValithria":
			return "36789"
		case "ICCSindragosa":
			return "36853"
		case "ICCLichKing":
			return "36597"
		}
	}
	return "encounter:" + instanceID + ":" + slug(id)
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

// bundledCatalog parses the embedded file once and returns a detached copy, so
// callers can never mutate the shared source.
func bundledCatalog() (Catalog, error) {
	bundle.Do(func() {
		bundle.catalog, bundle.err = decodeBundled(catalogdata.JSON())
	})
	if bundle.err != nil {
		return Catalog{}, bundle.err
	}
	detached := bundle.catalog
	detached.Sources = make([]Provenance, len(bundle.catalog.Sources))
	for i, provenance := range bundle.catalog.Sources {
		provenance.Notices = append([]string{}, provenance.Notices...)
		detached.Sources[i] = provenance
	}
	detached.Instances = append([]Instance{}, bundle.catalog.Instances...)
	detached.Bosses = append([]Boss{}, bundle.catalog.Bosses...)
	detached.Items = make([]Item, len(bundle.catalog.Items))
	for i, item := range bundle.catalog.Items {
		detached.Items[i] = cloneItem(item)
	}
	detached.Drops = append([]Drop{}, bundle.catalog.Drops...)
	return detached, nil
}

func cloneItem(item Item) Item {
	if item.Binding != nil {
		binding := *item.Binding
		item.Binding = &binding
	}
	if item.Quality != nil {
		quality := *item.Quality
		item.Quality = &quality
	}
	if item.DisplayID != nil {
		displayID := *item.DisplayID
		item.DisplayID = &displayID
	}
	if item.ItemLevel != nil {
		itemLevel := *item.ItemLevel
		item.ItemLevel = &itemLevel
	}
	if item.RequiredLevel != nil {
		requiredLevel := *item.RequiredLevel
		item.RequiredLevel = &requiredLevel
	}
	if item.ClassMask != nil {
		classMask := *item.ClassMask
		item.ClassMask = &classMask
	}
	if item.Subclass != nil {
		subclass := *item.Subclass
		item.Subclass = &subclass
	}
	if item.Armor != nil {
		armor := *item.Armor
		item.Armor = &armor
	}
	item.Stats = cloneIntMap(item.Stats)
	item.Sockets = append([]string{}, item.Sockets...)
	item.SocketBonus = cloneSocketBonus(item.SocketBonus)
	item.Effects = append([]string{}, item.Effects...)
	item.Requirements = append([]string{}, item.Requirements...)
	if item.Weapon != nil {
		item.Weapon = &Weapon{MinDamage: cloneFloat64(item.Weapon.MinDamage), MaxDamage: cloneFloat64(item.Weapon.MaxDamage), Speed: cloneFloat64(item.Weapon.Speed), DPS: cloneFloat64(item.Weapon.DPS)}
	}
	if len(item.Locales) > 0 {
		locales := make(map[string]LocalizedItemView, len(item.Locales))
		for locale, view := range item.Locales {
			locales[locale] = view
		}
		item.Locales = locales
	}
	item.Stamina = cloneInt(item.Stamina)
	item.Strength = cloneInt(item.Strength)
	item.Agility = cloneInt(item.Agility)
	item.Intellect = cloneInt(item.Intellect)
	item.Spirit = cloneInt(item.Spirit)
	item.SpellPower = cloneInt(item.SpellPower)
	item.MinDamage = cloneFloat64(item.MinDamage)
	item.MaxDamage = cloneFloat64(item.MaxDamage)
	item.DPS = cloneFloat64(item.DPS)
	return item
}

func cloneInt(input *int) *int {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}

func cloneFloat64(input *float64) *float64 {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}
