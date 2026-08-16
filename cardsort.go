package main

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// Sorting a list of cards on screen, which is a different thing from the
// order a Scryfall query asks for. That one is part of the search — it
// decides which 175 cards come back — and only ever applies to search
// results. This one rearranges whatever is already in front of you, results
// or deck, without asking anyone anything.

type cardSort int

const (
	// sortNone is the order the cards arrived in: Scryfall's for results,
	// decklist order for a deck.
	sortNone cardSort = iota
	sortName
	sortMana
	sortType
	sortColor
	sortEDHREC
)

var cardSorts = []cardSort{sortNone, sortName, sortMana, sortType, sortColor, sortEDHREC}

// name is what the header calls it. sortNone borrows the pane's own word for
// its arrival order, since "none" describes nothing useful.
func (s cardSort) name(dflt string) string {
	switch s {
	case sortName:
		return "name"
	case sortMana:
		return "mana value"
	case sortType:
		return "type"
	case sortColor:
		return "colour"
	case sortEDHREC:
		return "EDHREC rank"
	}
	return dflt
}

// next and prev cycle through the sorts, wrapping.
func (s cardSort) next(delta int) cardSort {
	n := len(cardSorts)
	return cardSort((int(s) + delta%n + n) % n)
}

// sortItems returns the items in the given order. The arrival order is kept
// as the tiebreak throughout — a stable sort over the list as it came in —
// so cards that compare equal stay in the order you last saw them.
//
// Commanders sort with everything else. They come first in decklist order
// because that's how the deck list is built, not because anything pins them
// there: sorting by mana value means by mana value.
func sortItems(items []list.Item, s cardSort) []list.Item {
	if s == sortNone || len(items) < 2 {
		return items
	}

	out := append([]list.Item(nil), items...)
	less := lessFor(s)
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := out[i].(cardItem)
		b, bok := out[j].(cardItem)
		if !aok || !bok {
			return false
		}
		return less(a.card, b.card)
	})
	return out
}

func lessFor(s cardSort) func(a, b ScryfallCard) bool {
	switch s {
	case sortName:
		return func(a, b ScryfallCard) bool { return a.Name < b.Name }

	case sortMana:
		return func(a, b ScryfallCard) bool {
			if a.CMC != b.CMC {
				return a.CMC < b.CMC
			}
			return a.Name < b.Name
		}

	case sortType:
		return func(a, b ScryfallCard) bool {
			ta, tb := typeRank(a.TypeLine), typeRank(b.TypeLine)
			if ta != tb {
				return ta < tb
			}
			return a.Name < b.Name
		}

	case sortColor:
		return func(a, b ScryfallCard) bool {
			ca, cb := colorRank(a), colorRank(b)
			if ca != cb {
				return ca < cb
			}
			if a.CMC != b.CMC {
				return a.CMC < b.CMC
			}
			return a.Name < b.Name
		}

	case sortEDHREC:
		return func(a, b ScryfallCard) bool {
			ra, rb := a.EDHRECRank, b.EDHRECRank
			// Unranked cards have no rank rather than rank zero, so they go
			// last instead of straight to the top.
			if ra == 0 {
				ra = 1 << 30
			}
			if rb == 0 {
				rb = 1 << 30
			}
			if ra != rb {
				return ra < rb
			}
			return a.Name < b.Name
		}
	}
	return func(a, b ScryfallCard) bool { return false }
}

// typeRank orders by the same precedence the deck list groups by, so sorting
// by type reads the way a decklist does.
func typeRank(typeLine string) int {
	primary := primaryType(typeLine)
	for i, t := range typePrecedence {
		if t == primary {
			return i
		}
	}
	return len(typePrecedence) // "Other"
}

// colorRank puts the mono-coloured cards in WUBRG order, then everything
// multicoloured, then the colourless — which is how a deck is usually laid
// out when it's laid out by colour at all.
func colorRank(c ScryfallCard) int {
	colors := c.displayColors()
	switch len(colors) {
	case 0:
		return 6
	case 1:
		if i := strings.Index("WUBRG", colors[0]); i >= 0 {
			return i
		}
		return 5
	}
	return 5
}
