package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/mtg"
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

	// The sort decides what the row is about, so it decides what is worth
	// colouring. Sorting by colour and reading a column of grey names tells
	// you nothing the order didn't already.
	nameStyle := lipgloss.NewStyle().Foreground(nameColour(c.Card, order))
	if st.cursor {
		nameStyle = lipgloss.NewStyle().Foreground(theme.SelectionFg).Bold(true)
	}

	painted := paintColumn(colText, c.Card, order, st.cursor)

	line := markStyle.Render(pad(mark, gutter)) + nameStyle.Render(text) + painted
	if st.cursor {
		// The cursor is a background so it reads at a glance across four
		// panels, where a colour change alone gets lost.
		return lipgloss.NewStyle().Background(theme.SelectionBg).Width(width).Render(line)
	}
	return line
}

// nameColour is what the card's name is written in.
//
// Plain text unless the list is ordered by something the name can carry: by
// colour, the name takes the card's colour; by type, its type's. The order
// you chose is the question you are asking, and the answer is worth being
// able to see down the column.
func nameColour(c mtg.Card, order cardSort) lipgloss.Color {
	switch order {
	case sortColor:
		return colourForCard(c.DisplayColors())
	case sortType:
		return typeColour(c.TypeLine)
	}
	return theme.Text
}

// paintColumn renders the second column. A mana cost gets a colour per
// symbol, which is how you read a curve at a glance; a type line takes its
// type's colour; anything else is dim, being a number rather than a fact
// about the card.
func paintColumn(text string, c mtg.Card, order cardSort, under bool) string {
	if text == "" {
		return ""
	}
	if under {
		return lipgloss.NewStyle().Foreground(theme.SelectionFg).Render(text)
	}

	switch {
	case order.showsMana():
		return paintMana(text)
	case order == sortType:
		return lipgloss.NewStyle().Foreground(typeColour(c.TypeLine)).Render(text)
	}
	return lipgloss.NewStyle().Foreground(theme.TextDim).Render(text)
}

// paintMana colours a rendered cost symbol by symbol. It works from the text
// rather than the symbol list because the ladder may have padded it, and the
// padding has to keep its place.
func paintMana(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch r {
		case ' ', '/':
			b.WriteString(lipgloss.NewStyle().Foreground(theme.TextMuted).Render(string(r)))
		default:
			b.WriteString(lipgloss.NewStyle().
				Foreground(symbolColour(string(r))).Render(string(r)))
		}
	}
	return b.String()
}

// typeColour gives each card type its own colour, so a list ordered by type
// reads as bands rather than as a column of identical grey.
//
// A mapping onto the existing roles rather than eight new ones: a theme
// that changes its greens changes creatures with them, which is the
// behaviour you would want anyway.
func typeColour(typeLine string) lipgloss.Color {
	switch mtg.PrimaryType(typeLine) {
	case "Creature":
		return theme.Success
	case "Instant":
		return theme.ManaU
	case "Sorcery":
		return theme.ManaR
	case "Artifact":
		return theme.TextDim
	case "Enchantment":
		return theme.ManaW
	case "Planeswalker":
		return theme.ManaMulti
	case "Battle":
		return theme.Accent
	case "Land":
		return theme.Member
	}
	return theme.TextMuted
}

// layoutRow works the ladder, returning the name and the column already
// padded to fill the width between them.
func layoutRow(name, col string, colAbbreviates bool, width int) (string, string) {
	fits := func(n, c string) bool {
		need := textWidth(n) + textWidth(c)
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
	if textWidth(short) <= width {
		return padBetween(short, "", width)
	}
	return truncate(short, width), ""
}

// padBetween puts the two columns at either end of the width.
func padBetween(left, right string, width int) (string, string) {
	space := width - textWidth(left) - textWidth(right)
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
