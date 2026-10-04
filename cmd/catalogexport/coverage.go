package main

import (
	"fmt"
	"sort"
	"strings"

	"raid-website/internal/lootcatalog"
)

type coverageRow struct {
	InstanceID      string `json:"instance_id"`
	InstanceName    string `json:"instance_name"`
	BossMetadata    int    `json:"boss_metadata"`
	RaidSize        int    `json:"raid_size"`
	Difficulty      string `json:"difficulty"`
	BossesWithDrops int    `json:"bosses_with_drops"`
	Items           int    `json:"items"`
	Drops           int    `json:"drops"`
	LootModes       []int  `json:"loot_modes"`
}

// coverage summarizes actual records, not kills, guild statistics, or completeness.
func coverage(dataset lootcatalog.Dataset) []coverageRow {
	bossInstances := map[string]string{}
	bossCounts := map[string]int{}
	instanceNames := map[string]string{}
	for _, instance := range dataset.Instances {
		instanceNames[instance.ID] = instance.Name
	}
	for _, boss := range dataset.Bosses {
		bossInstances[boss.ID] = boss.InstanceID
		bossCounts[boss.InstanceID]++
	}
	type counts struct {
		row           coverageRow
		bosses, items map[string]bool
		modes         map[int]bool
	}
	groups := map[string]*counts{}
	for _, drop := range dataset.Drops {
		instance := bossInstances[drop.BossID]
		difficulty := strings.SplitN(drop.Difficulty, "/", 2)[0]
		if strings.Contains(drop.Difficulty, "hardmode:true") {
			difficulty += "/hardmode:true"
		} else if strings.Contains(drop.Difficulty, "hardmode:false") {
			difficulty += "/hardmode:false"
		}
		key := fmt.Sprintf("%s/%d/%s", instance, drop.RaidSize, difficulty)
		group, ok := groups[key]
		if !ok {
			group = &counts{row: coverageRow{InstanceID: instance, InstanceName: instanceNames[instance], BossMetadata: bossCounts[instance], RaidSize: drop.RaidSize, Difficulty: difficulty, LootModes: []int{}}, bosses: map[string]bool{}, items: map[string]bool{}, modes: map[int]bool{}}
			groups[key] = group
		}
		group.row.Drops++
		group.bosses[drop.BossID] = true
		group.items[drop.ItemID] = true
		group.modes[drop.LootMode] = true
	}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []coverageRow{}
	for _, key := range keys {
		group := groups[key]
		group.row.BossesWithDrops = len(group.bosses)
		group.row.Items = len(group.items)
		for mode := range group.modes {
			group.row.LootModes = append(group.row.LootModes, mode)
		}
		sort.Ints(group.row.LootModes)
		result = append(result, group.row)
	}
	return result
}
