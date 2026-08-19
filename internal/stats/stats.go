// Package stats breaks a set of cards down by the properties worth counting:
// what the deck's author called them, what they are, and how they're printed.
//
// Every row carries the test it counted itself with, so filtering the cards
// to a category and the number printed beside that category can never
// disagree.
package stats

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/mtg"
	"scry/internal/theme"
)

// The statistics panel is a list you can walk with J/K. Every row carries
// the test it counted itself with, so filtering the cards to a category and
// the number printed beside that category can never disagree.

type Row struct {
	Group string // "Color", "Rarity", …
	Label string // "Blue", "rare", "3", "Creature", "Ramp"
	Count int    // over the cards on screen — what the bar shows
	Base  int    // over the whole result set — whether the row exists at all
	Color lipgloss.Color
	Match func(deck.Card) bool
}

type Group struct {
	Title string
	Rows  []Row
}

// same reports whether two rows name the same category. The rows carry
// closures, so they can't be compared directly.
func (r Row) Same(other *Row) bool {
	return other != nil && r.Group == other.Group && r.Label == other.Label
}

// ── Building the rows ───────────────────────────────────────────

// colorRows is the colour spread of the spells. Lands are left out for the
// same reason they're left out of the curve: nearly all of them are
// colourless by the card's own reckoning, so counting them says how many
// lands the deck runs — under a "Colorless" heading, where it reads as if
// the deck were full of colourless spells.
func colorRows() []Row {
	spec := []struct {
		label string
		code  string
		color lipgloss.Color
	}{
		{"White", "W", theme.ManaW},
		{"Blue", "U", theme.ManaU},
		{"Black", "B", theme.ManaB},
		{"Red", "R", theme.ManaR},
		{"Green", "G", theme.ManaG},
	}

	rows := make([]Row, 0, len(spec)+2)
	for _, s := range spec {
		code := s.code
		rows = append(rows, Row{
			Group: "Color", Label: s.label, Color: s.color,
			Match: func(ci deck.Card) bool {
				if mtg.IsLand(ci.Card) {
					return false
				}
				for _, c := range ci.Card.DisplayColors() {
					if c == code {
						return true
					}
				}
				return false
			},
		})
	}
	rows = append(rows,
		Row{
			Group: "Color", Label: "Colorless", Color: theme.ManaC,
			Match: func(ci deck.Card) bool {
				return !mtg.IsLand(ci.Card) && len(ci.Card.DisplayColors()) == 0
			},
		},
		Row{
			Group: "Color", Label: "Multi", Color: theme.ManaMulti,
			Match: func(ci deck.Card) bool {
				return !mtg.IsLand(ci.Card) && len(ci.Card.DisplayColors()) > 1
			},
		},
	)
	return rows
}

func rarityRows(entries []deck.Card) []Row {
	color := map[string]lipgloss.Color{
		"common": theme.RarityCommon, "uncommon": theme.RarityUncommon,
		"rare": theme.RarityRare, "mythic": theme.RarityMythic,
		"special": theme.RaritySpecial,
	}

	// The usual rarities in their usual order, then anything unexpected.
	order := []string{"common", "uncommon", "rare", "mythic", "special", "bonus"}
	known := map[string]bool{}
	for _, r := range order {
		known[r] = true
	}
	var extra []string
	for _, e := range entries {
		r := e.Card.Rarity
		if r == "" {
			r = "unknown"
		}
		if !known[r] {
			known[r] = true
			extra = append(extra, r)
		}
	}
	sort.Strings(extra)

	rows := make([]Row, 0, len(order)+len(extra))
	for _, r := range append(order, extra...) {
		rarity := r
		col, ok := color[rarity]
		if !ok {
			col = theme.TextMuted
		}
		rows = append(rows, Row{
			Group: "Rarity", Label: rarity, Color: col,
			Match: func(ci deck.Card) bool {
				got := ci.Card.Rarity
				if got == "" {
					got = "unknown"
				}
				return got == rarity
			},
		})
	}
	return rows
}

// cmcRows is the mana curve. Lands are left out of it: they nearly all cost
// nothing, so counting them buries the curve under a column at zero that
// says only how many lands the deck runs — which the type breakdown below
// already says, and better.
func cmcRows() []Row {
	rows := make([]Row, 0, 8)
	for i := 0; i <= 7; i++ {
		n := i
		label := strconv.Itoa(n)
		if n == 7 {
			label = "7+"
		}
		rows = append(rows, Row{
			Group: "Mana Value", Label: label, Color: theme.BarFill,
			Match: func(ci deck.Card) bool {
				if mtg.IsLand(ci.Card) {
					return false
				}
				cmc := int(ci.Card.CMC)
				if n == 7 {
					return cmc >= 7
				}
				return cmc == n
			},
		})
	}
	return rows
}

// isLand reports whether a card's front face is a land, which is what
// decides where it's counted.

func typeRows() []Row {
	types := []string{"Creature", "Instant", "Sorcery", "Artifact", "Enchantment", "Planeswalker", "Land"}
	rows := make([]Row, 0, len(types))
	for _, t := range types {
		cardType := t
		rows = append(rows, Row{
			Group: "Type", Label: cardType, Color: theme.Special,
			Match: func(ci deck.Card) bool {
				return strings.Contains(ci.Card.TypeLine, cardType)
			},
		})
	}
	return rows
}

// tagRows come from the deck itself — only a Moxfield deck whose author
// tagged their cards has any.
func tagRows(entries []deck.Card) []Row {
	seen := map[string]bool{}
	var labels []string
	for _, e := range entries {
		for _, t := range e.Tags {
			if !seen[t] {
				seen[t] = true
				labels = append(labels, t)
			}
		}
	}
	sort.Strings(labels)

	rows := make([]Row, 0, len(labels))
	for _, l := range labels {
		tag := l
		rows = append(rows, Row{
			Group: "Tags", Label: tag, Color: theme.Highlight,
			Match: func(ci deck.Card) bool {
				for _, t := range ci.Tags {
					if t == tag {
						return true
					}
				}
				return false
			},
		})
	}
	return rows
}

// Groups builds the category list from rowSource and counts it over
// counted. The two differ once you're filtering by a category: which rows
// exist, their order and their positions all come from the whole result set
// and so hold still, while the numbers beside them describe just the cards
// on screen — a category with nothing left in it stays put and reads zero.
func Groups(rowSource, counted []deck.Card) []Group {
	// Most telling first: what the deck's author called their cards, then
	// what those cards are, and the printing details last.
	groups := []Group{
		{Title: "Tags", Rows: tagRows(rowSource)},
		{Title: "Type", Rows: typeRows()},
		{Title: "Color (excl. lands)", Rows: colorRows()},
		{Title: "Mana Value (excl. lands)", Rows: cmcRows()},
		{Title: "Rarity", Rows: rarityRows(rowSource)},
	}

	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		kept := make([]Row, 0, len(g.Rows))
		for _, r := range g.Rows {
			for _, e := range rowSource {
				if r.Match(e) {
					r.Base += e.Qty
				}
			}
			// A category nothing in the deck has ever matched is left out
			// entirely; one the current filter has emptied is not.
			if r.Base == 0 {
				continue
			}
			for _, e := range counted {
				if r.Match(e) {
					r.Count += e.Qty
				}
			}
			kept = append(kept, r)
		}
		if len(kept) == 0 {
			continue
		}
		// Tags have no natural order, so the commonest lead — by their
		// standing in the whole set, so walking the list can't reorder it.
		if g.Title == "Tags" {
			sort.SliceStable(kept, func(i, j int) bool { return kept[i].Base > kept[j].Base })
		}
		out = append(out, Group{Title: g.Title, Rows: kept})
	}
	return out
}
