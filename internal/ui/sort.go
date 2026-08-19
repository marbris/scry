package ui

import (
	"sort"
	"strings"

	"scry/internal/deck"
	"scry/internal/mtg"
)

// How a list is ordered on screen, which is a different thing from the order
// a Scryfall query asks for. That one is part of the search — it decides
// which 175 cards come back — and only applies to search results. This one
// rearranges what is already in front of you, without asking anyone.
//
// The order also decides the list's second column: you sorted by a property,
// so that property is the one worth showing.

type cardSort int

const (
	// sortArrival is the order the cards came in: Scryfall's for results,
	// decklist order for a deck.
	sortArrival cardSort = iota
	sortMana
	sortName
	sortType
	sortColor
	sortRarity
	sortEDHREC
)

var cardSorts = []cardSort{
	sortArrival, sortMana, sortName, sortType, sortColor, sortRarity, sortEDHREC,
}

func (s cardSort) String() string {
	switch s {
	case sortMana:
		return "mana value"
	case sortName:
		return "name"
	case sortType:
		return "type"
	case sortColor:
		return "colour"
	case sortRarity:
		return "rarity"
	case sortEDHREC:
		return "edhrec"
	}
	return "as found"
}

// next cycles through the orders, wrapping.
func (s cardSort) next(delta int) cardSort {
	n := len(cardSorts)
	return cardSort((int(s) + delta%n + n) % n)
}

// column is what the second column shows under this order. Sorting by name
// or by arrival leaves nothing worth repeating, so both fall back to the
// mana cost — the one property you scan a list for regardless.
func (s cardSort) column(c mtg.Card) string {
	switch s {
	case sortType:
		return c.TypeLine
	case sortRarity:
		return c.Rarity
	case sortEDHREC:
		if c.EDHRECRank == 0 {
			return "—"
		}
		return itoa(c.EDHRECRank)
	}
	return manaCost(c)
}

// abbreviates reports whether this column can be shortened when the panel
// runs out of room. A type line can; a mana cost is already as short as it
// goes, and a rank abbreviated is a wrong number.
func (s cardSort) abbreviates() bool { return s == sortType }

// manaCost renders a cost the way the design asks for it: 3BG, no braces.
// Hybrid and phyrexian symbols keep their innards, since {W/U} shortened to
// anything is a different card.
func manaCost(c mtg.Card) string {
	cost := c.DisplayManaCost()
	if cost == "" {
		return ""
	}
	var b strings.Builder
	for _, sym := range strings.Split(cost, "}") {
		sym = strings.TrimPrefix(sym, "{")
		if sym == "" {
			continue
		}
		b.WriteString(sym)
	}
	return b.String()
}

// sortCards returns the cards in the given order. Arrival order is the
// tiebreak throughout — a stable sort over the list as it came in — so cards
// that compare equal stay where you last saw them.
//
// Commanders sort with everything else. They come first in decklist order
// because that is how a decklist is built, not because anything pins them
// there: sorting by mana value means by mana value.
func sortCards(cards []deck.Card, s cardSort) []deck.Card {
	if s == sortArrival || len(cards) < 2 {
		return cards
	}
	out := append([]deck.Card(nil), cards...)
	less := lessFor(s)
	sort.SliceStable(out, func(i, j int) bool { return less(out[i].Card, out[j].Card) })
	return out
}

func lessFor(s cardSort) func(a, b mtg.Card) bool {
	byName := func(a, b mtg.Card) bool { return a.Name < b.Name }

	switch s {
	case sortName:
		return byName

	case sortMana:
		return func(a, b mtg.Card) bool {
			if a.CMC != b.CMC {
				return a.CMC < b.CMC
			}
			return byName(a, b)
		}

	case sortType:
		return func(a, b mtg.Card) bool {
			ta, tb := typeRank(a.TypeLine), typeRank(b.TypeLine)
			if ta != tb {
				return ta < tb
			}
			return byName(a, b)
		}

	case sortColor:
		return func(a, b mtg.Card) bool {
			ca, cb := colorRank(a), colorRank(b)
			if ca != cb {
				return ca < cb
			}
			if a.CMC != b.CMC {
				return a.CMC < b.CMC
			}
			return byName(a, b)
		}

	case sortRarity:
		return func(a, b mtg.Card) bool {
			ra, rb := rarityRank(a.Rarity), rarityRank(b.Rarity)
			if ra != rb {
				return ra > rb // rarest first, which is what you're looking for
			}
			return byName(a, b)
		}

	case sortEDHREC:
		return func(a, b mtg.Card) bool {
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
			return byName(a, b)
		}
	}
	return func(a, b mtg.Card) bool { return false }
}

// typeRank orders by the precedence a decklist groups by, so sorting by type
// reads the way a decklist does.
func typeRank(typeLine string) int {
	primary := mtg.PrimaryType(typeLine)
	for i, t := range mtg.TypePrecedence {
		if t == primary {
			return i
		}
	}
	return len(mtg.TypePrecedence) // "Other"
}

// colorRank puts the mono-coloured cards in WUBRG order, then everything
// multicoloured, then the colourless — which is how a deck is laid out when
// it's laid out by colour at all.
func colorRank(c mtg.Card) int {
	colors := c.DisplayColors()
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

func rarityRank(r string) int {
	switch strings.ToLower(r) {
	case "mythic":
		return 4
	case "rare":
		return 3
	case "uncommon":
		return 2
	case "common":
		return 1
	}
	return 0
}
