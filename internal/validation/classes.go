package validation

import "strings"

// recruitmentSlotValues are the target-setup values that the recruitment editor
// offers (web/src/raidSetup.ts, setupOptions). "Leer", "Maintank" and "Offtank" are
// included because the editor offers them and the standard ICC setup uses them
// (owner decision 2026-10-25, FR-23.4).
var recruitmentSlotValues = map[string]bool{
	"Leer": true, "Maintank": true, "Offtank": true,
	"Irgendein Tank": true, "Irgendein Heiler": true, "Irgendein MDD": true, "Irgendein RDD": true,
	"Krieger Furor": true, "Schurke Kampf": true, "Todesritter Unheilig": true, "Paladin Vergeltung": true,
	"Jäger Treffsicherheit": true, "Hexer Dämonologie": true, "Druide Eule": true, "Magier Feuer": true,
	"Schamane Elementar": true, "Priester Schatten": true, "Paladin Heilig": true,
	"Schamane Wiederherstellung": true, "Druide Wiederherstellung": true, "Priester Disziplin": true,
}

// IsRecruitmentSlot reports whether value is an accepted target-setup slot value (FR-23.4).
// The match is exact: no trimming, no case folding, so stored values stay the ones the editor sent.
func IsRecruitmentSlot(value string) bool {
	return recruitmentSlotValues[value]
}

var ClassSpecs = map[string][]string{
	"Todesritter": {"Blut (Tank)", "Blut (DD)", "Frost", "Unheilig"},
	"Druide":      {"Gleichgewicht (Eule)", "Wilder Kampf (Cat)", "Wiederherstellung (Baum)", "Wächter (Tank / Bär)"},
	"Jäger":       {"Tierherrschaft", "Treffsicherheit", "Überleben"},
	"Magier":      {"Arkan", "Feuer", "Frost"},
	"Paladin":     {"Heilig", "Schutz (Tank)", "Vergeltung"},
	"Priester":    {"Disziplin", "Heilig", "Schatten"},
	"Schamane":    {"Elementar", "Verstärkung", "Wiederherstellung"},
	"Schurke":     {"Meucheln", "Kampf", "Täuschung"},
	"Hexer":       {"Dämonologie", "Gebrechen", "Zerstörung"},
	"Krieger":     {"Waffen", "Furor", "Schutz (Tank)"},
}

func NormalizeClassName(s string) string {
	v := strings.TrimSpace(s)
	switch strings.ToLower(v) {
	case "todesritter":
		return "Todesritter"
	case "druide":
		return "Druide"
	case "jäger", "jager":
		return "Jäger"
	case "magier":
		return "Magier"
	case "paladin":
		return "Paladin"
	case "priester":
		return "Priester"
	case "schamane":
		return "Schamane"
	case "schurke":
		return "Schurke"
	case "hexer", "warlock":
		return "Hexer"
	case "krieger", "warrior":
		return "Krieger"
	default:
		return v
	}
}
