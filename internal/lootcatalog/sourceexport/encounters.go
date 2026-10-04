package sourceexport

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"

	"raid-website/internal/lootcatalog"
)

const clientSpellArchive = "https://github.com/wowgaming/client-data/releases/download/dbc_sql_v1/AzerothcoreDBCToSQL.zip"
const clientSpellHash = "e524aba0bc05aa06e2b6e5421eb80a2358fec1ae27f3ff352c15b2b75081737e"

// Only the small, checksum-pinned archive is fetched. SQL is never executed.
func verifyChestSpells(roots []Root, read func(string) (string, error)) error {
	wanted := map[string]bool{}
	for _, root := range roots {
		if root.SpellID != "" {
			wanted[root.SpellID] = true
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	raw, err := read(clientSpellArchive)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) != clientSpellHash {
		return fmt.Errorf("client spell archive checksum mismatch")
	}
	z, err := zip.NewReader(bytes.NewReader([]byte(raw)), int64(len(raw)))
	if err != nil {
		return err
	}
	spells := map[string]map[string]string{}
	difficulties := "CREATE TABLE spelldifficulty_dbc (`id` int, `normal10` int, `normal25` int, `heroic10` int, `heroic25` int);\n"
	for _, file := range z.File {
		isSpell := file.Name == "AzerothcoreDBCToSQL/spell_dbc.sql"
		isDifficulty := file.Name == "AzerothcoreDBCToSQL/spelldifficulty_dbc.sql"
		if !isSpell && !isDifficulty {
			continue
		}
		r, err := file.Open()
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(io.LimitReader(r, 1<<30))
		scanner.Buffer(make([]byte, 64*1024), 2<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if isDifficulty && strings.HasPrefix(line, "INSERT INTO") {
				difficulties += line + "\n"
			}
			if isSpell {
				_, values, ok := strings.Cut(line, " VALUES (")
				if !ok {
					continue
				}
				id, _, _ := strings.Cut(values, ",")
				if !wanted[id] {
					continue
				}
				if err := Parse(line, "spell_dbc", func(row map[string]string) error { spells[row["id"]] = row; return nil }); err != nil {
					_ = r.Close()
					return err
				}
			}
		}
		scanErr := scanner.Err()
		closeErr := r.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	modes := map[string]map[string]string{}
	if err := Parse(difficulties, "spelldifficulty_dbc", func(row map[string]string) error { modes[row["id"]] = row; return nil }); err != nil {
		return err
	}
	for _, root := range roots {
		if root.SpellID == "" {
			continue
		}
		spell := spells[root.SpellID]
		if root.Kind != "gameobject" || spell["effect_1"] != "76" || spell["effectmiscvalue_1"] != root.Entry {
			return fmt.Errorf("spell %s does not summon chest %s", root.SpellID, root.Entry)
		}
		if id := spell["spelldifficultyid"]; id != "0" {
			mode := strings.Split(root.Difficulty, "/")[0]
			column := map[string]string{"raid-difficulty:0": "normal10", "raid-difficulty:1": "normal25", "raid-difficulty:2": "heroic10", "raid-difficulty:3": "heroic25"}[mode]
			if column == "" || modes[id][column] != root.SpellID {
				return fmt.Errorf("spell %s difficulty mismatch", root.SpellID)
			}
		}
	}
	return nil
}

func groupEncounters(d *lootcatalog.Dataset, encounters []Encounter) error {
	owners := map[string]string{}
	known := map[string]lootcatalog.Boss{}
	for _, boss := range d.Bosses {
		known[boss.ID] = boss
	}
	for _, encounter := range encounters {
		if !strings.HasPrefix(encounter.ID, "encounter:"+encounter.InstanceID+":") || known[encounter.ID].ID != "" {
			return fmt.Errorf("invalid encounter %s", encounter.ID)
		}
		for _, participant := range encounter.Participants {
			boss, ok := known[participant]
			if !ok || boss.InstanceID != encounter.InstanceID || owners[participant] != "" {
				return fmt.Errorf("invalid encounter participant %s", participant)
			}
			owners[participant] = encounter.ID
		}
		boss := lootcatalog.Boss{ID: encounter.ID, InstanceID: encounter.InstanceID, Name: encounter.Name}
		known[boss.ID] = boss
		d.Bosses = append(d.Bosses, boss)
	}
	bosses := []lootcatalog.Boss{}
	for _, boss := range d.Bosses {
		if owners[boss.ID] == "" {
			bosses = append(bosses, boss)
		}
	}
	d.Bosses = bosses
	seen := map[string]bool{}
	drops := []lootcatalog.Drop{}
	for _, drop := range d.Drops {
		if owner := owners[drop.BossID]; owner != "" {
			drop.BossID = owner
		}
		key := fmt.Sprintf("%+v", drop)
		if !seen[key] {
			drops = append(drops, drop)
			seen[key] = true
		}
	}
	d.Drops = drops
	return nil
}
