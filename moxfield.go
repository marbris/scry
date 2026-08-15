package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Moxfield serves a deck's whole contents from one endpoint, keyed by the
// id in the deck's public URL. Every card entry carries a scryfall_id, so
// the deck itself is only a list of ids and quantities — the card data all
// comes from Scryfall, and deck cards behave like search results everywhere
// else in the app.
const moxfieldDeckURL = "https://api2.moxfield.com/v3/decks/all/%s"

// ── Moxfield types ──────────────────────────────────────────────

// Only the fields we use — the response also carries prices, comments and
// per-vendor purchase URLs for every card.
type moxDeck struct {
	Name      string              `json:"name"`
	Format    string              `json:"format"`
	PublicURL string              `json:"publicUrl"`
	Boards    map[string]moxBoard `json:"boards"`
	// The author's own tags for their cards ("Ramp", "Removal"), keyed by
	// card name and held once for the whole deck rather than per entry. It
	// keeps tags for cards that have since left the deck, so names that
	// don't match anything are simply ignored.
	AuthorTags    map[string][]string `json:"authorTags"`
	CreatedByUser struct {
		UserName string `json:"userName"`
	} `json:"createdByUser"`
}

type moxBoard struct {
	Count int                 `json:"count"`
	Cards map[string]moxEntry `json:"cards"`
}

type moxEntry struct {
	Quantity int `json:"quantity"`
	Card     struct {
		ScryfallID string `json:"scryfall_id"`
		Name       string `json:"name"`
	} `json:"card"`
}

// ── Deck references ─────────────────────────────────────────────

var (
	moxfieldURLRe = regexp.MustCompile(`(?i)\bmoxfield\.com/decks/([A-Za-z0-9_-]+)`)
	moxfieldIDRe  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// moxfieldURLID pulls the deck id out of a Moxfield URL. A bare id is not
// accepted here, so an ordinary one-word query is never mistaken for a deck.
func moxfieldURLID(s string) (string, bool) {
	m := moxfieldURLRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// deckRef accepts either form `scry deck` takes: a full URL, or just the id
// out of one.
func deckRef(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if id, ok := moxfieldURLID(s); ok {
		return id, true
	}
	if s != "" && moxfieldIDRe.MatchString(s) {
		return s, true
	}
	return "", false
}

// ── Loading ─────────────────────────────────────────────────────

func loadDeckCmd(id string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := loadDeck(id)
		return deckLoadedMsg{info: info, cards: cards, err: err}
	}
}

// loadDeck fetches a deck and swaps Moxfield's card stubs for full Scryfall
// cards. Only the command zone and the mainboard are loaded.
func loadDeck(id string) (deckInfo, []deckCard, error) {
	body, err := doGet(fmt.Sprintf(moxfieldDeckURL, url.PathEscape(id)))
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			return deckInfo{}, nil, fmt.Errorf("no public Moxfield deck %q", id)
		}
		return deckInfo{}, nil, err
	}

	var d moxDeck
	if err := json.Unmarshal(body, &d); err != nil {
		return deckInfo{}, nil, fmt.Errorf("moxfield: %w", err)
	}

	type stub struct {
		id        string
		qty       int
		commander bool
		tags      []string
	}
	var stubs []stub
	seen := map[string]bool{}

	collect := func(board string, commander bool) {
		for _, e := range d.Boards[board].Cards {
			sid := e.Card.ScryfallID
			if sid == "" || seen[sid] {
				continue
			}
			seen[sid] = true
			qty := e.Quantity
			if qty < 1 {
				qty = 1
			}
			stubs = append(stubs, stub{
				id: sid, qty: qty, commander: commander,
				tags: d.AuthorTags[e.Card.Name],
			})
		}
	}
	collect("commanders", true)
	collect("mainboard", false)

	if len(stubs) == 0 {
		return deckInfo{}, nil, fmt.Errorf("deck %q has no cards", id)
	}

	ids := make([]string, len(stubs))
	for i, s := range stubs {
		ids[i] = s.id
	}
	byID, err := fetchCollection(ids)
	if err != nil {
		return deckInfo{}, nil, err
	}

	info := deckInfo{
		name:   d.Name,
		author: d.CreatedByUser.UserName,
		format: d.Format,
		id:     id,
		url:    d.PublicURL,
	}
	if info.url == "" {
		info.url = "https://moxfield.com/decks/" + id
	}

	cards := make([]deckCard, 0, len(stubs))
	for _, s := range stubs {
		// A card Scryfall no longer serves under that id is dropped rather
		// than shown as a blank row.
		c, ok := byID[s.id]
		if !ok {
			continue
		}
		cards = append(cards, deckCard{card: c, qty: s.qty, commander: s.commander, tags: s.tags})
		info.total += s.qty
		info.unique++
	}
	if len(cards) == 0 {
		return deckInfo{}, nil, fmt.Errorf("none of deck %q's cards resolved on Scryfall", id)
	}

	return info, cards, nil
}
