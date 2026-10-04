package sourceexport

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeCreatureChestReferenceAndMixedModesOffline(t *testing.T) {
	sha := strings.Repeat("a", 40)
	evidence := "https://github.com/azerothcore/azerothcore-wotlk/blob/" + sha + "/reviewed-script.cpp"
	m := Manifest{Revision: sha, Instances: []Instance{{ID: "1", Name: "Test instance", Script: "test_instance"}}, Bosses: []Boss{{ID: "2", InstanceID: "1", Evidence: evidence}}, Roots: []Root{{Kind: "creature", Entry: "2", BossID: "2", RaidSize: 10, Difficulty: "source-base", Evidence: evidence}, {Kind: "gameobject", Entry: "4", BossID: "2", RaidSize: 10, Difficulty: "source-chest", Evidence: evidence}}}
	sql := func(table, columns, rows string) string {
		return "CREATE TABLE `" + table + "` (" + columns + "); INSERT INTO `" + table + "` VALUES " + rows + ";"
	}
	files := map[string]string{
		"LICENSE":                  "GNU GENERAL PUBLIC LICENSE Version 2, June 1991",
		"AUTHORS":                  "Synthetic offline exporter fixture, not original source data",
		"instance_template":        sql("instance_template", "`map` int, `script` text", "(1,'test_instance')"),
		"creature_template":        sql("creature_template", "`entry` int, `name` text, `lootid` int, `difficulty_entry_1` int, `difficulty_entry_2` int, `difficulty_entry_3` int", "(2,'Test boss',20,0,0,0)"),
		"creature_loot_template":   sql("creature_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(20,0,30,1)"),
		"gameobject_template":      sql("gameobject_template", "`entry` int, `type` int, `data1` int", "(4,3,40)"),
		"gameobject_loot_template": sql("gameobject_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(40,3,0,3)"),
		"reference_loot_template":  sql("reference_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(30,3,0,2)"),
		"item_template":            sql("item_template", "`entry` int, `name` text, `bonding` int, `quality` int", "(3,'Test item',1,4)"),
		"item_template_locale":     sql("item_template_locale", "`ID` int, `locale` text, `Name` text", "(3,'deDE','Testgegenstand')"),
	}
	read := func(path string) (string, error) {
		key := strings.TrimSuffix(strings.TrimPrefix(path, "data/sql/base/db_world/"), ".sql")
		value, ok := files[key]
		if !ok {
			return "", fmt.Errorf("missing %s", path)
		}
		return value, nil
	}
	d, license, err := Normalize(m, read)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(license, "Version 2") || len(d.Items) != 1 || d.Items[0].Name != "Testgegenstand" || len(d.Drops) != 2 {
		t.Fatalf("normalized: %+v", d)
	}
	if d.Drops[0].LootMode != 2 || !strings.HasSuffix(d.Drops[0].Difficulty, "source-lootmode-path:1/2") {
		t.Fatal("lost mixed reference mode path")
	}
	mode := 0
	m.Roots[1].SpawnDifficulty = &mode
	files["gameobject"] = sql("gameobject", "`id` int, `map` int, `spawnMask` int", "(4,1,1)")
	if _, _, err := Normalize(m, read); err != nil {
		t.Fatal(err)
	}
	files["gameobject"] = sql("gameobject", "`id` int, `map` int, `spawnMask` int", "(4,1,2)")
	if _, _, err := Normalize(m, read); err == nil || !strings.Contains(err.Error(), "map/difficulty spawn") {
		t.Fatalf("wrong source spawn accepted: %v", err)
	}
	m.Roots[1].SpawnDifficulty = nil
	m.Proofs = []Proof{{Path: "src/server/test.h", Patterns: []string{`NPC_TEST\s*=\s*2`}}}
	files["src/server/test.h"] = "enum NPC {\nNPC_TEST = 2,\n};"
	m.BossGroups = []BossGroup{{InstanceID: "1", Evidence: "https://github.com/azerothcore/azerothcore-wotlk/blob/" + sha + "/src/server/test.h", Symbols: []string{"NPC_TEST"}}}
	m.Bosses = nil
	if _, _, err := Normalize(m, read); err != nil {
		t.Fatal(err)
	}
	files["src/server/test.h"] = "enum NPC {\nNPC_TEST = 99,\n};"
	if _, _, err := Normalize(m, read); err == nil || !strings.Contains(err.Error(), "source assertion failed") {
		t.Fatalf("changed source assertion accepted: %v", err)
	}
	m.Proofs = nil
	m.BossGroups = nil
	m.Bosses = []Boss{{ID: "2", InstanceID: "1", Evidence: evidence}}
	files["reference_loot_template"] = sql("reference_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(30,0,30,1)")
	if _, _, err := Normalize(m, read); err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("cyclic reference accepted: %v", err)
	}
	files["reference_loot_template"] = sql("reference_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(99,3,0,1)")
	if _, _, err := Normalize(m, read); err == nil || !strings.Contains(err.Error(), "missing loot") {
		t.Fatalf("missing reference accepted: %v", err)
	}
}

func TestCreatureVariantsUseSourceSlotsAndNeverInventMissingModes(t *testing.T) {
	sha := strings.Repeat("a", 40)
	prefix := "https://github.com/azerothcore/azerothcore-wotlk/blob/" + sha + "/"
	sql := func(table, columns, rows string) string {
		return "CREATE TABLE `" + table + "` (" + columns + "); INSERT INTO `" + table + "` VALUES " + rows + ";"
	}
	files := map[string]string{
		"LICENSE": "GNU GENERAL PUBLIC LICENSE Version 2, June 1991", "AUTHORS": "Synthetic offline fixture",
		"src/server/shared/DataStores/DBCEnums.h":        "RAID_DIFFICULTY_10MAN_NORMAL = 0, RAID_DIFFICULTY_25MAN_NORMAL = 1, RAID_DIFFICULTY_10MAN_HEROIC = 2, RAID_DIFFICULTY_25MAN_HEROIC = 3,",
		"src/server/game/Entities/Creature/Creature.cpp": "normalInfo->DifficultyEntry[diff - 1]",
		"instance_template":                              sql("instance_template", "`map` int, `script` text", "(1,'test')"),
		"creature_template":                              sql("creature_template", "`entry` int, `name` text, `lootid` int, `difficulty_entry_1` int, `difficulty_entry_2` int, `difficulty_entry_3` int", "(2,'Test boss',20,5,6,7),(5,'Test 25 normal',50,0,0,0),(6,'Test 10 heroic',60,0,0,0),(7,'Test 25 heroic',70,0,0,0)"),
		"creature_loot_template":                         sql("creature_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(20,3,0,1),(50,3,0,1),(60,3,0,1),(70,3,0,1)"),
		"gameobject_loot_template":                       sql("gameobject_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(99,3,0,1)"),
		"reference_loot_template":                        sql("reference_loot_template", "`entry` int, `item` int, `reference` int, `lootmode` int", "(99,3,0,1)"),
		"item_template":                                  sql("item_template", "`entry` int, `name` text, `bonding` int, `quality` int", "(3,'Item',1,4)"),
		"item_template_locale":                           sql("item_template_locale", "`ID` int, `locale` text, `Name` text", "(3,'deDE','Gegenstand')"),
	}
	read := func(path string) (string, error) {
		key := strings.TrimSuffix(strings.TrimPrefix(path, "data/sql/base/db_world/"), ".sql")
		value, ok := files[key]
		if !ok {
			return "", fmt.Errorf("missing %s", path)
		}
		return value, nil
	}
	m := Manifest{Revision: sha, CreatureVariants: true, Instances: []Instance{{ID: "1", Name: "Test", Script: "test"}}, Bosses: []Boss{{ID: "2", InstanceID: "1", Evidence: prefix + "src/server/test.h"}}}
	d, _, err := Normalize(m, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Drops) != 4 {
		t.Fatalf("source variants: %+v", d.Drops)
	}
	for _, drop := range d.Drops {
		want := 10
		if strings.HasPrefix(drop.Difficulty, "raid-difficulty:1/") || strings.HasPrefix(drop.Difficulty, "raid-difficulty:3/") {
			want = 25
		}
		if drop.RaidSize != want {
			t.Fatalf("wrong source size: %+v", drop)
		}
	}
	files["creature_template"] = sql("creature_template", "`entry` int, `name` text, `lootid` int, `difficulty_entry_1` int, `difficulty_entry_2` int, `difficulty_entry_3` int", "(2,'Test boss',20,5,0,0),(5,'Test 25 normal',0,0,0,0)")
	d, _, err = Normalize(m, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Drops) != 1 || d.Drops[0].RaidSize != 10 {
		t.Fatal("invented absent variants or zero-LootID fallback")
	}
	found := false
	for _, notice := range d.Notices {
		if strings.Contains(notice, "source LootID 0") {
			found = true
		}
	}
	if !found {
		t.Fatal("unavailable source loot not reported")
	}
}

func TestParseMySQLDumpOffline(t *testing.T) {
	// Synthetic SQL exercises the upstream dump syntax without shipping world tables.
	input := "-- dump\n/*!40101 SET NAMES utf8mb4 */; CREATE TABLE `fixture` (\n`ID` int NOT NULL,\n`Name` text,\n`Mode` int, PRIMARY KEY (`ID`)\n); INSERT INTO `fixture` VALUES (1,'Bone Warden\\'s Splitter',1),(2,'a,); -- x',3),(3,'doubled ''quote'' and \\\\ slash',2),(4,'',1);"
	rows := []map[string]string{}
	if err := Parse(input, "fixture", func(row map[string]string) error { rows = append(rows, row); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0]["name"] != "Bone Warden's Splitter" || rows[1]["name"] != "a,); -- x" || rows[2]["name"] != "doubled 'quote' and \\ slash" || rows[3]["name"] != "" {
		t.Fatalf("parsed rows: %+v", rows)
	}
}

func TestParseRejectsTruncationAndUnsupportedColumnOrder(t *testing.T) {
	for _, input := range []string{
		"CREATE TABLE `t` (`id` int); INSERT INTO `t` VALUES (",
		"CREATE TABLE `t` (`id` int); INSERT INTO `t` VALUES ('unfinished);",
		"CREATE TABLE `t` (`id` int); INSERT INTO `t` VALUES (1,2);",
		"CREATE TABLE `t` (`id` int); INSERT INTO `t` (`id`) VALUES (1);",
		"/* never closed",
		"CREATE TABLE `t` (`id` int);",
	} {
		if err := Parse(input, "t", func(map[string]string) error { return nil }); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestExportRejectsUnpinnedSourceOffline(t *testing.T) {
	_, _, err := Export(t.Context(), Manifest{Revision: "master"})
	if err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("unpinned export: %v", err)
	}
}
