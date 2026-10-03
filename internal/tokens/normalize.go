package tokens

import (
	"regexp"
	"strings"
)

var diacritics = regexp.MustCompile(`[ÄäÖöÜüßÉéÀàÈèÇç]`)

func NormalizeItemName(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	// basic folding
	replacer := strings.NewReplacer(
		"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
	)
	v = replacer.Replace(v)
	return v
}

func IsTokenItem(s string) bool {
	n := NormalizeItemName(s)
	for _, t := range TokenItemNames {
		if NormalizeItemName(t) == n {
			return true
		}
	}
	return false
}
