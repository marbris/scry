package ui

import "strings"

// splitLines and visibleWidth measure a rendered frame the way a terminal
// would: by what it shows, not by the escape sequences that colour it.

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func visibleWidth(s string) int {
	return runeLen(stripANSI(s))
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
