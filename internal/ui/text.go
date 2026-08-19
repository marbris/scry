package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Measuring and cutting text by the room it takes on screen.
//
// Not by bytes, and not by runes either. A card called "👑-Marchesa d'Amati"
// is nineteen runes and twenty-one columns, because an emoji occupies two
// cells; a row measured in runes overflows its panel, wraps, and leaves that
// panel a line taller than the one beside it. Every width in this package is
// a display width.

func textWidth(s string) int { return runewidth.StringWidth(s) }

// truncate cuts to a display width, marking the cut.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if textWidth(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return runewidth.Truncate(s, max, "…")
}

// pad extends a string to a display width, for lining columns up by hand
// where a lipgloss Width() would also repaint the background.
func pad(s string, width int) string {
	if n := textWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// fit does both: exactly this wide, cut or padded.
func fit(s string, width int) string { return pad(truncate(s, width), width) }

// wrap breaks text to a width, on spaces where it can. Blank lines in the
// source survive as blank lines, because a rule's paragraphs are how it is
// meant to be read.
func wrap(s string, width int) []string {
	if width < 1 {
		return nil
	}

	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case textWidth(line)+1+textWidth(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
