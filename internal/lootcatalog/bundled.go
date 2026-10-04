package lootcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	catalogdata "raid-website/scripts/data"
)

const (
	lootModePathToken = "source-lootmode-path:"
	hardmodeToken     = "hardmode:"
	// unconditionalLootPath is the only source path that proves plain loot. Every
	// other path is conditional and must stay labelled as such.
	unconditionalLootPath = "1"
)

var bundle struct {
	sync.Once
	catalog Catalog
	err     error
}

func validMode(mode Mode) bool {
	return mode == ModeNormal || mode == ModeHeroic || mode == ModeHardmode
}

// sourceGameMode normalizes one raw source relationship into a gameplay mode.
//
// The difficulty string is the only source of truth: normal and heroic come from
// the source raid difficulty, hardmode only from an explicit hardmode token. The
// loot-mode mask never implies heroic, because conditional creature loot exists
// in every difficulty. Conditional means "the source does not prove plain loot",
// never "heroic".
func sourceGameMode(drop Drop) (Mode, bool, error) {
	segments := strings.Split(drop.Difficulty, "/")
	var mode Mode
	switch strings.TrimSpace(segments[0]) {
	case "raid-difficulty:0", "raid-difficulty:1":
		mode = ModeNormal
	case "raid-difficulty:2", "raid-difficulty:3":
		mode = ModeHeroic
	default:
		return "", false, fmt.Errorf("%w: nicht unterstützte Quelldifficulty %q", ErrInvalid, drop.Difficulty)
	}
	conditional := drop.LootMode != 1
	hardmode := false
	for _, segment := range segments[1:] {
		key, _, structured := strings.Cut(strings.TrimSpace(segment), ":")
		switch {
		case strings.TrimSpace(segment) == hardmodeToken+"true":
			hardmode = true
		case strings.TrimSpace(segment) == hardmodeToken+"false":
		case strings.HasPrefix(strings.TrimSpace(segment), lootModePathToken):
			if strings.TrimPrefix(strings.TrimSpace(segment), lootModePathToken) != unconditionalLootPath {
				conditional = true
			}
		case structured && key != "source-creature" && key != "source-chest":
			// An unrecognized structured token could gate loot, so it stays conditional.
			conditional = true
		}
	}
	if hardmode {
		return ModeHardmode, true, nil
	}
	return mode, conditional, nil
}

// decodeBundled validates the compiled source file and hashes its exact bytes.
func decodeBundled(raw string) (Catalog, error) {
	var input Dataset
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Catalog{}, fmt.Errorf("gebündelter Katalog kann nicht gelesen werden: %w", err)
	}
	canonical, report := Canonical(input)
	if len(report.Errors) != 0 {
		return Catalog{}, fmt.Errorf("%w: %v", ErrInvalid, report.Errors)
	}
	for _, drop := range canonical.Drops {
		if _, _, err := sourceGameMode(drop); err != nil {
			return Catalog{}, err
		}
	}
	hash := sha256.Sum256([]byte(raw))
	return Catalog{
		Sources:   []Provenance{{Source: canonical.Source, RawHash: hex.EncodeToString(hash[:]), Notices: canonical.Notices}},
		Instances: canonical.Instances,
		Bosses:    canonical.Bosses,
		Items:     canonical.Items,
		Drops:     canonical.Drops,
	}, nil
}

// bundledCatalog parses the embedded file once and returns a detached copy, so
// callers can never mutate the shared source.
func bundledCatalog() (Catalog, error) {
	bundle.Do(func() {
		bundle.catalog, bundle.err = decodeBundled(catalogdata.JSON())
	})
	if bundle.err != nil {
		return Catalog{}, bundle.err
	}
	detached := bundle.catalog
	detached.Sources = make([]Provenance, len(bundle.catalog.Sources))
	for i, provenance := range bundle.catalog.Sources {
		provenance.Notices = append([]string{}, provenance.Notices...)
		detached.Sources[i] = provenance
	}
	detached.Instances = append([]Instance{}, bundle.catalog.Instances...)
	detached.Bosses = append([]Boss{}, bundle.catalog.Bosses...)
	detached.Items = make([]Item, len(bundle.catalog.Items))
	for i, item := range bundle.catalog.Items {
		detached.Items[i] = cloneItem(item)
	}
	detached.Drops = append([]Drop{}, bundle.catalog.Drops...)
	return detached, nil
}

func cloneItem(item Item) Item {
	if item.Binding != nil {
		binding := *item.Binding
		item.Binding = &binding
	}
	if item.Quality != nil {
		quality := *item.Quality
		item.Quality = &quality
	}
	return item
}
