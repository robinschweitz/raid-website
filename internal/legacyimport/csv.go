package legacyimport

import (
	"encoding/csv"
	"io"
	"strings"
)

type LootRow struct {
	Klasse            string
	Spec              string
	Boss              string
	Item              string
	ItemID            string
	RaidBuff          string
	Anzahl            string
	Tanks             string
	Heiler            string
	CurrentRaidPlayer string
	TokensGesammelt   string
}

var headerAliases = map[string]string{
	"klasse":              "Klasse",
	"spec":                "Spec",
	"boss":                "Boss",
	"item":                "Item",
	"itemid":              "ItemID",
	"raid buff":           "RaidBuff",
	"anzahl":              "Anzahl",
	"tanks":               "Tanks",
	"heiler":              "Heiler",
	"current raid player": "CurrentRaidPlayer",
	"tokens gesammelt":    "TokensGesammelt",
}

func ParseCSV(r io.Reader) ([]LootRow, error) {
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
		canonical, ok := headerAliases[key]
		if !ok {
			continue
		}
		if _, exists := idx[canonical]; !exists {
			idx[canonical] = i
		}
	}
	rows := make([]LootRow, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		rec := records[i]
		for j := range rec {
			rec[j] = strings.TrimSpace(rec[j])
		}
		get := func(name string) string {
			pos, ok := idx[name]
			if !ok || pos >= len(rec) {
				return ""
			}
			return rec[pos]
		}
		rw := LootRow{
			Klasse:            get("Klasse"),
			Spec:              get("Spec"),
			Boss:              get("Boss"),
			Item:              get("Item"),
			ItemID:            get("ItemID"),
			RaidBuff:          get("RaidBuff"),
			Anzahl:            get("Anzahl"),
			Tanks:             get("Tanks"),
			Heiler:            get("Heiler"),
			CurrentRaidPlayer: get("CurrentRaidPlayer"),
			TokensGesammelt:   get("TokensGesammelt"),
		}
		rows = append(rows, rw)
	}
	return rows, nil
}
