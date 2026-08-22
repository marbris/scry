package deck

import (
	"strings"
	"unicode"
)

// Slugify turns a deck's title into a name worth typing: "Winota: Snowball
// Stax" becomes "winota-snowball-stax".
//
// A slash groups decks into folders — "Aggro / Mono Red" becomes
// "aggro/mono-red" — so each segment between slashes is slugified on its own
// and the slashes kept as separators. Empty segments are dropped, which is
// also what keeps a stray ".." or a leading slash from escaping the decks
// directory: neither survives as a segment.
func Slugify(name string) string {
	var segs []string
	for _, seg := range strings.Split(name, "/") {
		if s := slugifySegment(seg); s != "" {
			segs = append(segs, s)
		}
	}
	return strings.Join(segs, "/")
}

func slugifySegment(name string) string {
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
