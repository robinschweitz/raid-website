package attendance

import (
	"strings"

	"raid-website/internal/validation"
)

// NormalizeClass accepts canonical class names and English names in existing records.
func NormalizeClass(class string) string {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "death knight":
		return "Todesritter"
	case "druid":
		return "Druide"
	case "hunter":
		return "J\u00e4ger"
	case "mage":
		return "Magier"
	case "priest":
		return "Priester"
	case "shaman":
		return "Schamane"
	case "rogue":
		return "Schurke"
	default:
		return validation.NormalizeClassName(class)
	}
}
