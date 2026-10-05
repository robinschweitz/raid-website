package legacyimport

import (
	"encoding/csv"
	"io"
	"strings"
)

// RaidArchiveRow is a row from the "Raid Archive" tab (attendance).
type RaidArchiveRow struct {
	RaidID       string
	TimestampRaw string
	Position     string
	Character    string
	Role         string
	Class        string
	Spec         string
}

// LootArchiveRow is a row from the "Loot Archive" tab (loot awards).
type LootArchiveRow struct {
	ImportID     string
	RaidID       string
	TimestampRaw string
	Player       string // The sheet calls this column Spieler, but its values are character names.
	Loot         string
	Priority     string
	ItemID       string
}

// CharacterRow is a row from the "Charaktere" tab.
type CharacterRow struct {
	Character  string
	Player     string
	Class      string
	MainSpec   string
	SecondSpec string
}

// PointsRow is a row from the "Punkte" tab.
type PointsRow struct {
	Player   string
	Points   string
	Inactive string
}

func readTable(r io.Reader, aliases map[string]string) ([]map[string]string, error) {
	c := csv.NewReader(r)
	c.Comma = ','
	c.FieldsPerRecord = -1
	records, err := c.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	header := records[0]
	idx := make(map[string]int, len(header))
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		canonical, ok := aliases[key]
		if !ok {
			continue
		}
		if _, exists := idx[canonical]; !exists {
			idx[canonical] = i
		}
	}
	out := make([]map[string]string, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		rec := records[i]
		row := make(map[string]string, len(idx))
		for canonical, pos := range idx {
			if pos < len(rec) {
				row[canonical] = strings.TrimSpace(rec[pos])
			}
		}
		out = append(out, row)
	}
	return out, nil
}

var raidArchiveAliases = map[string]string{
	"raid id":   "RaidID",
	"timestamp": "Timestamp",
	"position":  "Position",
	"charakter": "Character",
	"rolle":     "Role",
	"klasse":    "Class",
	"spec":      "Spec",
}

func ParseRaidArchive(r io.Reader) ([]RaidArchiveRow, error) {
	rows, err := readTable(r, raidArchiveAliases)
	if err != nil {
		return nil, err
	}
	out := make([]RaidArchiveRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, RaidArchiveRow{
			RaidID:       m["RaidID"],
			TimestampRaw: m["Timestamp"],
			Position:     m["Position"],
			Character:    m["Character"],
			Role:         m["Role"],
			Class:        m["Class"],
			Spec:         m["Spec"],
		})
	}
	return out, nil
}

var lootArchiveAliases = map[string]string{
	"import id": "ImportID",
	"raid id":   "RaidID",
	"timestamp": "Timestamp",
	"spieler":   "Player",
	"loot":      "Loot",
	"priority":  "Priority",
	"itemid":    "ItemID",
}

func ParseLootArchive(r io.Reader) ([]LootArchiveRow, error) {
	rows, err := readTable(r, lootArchiveAliases)
	if err != nil {
		return nil, err
	}
	out := make([]LootArchiveRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, LootArchiveRow{
			ImportID:     m["ImportID"],
			RaidID:       m["RaidID"],
			TimestampRaw: m["Timestamp"],
			Player:       m["Player"],
			Loot:         m["Loot"],
			Priority:     m["Priority"],
			ItemID:       m["ItemID"],
		})
	}
	return out, nil
}

var characterAliases = map[string]string{
	"charakter":   "Character",
	"spieler":     "Player",
	"klasse":      "Class",
	"main spec":   "MainSpec",
	"second spec": "SecondSpec",
}

func ParseCharaktere(r io.Reader) ([]CharacterRow, error) {
	rows, err := readTable(r, characterAliases)
	if err != nil {
		return nil, err
	}
	out := make([]CharacterRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, CharacterRow{
			Character:  m["Character"],
			Player:     m["Player"],
			Class:      m["Class"],
			MainSpec:   m["MainSpec"],
			SecondSpec: m["SecondSpec"],
		})
	}
	return out, nil
}

var pointsAliases = map[string]string{
	"spieler": "Player",
	"punkte":  "Points",
	"inaktiv": "Inactive",
}

func ParsePunkte(r io.Reader) ([]PointsRow, error) {
	rows, err := readTable(r, pointsAliases)
	if err != nil {
		return nil, err
	}
	out := make([]PointsRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, PointsRow{
			Player:   m["Player"],
			Points:   m["Points"],
			Inactive: m["Inactive"],
		})
	}
	return out, nil
}
