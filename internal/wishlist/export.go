package wishlist

import (
	"strconv"
	"strings"
	"time"
)

// addonFormat versions the wishlist text the ForgeLootWishlist WoW addon
// imports by copy and paste (ADR 070).
const addonFormat = "WL2"

// BuildAddonExport renders a character's wishlist for the in-game addon:
//
//	WL2;<character>;<exportUnix>;<EP balance>
//	<itemID>;<ms|os>;<enhanced 0/1>;<rank>;<note>
//
// Wishes keep their list order (highest priority first) and are ranked from 1.
// The addon rolls 1+X..100+X with X = min(balance, 50) on enhanced wishes.
func BuildAddonExport(characterName string, exportedAt time.Time, balance int, wishes []Wish) string {
	lines := []string{strings.Join([]string{addonFormat, field(characterName), strconv.FormatInt(exportedAt.Unix(), 10), strconv.Itoa(balance)}, ";")}
	for i, wish := range wishes {
		spec, enhanced := "ms", "0"
		if wish.Spec == SpecSecond {
			spec = "os"
		}
		if wish.IsEnhanced && wish.Spec != SpecSecond {
			enhanced = "1"
		}
		lines = append(lines, strings.Join([]string{field(wish.ItemID), spec, enhanced, strconv.Itoa(i + 1), field(wish.Notes)}, ";"))
	}
	return strings.Join(lines, "\n")
}

// field keeps free text from breaking the line and column separators; "|"
// starts escape sequences in a WoW EditBox.
func field(value string) string {
	return strings.TrimSpace(strings.NewReplacer(";", ",", "|", "/", "\r", " ", "\n", " ").Replace(value))
}
