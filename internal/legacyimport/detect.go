package legacyimport

import (
	"strings"

	"raid-website/internal/tokens"
)

// DetectTokenItems returns true if any of the three token item names appear (normalized)
func DetectTokenItems(items ...string) bool {
	for _, it := range items {
		if it == "" {
			continue
		}
		if tokens.IsTokenItem(it) {
			return true
		}
	}
	return false
}

// NormalizeForMatching lowercases and trims
func NormalizeForMatching(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
