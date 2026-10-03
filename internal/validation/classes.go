package validation

import "strings"

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
