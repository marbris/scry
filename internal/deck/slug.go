package deck

import (
	"strings"
	"unicode"
)

// Slugify turns a deck's title into a name worth typing: "Winota: Snowball
// Stax" becomes "winota-snowball-stax".
func Slugify(name string) string {
	var b strings.Builder
	lastDash := true // no leading dash
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
