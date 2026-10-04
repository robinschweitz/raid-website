package sourceexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/lootcatalog"
)

// Manifest contains explicitly reviewed associations, not guesses based on item names.
// Evidence must identify the pinned upstream script establishing a chest/boss or size mapping.
type Manifest struct {
	Revision         string      `json:"revision"`
	Instances        []Instance  `json:"instances"`
	Bosses           []Boss      `json:"bosses"`
	Roots            []Root      `json:"roots"`
	Notices          []string    `json:"notices"`
	CreatureVariants bool        `json:"creature_variants,omitempty"`
	Proofs           []Proof     `json:"proofs,omitempty"`
	BossGroups       []BossGroup `json:"boss_groups,omitempty"`
	Encounters       []Encounter `json:"encounters,omitempty"`
}

// Encounter groups source participants under one selectable fight or reward.
type Encounter struct {
	ID           string   `json:"id"`
	InstanceID   string   `json:"instance_id"`
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

// BossGroup resolves explicit encounter symbols or script names from pinned source data.
// Scripts are useful where a header omits numeric NPC IDs, as in Naxxramas.
type BossGroup struct {
	InstanceID string   `json:"instance_id"`
	Evidence   string   `json:"evidence"`
	Symbols    []string `json:"symbols,omitempty"`
	Scripts    []string `json:"scripts,omitempty"`
}

// Proof records source assertions checked at the pinned revision, not human approval.
type Proof struct {
	Path     string   `json:"path"`
	Patterns []string `json:"patterns"`
}

// Instance supplies a reviewed display name and verifies the source instance script.
type Instance struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

// Boss ties a source creature to a reviewed instance association.
type Boss struct {
	ID         string `json:"id"`
	InstanceID string `json:"instance_id"`
	Evidence   string `json:"evidence"`
	Symbol     string `json:"symbol,omitempty"`
	ScriptName string `json:"script_name,omitempty"`
}

// Root identifies a creature variant or chest loot source with reviewed size metadata.
type Root struct {
	Kind            string `json:"kind"`
	Entry           string `json:"entry"`
	BossID          string `json:"boss_id"`
	RaidSize        int    `json:"raid_size"`
	Difficulty      string `json:"difficulty"`
	Evidence        string `json:"evidence"`
	SpawnDifficulty *int   `json:"spawn_difficulty,omitempty"`
	SpellID         string `json:"spell_id,omitempty"`
}

// Export fetches only a pinned revision, parses data as text, and emits a normalized subset.
// Network access belongs exclusively to the standalone export command, not the application.
func Export(ctx context.Context, manifest Manifest) (lootcatalog.Dataset, string, error) {
	client := &http.Client{Timeout: 3 * time.Minute}
	return Normalize(manifest, func(path string) (string, error) {
		url := "https://raw.githubusercontent.com/azerothcore/azerothcore-wotlk/" + manifest.Revision + "/" + path
		if path == clientSpellArchive {
			url = clientSpellArchive
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		response, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return "", fmt.Errorf("fetch %s: HTTP %d", path, response.StatusCode)
		}
		content, err := io.ReadAll(io.LimitReader(response.Body, (128<<20)+1))
		if err != nil {
			return "", err
		}
		if len(content) > 128<<20 {
			return "", fmt.Errorf("source file too large: %s", path)
		}
		return string(content), nil
	})
}

// Normalize extracts a reviewed manifest from supplied pinned source files.
// The reader seam permits entirely offline fixture tests and local source archives.
func Normalize(manifest Manifest, read func(string) (string, error)) (lootcatalog.Dataset, string, error) {
	empty := lootcatalog.Dataset{}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(manifest.Revision) {
		return empty, "", fmt.Errorf("a full pinned commit SHA is required")
	}
	evidencePrefix := "https://github.com/azerothcore/azerothcore-wotlk/blob/" + manifest.Revision + "/"
	for _, b := range manifest.Bosses {
		if !strings.HasPrefix(b.Evidence, evidencePrefix) {
			return empty, "", fmt.Errorf("boss %s requires pinned source evidence", b.ID)
		}
	}
	for _, root := range manifest.Roots {
		if !strings.HasPrefix(root.Evidence, evidencePrefix) || root.RaidSize != 10 && root.RaidSize != 25 || root.Difficulty == "" {
			return empty, "", fmt.Errorf("loot root %s requires reviewed size/difficulty and pinned evidence", root.Entry)
		}
	}
	d := lootcatalog.Dataset{Source: lootcatalog.Source{ID: "azerothcore-wotlk", Revision: manifest.Revision, URL: "https://github.com/azerothcore/azerothcore-wotlk/tree/" + manifest.Revision, License: "GPL-2.0", Attribution: "AzerothCore contributors; see upstream AUTHORS and the separately delivered LICENSE. Normalized subset; not the original world database."}, Notices: append([]string{}, manifest.Notices...), Instances: []lootcatalog.Instance{}, Bosses: []lootcatalog.Boss{}, Items: []lootcatalog.Item{}, Drops: []lootcatalog.Drop{}}
	d.Notices = append(d.Notices, "Quelle: Original-WotLK 3.3.5 AzerothCore; keine Wrath-Classic-Daten. Keine vollstaendige Dropwahrscheinlichkeit oder Kill-Historie. Varianten und Truhenzuordnungen stammen aus dem geprueften Manifest.")
	if manifest.CreatureVariants {
		d.Notices[len(d.Notices)-1] = "Quelle: Original-WotLK 3.3.5 AzerothCore. Source-gepruefte Teilmenge, keine menschliche Freigabe und kein vollstaendiger Produktionskatalog. Nur vorhandene DifficultyEntry-Varianten mit expliziter LootID werden exportiert; fehlende Varianten werden nicht erfunden."
	}
	fetch := func(path string) (string, error) {
		content, err := read(path)
		if err != nil {
			return "", err
		}
		if len(content) > 128<<20 {
			return "", fmt.Errorf("source file too large: %s", path)
		}
		hash := sha256.Sum256([]byte(content))
		d.Notices = append(d.Notices, path+" sha256:"+hex.EncodeToString(hash[:]))
		return content, nil
	}
	license, err := fetch("LICENSE")
	if err != nil {
		return empty, "", err
	}
	if !strings.Contains(license, "Version 2, June 1991") {
		return empty, "", fmt.Errorf("upstream license changed; owner review required")
	}
	authors, err := fetch("AUTHORS")
	if err != nil {
		return empty, "", err
	}
	d.Notices = append(d.Notices, authors)
	proofCache := map[string]string{}
	proofRead := func(path string) (string, error) {
		if content, ok := proofCache[path]; ok {
			return content, nil
		}
		if (!strings.HasPrefix(path, "src/server/") && !strings.HasPrefix(path, "data/sql/updates/db_world/")) || strings.Contains(path, "..") {
			return "", fmt.Errorf("invalid proof path %q", path)
		}
		content, err := fetch(path)
		if err != nil {
			return "", err
		}
		proofCache[path] = content
		return content, nil
	}
	for _, proof := range manifest.Proofs {
		content, err := proofRead(proof.Path)
		if err != nil {
			return empty, "", err
		}
		for _, pattern := range proof.Patterns {
			expression, err := regexp.Compile(pattern)
			if err != nil {
				return empty, "", fmt.Errorf("invalid proof pattern: %w", err)
			}
			if !expression.MatchString(content) {
				return empty, "", fmt.Errorf("source assertion failed: %s / %s", proof.Path, pattern)
			}
		}
	}
	if manifest.CreatureVariants {
		enums, err := proofRead("src/server/shared/DataStores/DBCEnums.h")
		if err != nil {
			return empty, "", err
		}
		for mode, symbol := range []string{"RAID_DIFFICULTY_10MAN_NORMAL", "RAID_DIFFICULTY_25MAN_NORMAL", "RAID_DIFFICULTY_10MAN_HEROIC", "RAID_DIFFICULTY_25MAN_HEROIC"} {
			if !regexp.MustCompile(symbol + `\s*=\s*` + strconv.Itoa(mode) + `\s*,`).MatchString(enums) {
				return empty, "", fmt.Errorf("source raid difficulty mapping changed")
			}
		}
		creatureCode, err := proofRead("src/server/game/Entities/Creature/Creature.cpp")
		if err != nil {
			return empty, "", err
		}
		if !strings.Contains(creatureCode, "normalInfo->DifficultyEntry[diff - 1]") {
			return empty, "", fmt.Errorf("source creature difficulty selection changed")
		}
	}
	manifest.Bosses = append([]Boss{}, manifest.Bosses...)
	scriptGroups := map[string]BossGroup{}
	for _, group := range manifest.BossGroups {
		if !strings.HasPrefix(group.Evidence, evidencePrefix) {
			return empty, "", fmt.Errorf("boss group requires pinned evidence")
		}
		path := strings.TrimPrefix(group.Evidence, evidencePrefix)
		content, err := proofRead(path)
		if err != nil {
			return empty, "", err
		}
		for _, symbol := range group.Symbols {
			match := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(symbol) + `\s*=\s*([0-9]+)\s*,`).FindStringSubmatch(content)
			if len(match) != 2 {
				return empty, "", fmt.Errorf("missing encounter enum %s in %s", symbol, path)
			}
			manifest.Bosses = append(manifest.Bosses, Boss{ID: match[1], InstanceID: group.InstanceID, Evidence: group.Evidence, Symbol: symbol})
		}
		for _, script := range group.Scripts {
			if _, duplicate := scriptGroups[script]; duplicate {
				return empty, "", fmt.Errorf("duplicate script boss %s", script)
			}
			if !regexp.MustCompile(`^boss_[a-z0-9_]+$`).MatchString(script) {
				return empty, "", fmt.Errorf("invalid script boss")
			}
			scriptGroups[script] = group
		}
	}
	parse := func(table string, visit func(map[string]string) error) error {
		content, err := fetch("data/sql/base/db_world/" + table + ".sql")
		if err != nil {
			return err
		}
		return Parse(content, table, visit)
	}
	wantedInstances := map[string]Instance{}
	for _, i := range manifest.Instances {
		if _, ok := wantedInstances[i.ID]; ok {
			return empty, "", fmt.Errorf("duplicate manifest instance")
		}
		wantedInstances[i.ID] = i
	}
	if err := parse("instance_template", func(row map[string]string) error {
		if i, ok := wantedInstances[row["map"]]; ok {
			if i.Script != row["script"] {
				return fmt.Errorf("instance %s script mismatch", i.ID)
			}
			d.Instances = append(d.Instances, lootcatalog.Instance{ID: i.ID, Name: i.Name})
		}
		return nil
	}); err != nil {
		return empty, "", err
	}
	if len(d.Instances) != len(wantedInstances) {
		return empty, "", fmt.Errorf("manifest instance missing from source")
	}
	wantedCreatures := map[string]bool{}
	for _, b := range manifest.Bosses {
		wantedCreatures[b.ID] = true
	}
	for _, root := range manifest.Roots {
		if root.Kind == "creature" {
			wantedCreatures[root.Entry] = true
		}
	}
	creatures := map[string]map[string]string{}
	resolvedScripts := map[string]bool{}
	creatureSQL, err := fetch("data/sql/base/db_world/creature_template.sql")
	if err != nil {
		return empty, "", err
	}
	if err := Parse(creatureSQL, "creature_template", func(row map[string]string) error {
		if group, ok := scriptGroups[row["scriptname"]]; ok {
			script := row["scriptname"]
			if resolvedScripts[script] {
				return fmt.Errorf("ambiguous source script boss %s", script)
			}
			resolvedScripts[script] = true
			path := strings.TrimPrefix(group.Evidence, evidencePrefix)
			path = path[:strings.LastIndex(path, "/")+1] + script + ".cpp"
			content, err := proofRead(path)
			if err != nil {
				return err
			}
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(script) + `\b`).MatchString(content) {
				return fmt.Errorf("script boss missing from source file")
			}
			manifest.Bosses = append(manifest.Bosses, Boss{ID: row["entry"], InstanceID: group.InstanceID, Evidence: evidencePrefix + path, ScriptName: script})
			wantedCreatures[row["entry"]] = true
		}
		if wantedCreatures[row["entry"]] {
			creatures[row["entry"]] = row
		}
		return nil
	}); err != nil {
		return empty, "", err
	}
	if len(resolvedScripts) != len(scriptGroups) {
		return empty, "", fmt.Errorf("source script boss missing: resolved %d of %d", len(resolvedScripts), len(scriptGroups))
	}
	if manifest.CreatureVariants {
		for _, boss := range manifest.Bosses {
			for _, column := range []string{"difficulty_entry_1", "difficulty_entry_2", "difficulty_entry_3"} {
				id := creatures[boss.ID][column]
				if id != "" && id != "0" {
					wantedCreatures[id] = true
				}
			}
		}
		if err := Parse(creatureSQL, "creature_template", func(row map[string]string) error {
			if wantedCreatures[row["entry"]] {
				creatures[row["entry"]] = row
			}
			return nil
		}); err != nil {
			return empty, "", err
		}
	}
	for _, b := range manifest.Bosses {
		row, ok := creatures[b.ID]
		if !ok {
			return empty, "", fmt.Errorf("boss %s missing", b.ID)
		}
		if b.ScriptName != "" && row["scriptname"] != b.ScriptName {
			return empty, "", fmt.Errorf("boss %s source ScriptName mismatch", b.ID)
		}
		if b.Symbol != "" {
			path := strings.TrimPrefix(b.Evidence, evidencePrefix)
			content, err := proofRead(path)
			if err != nil {
				return empty, "", err
			}
			if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(b.Symbol) + `\s*=\s*` + regexp.QuoteMeta(b.ID) + `\s*,`).MatchString(content) {
				return empty, "", fmt.Errorf("boss %s source enum mismatch", b.ID)
			}
		}
		d.Bosses = append(d.Bosses, lootcatalog.Boss{ID: b.ID, InstanceID: b.InstanceID, Name: row["name"]})
		d.Notices = append(d.Notices, "boss "+b.ID+" evidence: "+b.Evidence)
	}
	type lootRow struct {
		item, reference string
		mode            int
	}
	loot := map[string]map[string][]lootRow{}
	for _, table := range []string{"creature_loot_template", "gameobject_loot_template", "reference_loot_template"} {
		loot[table] = map[string][]lootRow{}
		if err := parse(table, func(row map[string]string) error {
			mode, err := strconv.Atoi(row["lootmode"])
			if err != nil || mode < 1 || mode > 65535 {
				return fmt.Errorf("invalid source loot mode")
			}
			loot[table][row["entry"]] = append(loot[table][row["entry"]], lootRow{row["item"], row["reference"], mode})
			return nil
		}); err != nil {
			return empty, "", err
		}
	}
	if manifest.CreatureVariants {
		manifest.Roots = append([]Root{}, manifest.Roots...)
		for _, boss := range manifest.Bosses {
			base := creatures[boss.ID]
			for mode, id := range []string{boss.ID, base["difficulty_entry_1"], base["difficulty_entry_2"], base["difficulty_entry_3"]} {
				if id == "" || id == "0" {
					continue
				}
				row, ok := creatures[id]
				if !ok {
					return empty, "", fmt.Errorf("boss %s missing difficulty creature %s", boss.ID, id)
				}
				if _, ok := loot["creature_loot_template"][row["lootid"]]; !ok {
					d.Notices = append(d.Notices, fmt.Sprintf("OHNE CORPSE-DROPS: boss %s %s, Difficulty %d, creature %s, source LootID %s hat keine Tabelle. Kein Fallback, keine erfundene Beziehung.", boss.ID, base["name"], mode, id, row["lootid"]))
					continue
				}
				size := 10
				if mode == 1 || mode == 3 {
					size = 25
				}
				manifest.Roots = append(manifest.Roots, Root{Kind: "creature", Entry: id, BossID: boss.ID, RaidSize: size, Difficulty: fmt.Sprintf("raid-difficulty:%d/source-creature:%s", mode, id), Evidence: evidencePrefix + "src/server/game/Entities/Creature/Creature.cpp"})
			}
		}
	}
	wantedObjects := map[string]bool{}
	for _, root := range manifest.Roots {
		if root.Kind == "gameobject" {
			wantedObjects[root.Entry] = true
		}
	}
	if err := verifyChestSpells(manifest.Roots, read); err != nil {
		return empty, "", err
	}
	for _, root := range manifest.Roots {
		if root.SpellID != "" {
			d.Notices = append(d.Notices, "Chest spell evidence: "+clientSpellArchive+" sha256:"+clientSpellHash)
			break
		}
	}
	objects := map[string]map[string]string{}
	if len(wantedObjects) != 0 {
		if err := parse("gameobject_template", func(row map[string]string) error {
			if wantedObjects[row["entry"]] {
				objects[row["entry"]] = row
			}
			return nil
		}); err != nil {
			return empty, "", err
		}
	}
	spawnRoots := map[string]Root{}
	for _, root := range manifest.Roots {
		if root.SpawnDifficulty != nil {
			mode := *root.SpawnDifficulty
			if mode < 0 || mode > 3 || root.Kind != "gameobject" {
				return empty, "", fmt.Errorf("invalid static chest difficulty")
			}
			size := 10
			if mode == 1 || mode == 3 {
				size = 25
			}
			if root.RaidSize != size {
				return empty, "", fmt.Errorf("static chest raid size contradicts source difficulty")
			}
			spawnRoots[root.Entry] = root
		}
	}
	if len(spawnRoots) != 0 {
		bossInstances := map[string]string{}
		for _, boss := range d.Bosses {
			bossInstances[boss.ID] = boss.InstanceID
		}
		found := map[string]bool{}
		if err := parse("gameobject", func(row map[string]string) error {
			root, ok := spawnRoots[row["id"]]
			if !ok {
				return nil
			}
			mask, err := strconv.Atoi(row["spawnmask"])
			if err != nil {
				return err
			}
			if row["map"] == bossInstances[root.BossID] && mask&(1<<*root.SpawnDifficulty) != 0 {
				found[root.Entry] = true
			}
			return nil
		}); err != nil {
			return empty, "", err
		}
		for entry := range spawnRoots {
			if !found[entry] {
				return empty, "", fmt.Errorf("static chest %s has no source map/difficulty spawn", entry)
			}
		}
	}
	wantedItems := map[string]bool{}
	seenDrops := map[string]bool{}
	for _, root := range manifest.Roots {
		table, entry := "", ""
		switch root.Kind {
		case "creature":
			row, ok := creatures[root.Entry]
			if !ok {
				return empty, "", fmt.Errorf("creature root %s missing", root.Entry)
			}
			base, ok := creatures[root.BossID]
			if !ok {
				return empty, "", fmt.Errorf("unknown root boss")
			}
			if root.Entry != root.BossID && root.Entry != base["difficulty_entry_1"] && root.Entry != base["difficulty_entry_2"] && root.Entry != base["difficulty_entry_3"] {
				return empty, "", fmt.Errorf("root is not a source boss variant")
			}
			table, entry = "creature_loot_template", row["lootid"]
		case "gameobject":
			row, ok := objects[root.Entry]
			if !ok || row["type"] != "3" {
				return empty, "", fmt.Errorf("gameobject root must be a source chest")
			}
			table, entry = "gameobject_loot_template", row["data1"]
		default:
			return empty, "", fmt.Errorf("unsupported loot root kind")
		}
		d.Notices = append(d.Notices, root.Kind+" "+root.Entry+" evidence: "+root.Evidence)
		var expand func(string, string, []string, map[string]bool) error
		expand = func(table, entry string, modes []string, stack map[string]bool) error {
			key := table + ":" + entry
			if stack[key] {
				return fmt.Errorf("cyclic loot reference %s", key)
			}
			if len(stack) > 64 {
				return fmt.Errorf("loot reference depth exceeded")
			}
			rows, ok := loot[table][entry]
			if !ok {
				return fmt.Errorf("missing loot reference %s", key)
			}
			stack[key] = true
			defer delete(stack, key)
			for _, row := range rows {
				path := append(append([]string{}, modes...), strconv.Itoa(row.mode))
				if row.reference != "0" {
					if err := expand("reference_loot_template", strings.TrimPrefix(row.reference, "-"), path, stack); err != nil {
						return err
					}
					continue
				}
				// Keep every reference-path mode, since intersecting masks would lose mixed hardmodes.
				drop := lootcatalog.Drop{BossID: root.BossID, ItemID: row.item, RaidSize: root.RaidSize, Difficulty: root.Difficulty + " / source-lootmode-path:" + strings.Join(path, "/"), LootMode: row.mode}
				key := fmt.Sprintf("%+v", drop)
				if !seenDrops[key] {
					d.Drops = append(d.Drops, drop)
					seenDrops[key] = true
				}
				wantedItems[row.item] = true
			}
			return nil
		}
		if err := expand(table, entry, nil, map[string]bool{}); err != nil {
			return empty, "", err
		}
	}
	if err := parse("item_template", func(row map[string]string) error {
		if !wantedItems[row["entry"]] {
			return nil
		}
		binding, err := strconv.Atoi(row["bonding"])
		if err != nil {
			return err
		}
		quality, err := strconv.Atoi(row["quality"])
		if err != nil {
			return err
		}
		d.Items = append(d.Items, lootcatalog.Item{ItemID: row["entry"], Name: row["name"], Binding: &binding, Quality: &quality})
		return nil
	}); err != nil {
		return empty, "", err
	}
	localized := map[string]string{}
	if err := parse("item_template_locale", func(row map[string]string) error {
		if wantedItems[row["id"]] && row["locale"] == "deDE" && row["name"] != "" {
			localized[row["id"]] = row["name"]
		}
		return nil
	}); err != nil {
		return empty, "", err
	}
	for index := range d.Items {
		if name, ok := localized[d.Items[index].ItemID]; ok {
			d.Items[index].Name = name
		} else {
			d.Notices = append(d.Notices, "item "+d.Items[index].ItemID+": deutscher Name fehlt; Source-Name erhalten")
		}
	}
	if err := groupEncounters(&d, manifest.Encounters); err != nil {
		return empty, "", err
	}
	d, report := lootcatalog.Canonical(d)
	if len(report.Errors) != 0 {
		return empty, "", fmt.Errorf("invalid normalized catalog: %v", report.Errors)
	}
	return d, license, nil
}
