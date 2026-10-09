package legacyimport

import (
	"encoding/json"
	"strconv"

	"raid-website/internal/loot"
)

type DryRunReport struct {
	RowsRead       int      `json:"rows_read"`
	TokenItems     int      `json:"token_items_detected"`
	Warnings       []string `json:"warnings"`
	Errors         []string `json:"errors"`
	CreatedCount   int      `json:"created_count"`
	UpdatedCount   int      `json:"updated_count"`
	UnchangedCount int      `json:"unchanged_count"`
}

func BuildDryRun(rows []LootRow) DryRunReport {
	rep := DryRunReport{RowsRead: len(rows)}
	for i, r := range rows {
		if loot.IsTokenAward(r.Item) {
			rep.TokenItems++
		}
		if r.CurrentRaidPlayer == "" && r.Item != "" {
			rep.Warnings = append(rep.Warnings, "row "+strconv.Itoa(i+2)+": empty Current Raid Player for item")
		}
	}
	return rep
}

func (r DryRunReport) ToJSON() string {
	b, _ := json.Marshal(r)
	return string(b)
}
