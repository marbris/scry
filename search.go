package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

type notFoundError struct{}

func (e notFoundError) Error() string { return "no results found" }

func doGet(u string) ([]byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json;q=0.9,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, notFoundError{}
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s (%d): %s", hostOf(u), resp.StatusCode, string(body))
	}
	return body, nil
}

// hostOf labels an error with the service that produced it — cards and
// rulings come from Scryfall, decks from Moxfield.
func hostOf(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return "request"
	}
	return strings.TrimPrefix(parsed.Host, "api.")
}

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

const (
	// Scryfall takes up to 75 identifiers per collection request.
	collectionChunk = 75

	// Scryfall asks for a pause between requests; a Commander deck is two.
	collectionDelay = 100 * time.Millisecond
)

// fetchCollection looks up cards by Scryfall id, 75 at a time. The response
// isn't ordered, so it comes back keyed by id for the caller to arrange.
func fetchCollection(ids []string) (map[string]ScryfallCard, error) {
	out := make(map[string]ScryfallCard, len(ids))

	for start := 0; start < len(ids); start += collectionChunk {
		end := start + collectionChunk
		if end > len(ids) {
			end = len(ids)
		}

		var req struct {
			Identifiers []map[string]string `json:"identifiers"`
		}
		for _, id := range ids[start:end] {
			req.Identifiers = append(req.Identifiers, map[string]string{"id": id})
		}
		payload, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}

		if start > 0 {
			time.Sleep(collectionDelay)
		}
		body, err := doPost("https://api.scryfall.com/cards/collection", payload)
		if err != nil {
			return nil, err
		}

		var cr struct {
			Data []ScryfallCard `json:"data"`
		}
		if err := json.Unmarshal(body, &cr); err != nil {
			return nil, err
		}
		for _, c := range cr.Data {
			out[c.ID] = c
		}
	}
	return out, nil
}

func doPost(u string, payload []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", u, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s (%d)", hostOf(u), resp.StatusCode)
	}
	return body, nil
}
