package main

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// ── Deck model ──────────────────────────────────────────────────

type deckInfo struct {
	name   string
	author string
	format string
	id     string
	url    string
	total  int // cards counting quantities
	unique int // distinct cards
}

type deckCard struct {
	card      ScryfallCard
	qty       int
	commander bool
	tags      []string
}

// section is the heading a card is listed under.
func (d deckCard) section() string {
	if d.commander {
		return "Commander"
	}
	return primaryType(d.card.TypeLine)
}

type deckLoadedMsg struct {
	info  deckInfo
	cards []deckCard
	err   error
}

// deckSections is the order the list is grouped in: the command zone first,
// then spells roughly in the order they get cast, then lands.
var deckSections = []string{
	"Commander", "Creature", "Planeswalker", "Battle",
	"Instant", "Sorcery", "Artifact", "Enchantment", "Land", "Other",
}

// typePrecedence decides the one section a card with several types lands in.
// Creature wins over everything, so an Artifact Creature is a creature; Land
// comes before Artifact and Enchantment, so an artifact land is a land.
var typePrecedence = []string{
	"Creature", "Planeswalker", "Battle", "Land",
	"Instant", "Sorcery", "Artifact", "Enchantment",
}

func primaryType(typeLine string) string {
	// Modal double-faced cards join their halves with "//"; the front face
	// is the one that decides where the card is listed.
	if i := strings.Index(typeLine, "//"); i >= 0 {
		typeLine = typeLine[:i]
	}
	for _, t := range typePrecedence {
		if strings.Contains(typeLine, t) {
			return t
		}
	}
	return "Other"
}

// ── List layout ─────────────────────────────────────────────────

// deckItems orders the deck the way a decklist reads — commanders, then
// creatures, spells and lands, alphabetically inside each group. The groups
// aren't labelled: the statistics panel already breaks the deck down by
// type, and headings in the list only get in the way of scrolling it.
func deckItems(cards []deckCard) []list.Item {
	bySection := map[string][]deckCard{}
	for _, dc := range cards {
		sec := dc.section()
		bySection[sec] = append(bySection[sec], dc)
	}

	var items []list.Item
	for _, sec := range deckSections {
		group := bySection[sec]
		sort.Slice(group, func(i, j int) bool {
			return group[i].card.Name < group[j].card.Name
		})
		for _, dc := range group {
			items = append(items, cardItem{card: dc.card, qty: dc.qty, tags: dc.tags})
		}
	}
	return items
}
