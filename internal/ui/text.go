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

func textWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// runeWidth is how many cells a rune takes.
//
// runewidth alone isn't quite enough. A variation selector U+FE0F asks for
// the *emoji* presentation of the character before it, which terminals draw
// double-width — but runewidth measures the base character on its own and
// calls something like ♟ (U+265F) one cell. Counting the selector as one
// cell rather than zero gets the pair to the two the terminal will use. A
// deck of mine called "…Queens Gambit♟️" is what found this, twice.
//
// Emoji width in terminals is genuinely not fully determinable — different
// terminals disagree — so this is the closest honest approximation rather
// than a guarantee.
func runeWidth(r rune) int {
	switch r {
	case 0xFE0F: // emoji presentation
		return 1
	case 0xFE0E: // text presentation: leave the base narrow
		return 0
	}
	return runewidth.RuneWidth(r)
}

// truncate cuts to a display width, marking the cut. Walks runes with the
// same measure as textWidth, rather than runewidth's own, so the two can't
// disagree about where the cut falls.
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

	room := max - 1 // the ellipsis takes one
	w, cut := 0, 0
	for i, r := range s {
		rw := runeWidth(r)
		if w+rw > room {
			cut = i
			break
		}
		w += rw
		cut = i + len(string(r))
	}
	return s[:cut] + "…"
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

// stripStyles removes colour, for measuring text that has already been
// painted. Needed where a line is assembled from styled parts and then has
// to be padded to a width — measuring the escape sequences as characters is
// what wraps a row and grows a panel.
func stripStyles(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == 0x1b:
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
