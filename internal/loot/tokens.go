package loot

import "strings"

const (
	TokenBeschuetzer = "Weiheabzeichen des Beschützers"
	TokenBezwinger   = "Weiheabzeichen des Bezwingers"
	TokenEroberer    = "Weiheabzeichen des Eroberers"
)

var TokenItemNames = []string{TokenBeschuetzer, TokenBezwinger, TokenEroberer}

func IsTokenAward(itemName string) bool {
	n := NormalizeItemName(itemName)
	for _, tokenName := range TokenItemNames {
		if n == NormalizeItemName(tokenName) {
			return true
		}
	}
	return false
}

func NormalizeItemName(itemName string) string {
	n := strings.ToLower(strings.TrimSpace(itemName))
	replacer := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
	return replacer.Replace(n)
}

func IsTokenItem(itemName string) bool { return IsTokenAward(itemName) }
