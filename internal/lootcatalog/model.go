// Package lootcatalog stores reviewed, local game data, never guild raid history.
package lootcatalog

import (
	"encoding/json"
	"errors"
)

// ErrInvalid marks a structurally invalid catalog source, filter, or selection.
var ErrInvalid = errors.New("invalid loot catalog")

// Source identifies the independently licensed, version-pinned dataset.
type Source struct {
	ID          string `json:"id"`
	Revision    string `json:"revision"`
	URL         string `json:"url"`
	License     string `json:"license"`
	Attribution string `json:"attribution"`
}

// Instance uses the source's game map key, not a guild raid ID.
type Instance struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Boss uses the source's creature key.
type Boss struct {
	ID         string `json:"id"`
	InstanceID string `json:"instance_id"`
	Name       string `json:"name"`
}

// Item retains the game item key and a consistent display-name field.
type Item struct {
	ItemID  string `json:"item_id"`
	Name    string `json:"name"`
	Binding *int   `json:"binding,omitempty"`
	Quality *int   `json:"quality,omitempty"`
}

// Drop preserves opaque source mode metadata rather than assuming normal/heroic.
type Drop struct {
	BossID     string `json:"boss_id"`
	ItemID     string `json:"item_id"`
	RaidSize   int    `json:"raid_size"`
	Difficulty string `json:"difficulty"`
	LootMode   int    `json:"loot_mode"`
}

// Dataset is the normalized source file format.
type Dataset struct {
	Source    Source     `json:"source"`
	Notices   []string   `json:"notices"`
	Instances []Instance `json:"instances"`
	Bosses    []Boss     `json:"bosses"`
	Items     []Item     `json:"items"`
	Drops     []Drop     `json:"drops"`
}

// Catalog includes provenance for every merged dataset and never null lists.
type Catalog struct {
	Sources   []Provenance `json:"sources"`
	Instances []Instance   `json:"instances"`
	Bosses    []Boss       `json:"bosses"`
	Items     []Item       `json:"items"`
	Drops     []Drop       `json:"drops"`
}

// Provenance preserves the reviewed dataset's notices and canonical hash.
type Provenance struct {
	Source  Source   `json:"source"`
	RawHash string   `json:"raw_hash"`
	Notices []string `json:"notices"`
}

// Report is the validation result for one normalized dataset.
type Report struct {
	RawHash string   `json:"raw_hash"`
	Source  Source   `json:"source"`
	Notices []string `json:"notices"`
	Errors  []string `json:"errors"`
}

// Mode is the gameplay selection offered to raid staff. It is derived from the
// source relationship, never chosen by a caller.
type Mode string

const (
	// ModeNormal covers both 10-player and 25-player normal difficulty.
	ModeNormal Mode = "NORMAL"
	// ModeHeroic covers both 10-player and 25-player heroic difficulty.
	ModeHeroic Mode = "HEROIC"
	// ModeHardmode covers source loot that requires the encounter hard mode.
	ModeHardmode Mode = "HARDMODE"
)

// SourceVariant is one raw source relationship behind a merged gameplay mode.
type SourceVariant struct {
	Difficulty string `json:"difficulty"`
	LootMode   int    `json:"loot_mode"`
}

// Filter selects drops by gameplay mode and raid size.
type Filter struct {
	BossID   string
	RaidSize int
	Mode     Mode
}

// ItemDrop is one selectable item for a boss, raid size, and gameplay mode. All
// source relationships that lead to it stay visible in SourceVariants.
type ItemDrop struct {
	Item
	BossID         string          `json:"boss_id"`
	RaidSize       int             `json:"raid_size"`
	Mode           Mode            `json:"mode"`
	Conditional    bool            `json:"conditional"`
	SourceVariants []SourceVariant `json:"source_variants"`
}

// InitializationReport records exactly what automatic mirroring changed.
type InitializationReport struct {
	Source     Source        `json:"source"`
	SourceHash string        `json:"source_hash"`
	Inserted   int           `json:"inserted"`
	Updated    int           `json:"updated"`
	Unchanged  int           `json:"unchanged"`
	Retained   int           `json:"retained"`
	Diffs      []CatalogDiff `json:"diffs"`
	Warnings   []string      `json:"warnings"`
}

// CatalogDiff is one explicit before/after fact change. Retained rows record
// historical database targets that the bundled source no longer describes.
type CatalogDiff struct {
	Kind   string          `json:"kind"`
	Key    string          `json:"key"`
	Action string          `json:"action"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}
