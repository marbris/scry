package ui

import (
	"strings"
	"unicode/utf8"
)

// Measuring and cutting text by what it looks like rather than what it is.

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// truncate cuts to a rune count, not a byte count — an em dash in a type
// line is three bytes, and slicing through one renders as a replacement
// character.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runeLen(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string([]rune(s)[:max-1]) + "…"
}

// pad extends a string to a rune width, for lining columns up by hand where
// a lipgloss Width() would also repaint the background.
func pad(s string, width int) string {
	if n := runeLen(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// fit does both: exactly this wide, cut or padded.
func fit(s string, width int) string { return pad(truncate(s, width), width) }
