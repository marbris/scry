package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/theme"
)

// One card, on one line, in a column that may be very narrow.
//
// A row is two columns: the card's name, and whatever property the list is
// sorted by. With several panels open there is never enough width, so rather
// than truncating — which loses the end of every name equally — the row gives
// things up in a fixed order:
//
//  1. everything fits
//  2. the type line becomes an initialism: Legendary Creature — Elf Faerie
//     Noble → LC-EFN
//  3. the name becomes one too, with a full stop to say so: Dwynen,
//     Gilt-Leaf Daen → D,GLD.
//  4. only then is anything cut off
//
// An initialism is still recognisable to someone who knows the card, which a
// name cut off after nine characters often isn't.
//
// The ladder is decided per row, not per list, so a column ends up mixing
// full names with shortened ones. That is what the full stop is for: it says
// which is which. Shortening every row to match the longest would cost the
// names that fit perfectly well, and gain only tidiness.

const (
	// gutter is the marker column: one character and a space.
	gutter = 2
	// gap is the space between the name and the column beside it.
	gap = 1
)

// rowState is what the list knows about a card that the card doesn't.
type rowState struct {
	selected bool // picked out with v
	member   bool // also in the editing deck, or — in the editing deck — also in a list on screen
	cursor   bool // under the cursor
}

// renderRow draws one card to exactly width columns.
func renderRow(c deck.Card, order cardSort, st rowState, width int) string {
	if width < 1 {
		return ""
	}

	mark, markStyle := marker(c, st)
	body := maxInt(width-gutter, 1)

	name := cardName(c)
	col := order.column(c.Card)
	text, colText := layoutRow(name, col, order.abbreviates(), body)

	nameStyle := lipgloss.NewStyle().Foreground(theme.Text)
	if st.cursor {
		nameStyle = nameStyle.Foreground(theme.SelectionFg).Bold(true)
	}
	colStyle := lipgloss.NewStyle().Foreground(theme.TextDim)

	line := markStyle.Render(pad(mark, gutter)) +
		nameStyle.Render(text) +
		colStyle.Render(colText)

	if st.cursor {
		// The cursor is a background so it reads at a glance across four
		// panels, where a colour change alone gets lost.
		return lipgloss.NewStyle().Background(theme.SelectionBg).Width(width).Render(
			markStyle.Render(pad(mark, gutter)) + nameStyle.Render(text) + colStyle.Render(colText))
	}
	return line
}

// layoutRow works the ladder, returning the name and the column already
// padded to fill the width between them.
func layoutRow(name, col string, colAbbreviates bool, width int) (string, string) {
	fits := func(n, c string) bool {
		need := runeLen(n) + runeLen(c)
		if c != "" {
			need += gap
		}
		return need <= width
	}

	// 1. As they are.
	if fits(name, col) {
		return padBetween(name, col, width)
	}

	// 2. Shorten the column, if it's the kind that can be.
	if colAbbreviates {
		if short := initialism(col, false); fits(name, short) {
			return padBetween(name, short, width)
		}
		col = initialism(col, false)
	}

	// 3. Shorten the name.
	short := initialism(name, true)
	if fits(short, col) {
		return padBetween(short, col, width)
	}

	// 4. Give up and cut. The column goes first: you can work out a mana
	// cost from the card, but not a name you can't read.
	if runeLen(short) <= width {
		return padBetween(short, "", width)
	}
	return truncate(short, width), ""
}

// padBetween puts the two columns at either end of the width.
func padBetween(left, right string, width int) (string, string) {
	space := width - runeLen(left) - runeLen(right)
	if space < 0 {
		space = 0
	}
	return left + strings.Repeat(" ", space), right
}

// initialism reduces a phrase to its first letters, keeping enough
// punctuation to stay recognisable.
//
//	Legendary Creature — Elf Faerie Noble  →  LC-EFN
//	Dwynen, Gilt-Leaf Daen                 →  D,GLD.
//	Miara, Thorn of the Glade              →  M,TotG.
//
// Case is preserved, which is what keeps the little words little: "of the"
// contributes "ot", not "OT". A dash between clauses survives as a dash,
// because a type line without it reads as one long word. The full stop marks
// a shortened *name* — a type line is obviously not a name either way.
func initialism(s string, trailingDot bool) string {
	var b strings.Builder

	for _, word := range strings.Fields(s) {
		// A dash standing on its own is a separator, not a word.
		if isDash(word) {
			b.WriteString("-")
			continue
		}
		// Hyphenated words are several words wearing one coat: Gilt-Leaf
		// gives up G and L.
		for _, part := range splitHyphens(word) {
			trailing := ""
			part = strings.TrimRightFunc(part, func(r rune) bool {
				if r == ',' || r == ':' {
					trailing = string(r) + trailing
					return true
				}
				return false
			})
			for _, r := range part {
				b.WriteRune(r)
				break
			}
			b.WriteString(trailing)
		}
	}

	out := b.String()
	if out == "" {
		return out
	}
	if trailingDot {
		out += "."
	}
	return out
}

func isDash(w string) bool {
	switch w {
	case "-", "–", "—", "//":
		return true
	}
	return false
}

func splitHyphens(word string) []string {
	parts := strings.FieldsFunc(word, func(r rune) bool { return r == '-' || r == '–' })
	if len(parts) == 0 {
		return []string{word}
	}
	return parts
}

// cardName is the name as the row shows it, with the quantity in front when
// a deck runs more than one.
func cardName(c deck.Card) string {
	if c.Qty > 1 {
		return itoa(c.Qty) + "x " + c.Card.Name
	}
	return c.Card.Name
}

// marker is the one character in front of a row.
//
// One character, so there is a precedence: what you just picked out matters
// more than what the card is, which matters more than where else it lives.
func marker(c deck.Card, st rowState) (string, lipgloss.Style) {
	switch {
	case st.selected:
		return "▸", lipgloss.NewStyle().Foreground(theme.Marked).Bold(true)
	case c.Commander:
		return "★", lipgloss.NewStyle().Foreground(theme.Accent)
	case st.member:
		return "•", lipgloss.NewStyle().Foreground(theme.Member)
	}
	return " ", lipgloss.NewStyle()
}
