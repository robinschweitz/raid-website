// Package lootcatalog stores reviewed, local game data, never guild raid history.
package lootcatalog

import "errors"

var (
	ErrInvalid         = errors.New("invalid loot catalog")
	ErrConflict        = errors.New("loot catalog conflict")
	ErrPreviewRequired = errors.New("loot catalog preview required")
	ErrHashMismatch    = errors.New("loot catalog hash mismatch")
)

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

// Dataset is the normalized upload format. RawHash is calculated over canonical JSON.
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

// Report describes validation, conflicts, and insert/skip counts without overwrites.
type Report struct {
	RawHash   string   `json:"raw_hash"`
	Source    Source   `json:"source"`
	Notices   []string `json:"notices"`
	Errors    []string `json:"errors"`
	Conflicts []string `json:"conflicts"`
	Inserted  int      `json:"inserted"`
	Skipped   int      `json:"skipped"`
	Merged    bool     `json:"merged"`
}

// Filter selects drops using exact, dataset-provided variant values.
type Filter struct {
	BossID     string
	RaidSize   int
	Difficulty string
}

// ItemDrop carries the variant alongside the item for unambiguous dropdown labels.
type ItemDrop struct {
	Item
	BossID     string `json:"boss_id"`
	RaidSize   int    `json:"raid_size"`
	Difficulty string `json:"difficulty"`
	LootMode   int    `json:"loot_mode"`
}
