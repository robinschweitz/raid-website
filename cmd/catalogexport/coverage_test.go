package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"raid-website/internal/lootcatalog"
)

func TestSixRaidSourceDatasetOffline(t *testing.T) {
	content, err := os.ReadFile("../../scripts/data/catalog-wotlk335.json")
	if err != nil {
		t.Fatal(err)
	}
	var dataset lootcatalog.Dataset
	if err := json.Unmarshal(content, &dataset); err != nil {
		t.Fatal(err)
	}
	_, report := lootcatalog.Canonical(dataset)
	if len(report.Errors) != 0 || report.RawHash != "bc64971cacf8b7569695f414cf825b91c0061f7ff5249653f9954216c3f92bb3" {
		t.Fatalf("source dataset changed: %+v", report)
	}
	if len(dataset.Instances) != 6 || len(dataset.Bosses) != 67 || len(dataset.Items) != 2913 || len(dataset.Drops) != 5154 {
		t.Fatal("dataset coverage changed")
	}
	counts := map[string]int{}
	for _, boss := range dataset.Bosses {
		counts[boss.InstanceID]++
	}
	for id, want := range map[string]int{"631": 15, "649": 10, "603": 19, "533": 15, "724": 4, "624": 4} {
		if counts[id] != want {
			t.Fatalf("instance %s boss metadata: %d", id, counts[id])
		}
	}
	for _, drop := range dataset.Drops {
		if drop.BossID == "36939" || drop.BossID == "36948" || drop.BossID == "30549" || drop.BossID == "16063" || drop.BossID == "16064" || drop.BossID == "16065" {
			t.Fatalf("shared chest attributed to participant: %+v", drop)
		}
	}
	rows := coverage(dataset)
	total := 0
	for _, row := range rows {
		total += row.Drops
		if row.Items < 1 || row.BossesWithDrops < 1 || row.LootModes == nil {
			t.Fatal("invalid coverage summary")
		}
	}
	if total != len(dataset.Drops) {
		t.Fatalf("coverage rows: %d drops: %d", len(rows), total)
	}
	license, err := os.ReadFile("../../scripts/data/catalog-wotlk335.json.LICENSE")
	if err != nil || !strings.Contains(string(license), "Version 2, June 1991") {
		t.Fatal("separate upstream license missing")
	}
	if !strings.Contains(strings.ToLower(strings.Join(dataset.Notices, "\n")), "keine menschliche freigabe") {
		t.Fatal("source check misrepresented as human approval")
	}
}
