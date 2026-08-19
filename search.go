package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Everything that talks to Scryfall: the HTTP helpers, the search and
// rulings commands, and the collection endpoint decks resolve their cards
// through.

// ── Sort options ────────────────────────────────────────────────

var sortOptions = []string{
	"name",
	"released",
	"set",
	"rarity",
	"color",
	"usd",
	"cmc",
	"power",
	"toughness",
	"edhrec",
	"artist",
	"review",
}

// ── HTTP helper ─────────────────────────────────────────────────

// ── Commands ────────────────────────────────────────────────────

func searchScryfall(query string, sort string, limit int) tea.Cmd {
	return func() tea.Msg {
		var allCards []ScryfallCard
		page := 1

		for {
			u := fmt.Sprintf(
				"https://api.scryfall.com/cards/search?q=%s&order=%s&page=%d",
				url.QueryEscape(query), url.QueryEscape(sort), page,
			)
			body, err := doGet(u)
			if err != nil {
				if _, ok := err.(notFoundError); ok {
					return searchResultMsg{cards: []ScryfallCard{}, totalCards: 0}
				}
				if len(allCards) > 0 {
					break
				}
				return searchResultMsg{err: err}
			}

			var sr ScryfallResponse
			if err := json.Unmarshal(body, &sr); err != nil {
				return searchResultMsg{err: err}
			}

			allCards = append(allCards, sr.Data...)

			if len(allCards) >= limit || !sr.HasMore {
				if len(allCards) > limit {
					allCards = allCards[:limit]
				}
				return searchResultMsg{cards: allCards, totalCards: sr.TotalCards}
			}
			page++
		}

		if len(allCards) > limit {
			allCards = allCards[:limit]
		}
		return searchResultMsg{cards: allCards, totalCards: len(allCards)}
	}
}

func fetchRulings(key, uri string) tea.Cmd {
	return func() tea.Msg {
		rulings, err := getRulings(uri)
		if err != nil {
			return rulingsMsg{key: key, err: err}
		}
		return rulingsMsg{key: key, rulings: rulings}
	}
}

// getRulings fetches a card's rulings, returning an empty (non-nil) slice
// when the card simply has none.
func getRulings(uri string) ([]Ruling, error) {
	if uri == "" {
		return []Ruling{}, nil
	}
	body, err := doGet(uri)
	if err != nil {
		return nil, err
	}
	var rr RulingsResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, err
	}
	if rr.Data == nil {
		rr.Data = []Ruling{}
	}
	return rr.Data, nil
}

// loadRulesCmd parses the comprehensive rules off the main thread — it's
// a megabyte of text, and nothing needs it until a card is on screen.
func loadRulesCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := loadRules()
		return rulesLoadedMsg{data: data, err: err}
	}
}

// scryfallCollectionURL is a variable so a test can point it somewhere
// unreachable. Simulating a failure by setting HTTP_PROXY doesn't work:
// Go resolves the proxy once per process, so it leaks into every request
// made afterwards.
var scryfallCollectionURL = "https://api.scryfall.com/cards/collection"

const (
	// Scryfall takes up to 75 identifiers per collection request.
	collectionChunk = 75

	// Scryfall asks for a pause between requests; a Commander deck is two.
	collectionDelay = 100 * time.Millisecond
)

// fetchIdentifiers resolves Scryfall collection identifiers — {"id": …},
// {"name": …} or {"set": …, "collector_number": …} — 75 at a time. The
// response is unordered, so both the cards and the identifiers Scryfall
// couldn't match come back for the caller to arrange.
func fetchIdentifiers(idents []map[string]string) ([]ScryfallCard, []map[string]string, error) {
	var found []ScryfallCard
	var missing []map[string]string

	for start := 0; start < len(idents); start += collectionChunk {
		end := start + collectionChunk
		if end > len(idents) {
			end = len(idents)
		}

		var req struct {
			Identifiers []map[string]string `json:"identifiers"`
		}
		req.Identifiers = idents[start:end]
		payload, err := json.Marshal(req)
		if err != nil {
			return nil, nil, err
		}

		if start > 0 {
			time.Sleep(collectionDelay)
		}
		body, err := doPost(scryfallCollectionURL, payload)
		if err != nil {
			return nil, nil, err
		}

		var cr struct {
			Data     []ScryfallCard      `json:"data"`
			NotFound []map[string]string `json:"not_found"`
		}
		if err := json.Unmarshal(body, &cr); err != nil {
			return nil, nil, err
		}
		found = append(found, cr.Data...)
		missing = append(missing, cr.NotFound...)
	}
	return found, missing, nil
}

// fetchCollection looks up cards by Scryfall id, keyed by id for the caller
// to arrange.
func fetchCollection(ids []string) (map[string]ScryfallCard, error) {
	idents := make([]map[string]string, len(ids))
	for i, id := range ids {
		idents[i] = map[string]string{"id": id}
	}
	cards, _, err := fetchIdentifiers(idents)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ScryfallCard, len(cards))
	for _, c := range cards {
		out[c.ID] = c
	}
	return out, nil
}
