package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

	// slug names the file this deck was read from, and is empty for a deck
	// being browsed straight off Moxfield. Editing needs somewhere to write
	// to, so it's what tells an editable deck from a borrowed one.
	slug string
}

func (d deckInfo) local() bool { return d.slug != "" }

// ref is what the search bar shows while the deck is open: the URL it came
// from, or the command that would reopen it.
func (d deckInfo) ref() string {
	if d.local() {
		return "deck " + d.slug
	}
	return d.url
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

// deckFileFrom builds a deck file out of a deck that's already open, so
// saving what's on screen never needs to fetch it again — and works with no
// network at all.
func deckFileFrom(info deckInfo, cards []deckCard) *deckFile {
	d := &deckFile{Name: info.name, Format: info.format, Source: info.url}
	for _, dc := range cards {
		section := "mainboard"
		if dc.commander {
			section = "commander"
		}
		d.Entries = append(d.Entries, deckEntry{
			Qty:     dc.qty,
			Name:    dc.card.Name,
			Tags:    dc.tags,
			Section: section,
		})
	}
	return d
}

// ── Opening a local deck ────────────────────────────────────────

func openLocalDeckCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := openLocalDeck(slug)
		return deckLoadedMsg{info: info, cards: cards, err: err}
	}
}

// openLocalDeck reads a deck file and resolves its card names. Cards that
// won't resolve don't stop the deck opening — the error rides alongside the
// cards that did, for the caller to show as a notice.
func openLocalDeck(slug string) (deckInfo, []deckCard, error) {
	d, err := readDeck(slug)
	if err != nil {
		if os.IsNotExist(err) {
			return deckInfo{}, nil, fmt.Errorf("no saved deck %q", slug)
		}
		return deckInfo{}, nil, err
	}

	cards, resolveErr := resolveEntries(d.mainEntries())
	if len(cards) == 0 {
		if resolveErr != nil {
			return deckInfo{}, nil, resolveErr
		}
		return deckInfo{}, nil, fmt.Errorf("deck %q has no cards", slug)
	}

	total, unique := d.counts()
	info := deckInfo{
		name:   d.Name,
		format: d.Format,
		url:    d.Source,
		slug:   slug,
		total:  total,
		unique: unique,
	}
	return info, cards, resolveErr
}

// mainEntries is the deck proper — everything a maybeboard shortlist isn't.
func (d *deckFile) mainEntries() []deckEntry {
	out := make([]deckEntry, 0, len(d.Entries))
	for _, e := range d.Entries {
		if e.Section != "maybeboard" {
			out = append(out, e)
		}
	}
	return out
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
