package main

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// What the UI adds to a deck: the message it arrives in, the command that
// fetches it, and the ordering it's listed in. The deck itself lives in
// package deck.

type deckLoadedMsg struct {
	info  deckInfo
	cards []deckCard
	err   error
}

func openLocalDeckCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := openLocalDeck(slug)
		return deckLoadedMsg{info: info, cards: cards, err: err}
	}
}

// ── List layout ─────────────────────────────────────────────────

// deckItems orders the deck the way a decklist reads — commanders, then
// creatures, spells and lands, alphabetically inside each group. The groups
// aren't labelled: the statistics panel already breaks the deck down by
// type, and headings in the list only get in the way of scrolling it.
func deckItems(cards []deckCard) []list.Item {
	bySection := map[string][]deckCard{}
	for _, dc := range cards {
		sec := dc.Section()
		bySection[sec] = append(bySection[sec], dc)
	}

	var items []list.Item
	for _, sec := range deckSections {
		group := bySection[sec]
		sort.Slice(group, func(i, j int) bool {
			return group[i].Card.Name < group[j].Card.Name
		})
		for _, dc := range group {
			items = append(items, cardItem{
				Card: dc.Card, Qty: dc.Qty, Tags: dc.Tags, Commander: dc.Commander,
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
		out[strings.ToLower(dc.Card.Name)] = true
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
			out[strings.ToLower(ci.Card.Name)] = true
		}
	}
	return out
}
