package main

import (
	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/rules"
	"scry/internal/scryfall"
)

// The Bubbletea commands that run scryfall and rules work off the main
// thread. The work itself lives in those packages; what's here is only the
// wrapping that turns an answer into a message.

func searchScryfall(query string, sort string, limit int) tea.Cmd {
	return func() tea.Msg {
		cards, total, err := scryfall.Search(query, sort, limit)
		return searchResultMsg{cards: cards, totalCards: total, err: err}
	}
}

func fetchRulings(key, uri string) tea.Cmd {
	return func() tea.Msg {
		rulings, err := scryfall.Rulings(uri)
		if err != nil {
			return rulingsMsg{key: key, err: err}
		}
		return rulingsMsg{key: key, rulings: rulings}
	}
}

// loadRulesCmd parses the comprehensive rules off the main thread — it's a
// megabyte of text, and nothing needs it until a card is on screen.
func loadRulesCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := rules.Load()
		return rulesLoadedMsg{data: data, err: err}
	}
}
