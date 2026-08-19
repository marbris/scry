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

	// An empty deck is a deck — `scry deck new` makes one, and you fill it
	// by adding cards to it. Only a deck whose cards all failed to resolve
	// is a problem worth refusing to open.
	cards, resolveErr := resolveEntries(d.mainEntries())
	if len(cards) == 0 && len(d.mainEntries()) > 0 {
		return deckInfo{}, nil, resolveErr
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
			items = append(items, cardItem{
				card: dc.card, qty: dc.qty, tags: dc.tags, commander: dc.commander,
			})
		}
	}
	return items
}

// deckMembership is what the open deck holds, by lowercased name, for
// flagging the search results it already runs.
func (m model) deckMembership() map[string]bool {
	out := make(map[string]bool, len(m.deckCards))
	for _, dc := range m.deckCards {
		out[strings.ToLower(dc.card.Name)] = true
	}
	return out
}

// resultMembership is what the search turned up, for flagging the deck cards
// it matched. Built from the whole result set rather than what a filter
// leaves on screen: the question is whether the search found the card, not
// whether it's currently scrolled into view.
func (m model) resultMembership() map[string]bool {
	out := make(map[string]bool, len(m.results.baseItems))
	for _, it := range m.results.baseItems {
		if ci, ok := it.(cardItem); ok {
			out[strings.ToLower(ci.card.Name)] = true
		}
	}
	return out
}
