package tokens

import (
	"strings"
)

const (
	TokenBeschuetzer = "Weiheabzeichen des Beschützers"
	TokenBezwinger   = "Weiheabzeichen des Bezwingers"
	TokenEroberer    = "Weiheabzeichen des Eroberers"
)

var tokenNamesLower = []string{
	strings.ToLower(TokenBeschuetzer),
	strings.ToLower(TokenBezwinger),
	strings.ToLower(TokenEroberer),
}

// IsTokenAward returns true if item is one of the 3 token items
func IsTokenAward(itemName string) bool {
	n := strings.ToLower(strings.TrimSpace(itemName))
	for _, t := range tokenNamesLower {
		if n == t {
			return true
		}
	}
	return false
}

// DeriveTokenAmount returns +1 per token award (or +Anzahl if provided, but sheet uses count; default +1)
func DeriveTokenAmount(count int) int {
	if count <= 0 {
		return 1
	}
	return count
}
