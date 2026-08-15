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

// moxfieldIDLen is the shortest bare string taken for a deck id. Moxfield's
// are 22 characters; your own decks have names like "ghen" or "winota". A
// floor well above one and well below the other means a mistyped deck name
// is answered with "no deck called that" rather than a fruitless trip to
// Moxfield. A full URL is always taken as one, however short.
const moxfieldIDLen = 16

// deckRef accepts either form `scry deck` takes: a full URL, or just the id
// out of one.
func deckRef(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if id, ok := moxfieldURLID(s); ok {
		return id, true
	}
	if len(s) >= moxfieldIDLen && moxfieldIDRe.MatchString(s) {
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

// fetchMoxfield pulls a deck's JSON. Browsing and importing both start here
// and part ways afterwards: browsing keeps Moxfield's exact printings by
// resolving the scryfall_ids, while importing writes plain card names.
func fetchMoxfield(id string) (moxDeck, error) {
	body, err := doGet(fmt.Sprintf(moxfieldDeckURL, url.PathEscape(id)))
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			return moxDeck{}, fmt.Errorf("no public Moxfield deck %q", id)
		}
		return moxDeck{}, err
	}

	var d moxDeck
	if err := json.Unmarshal(body, &d); err != nil {
		return moxDeck{}, fmt.Errorf("moxfield: %w", err)
	}
	return d, nil
}

// moxBoardOrder is the order boards are read in, and the deck file section
// each becomes. Moxfield carries more boards than these; the rest are a
// deck's history rather than the deck, and aren't imported.
var moxBoardOrder = []struct{ board, section string }{
	{"commanders", "commander"},
	{"mainboard", "mainboard"},
}

// importMoxfield turns a Moxfield deck into a deck file. Cards are recorded
// by name rather than by printing: the printing Moxfield happens to hold
// isn't one you chose, and pinning all hundred lines would bury the diffs
// the format exists to keep readable.
func importMoxfield(id string) (*deckFile, error) {
	d, err := fetchMoxfield(id)
	if err != nil {
		return nil, err
	}
	return moxDeckToFile(d, id)
}

// moxDeckToFile is the conversion itself, kept apart from the fetch so it
// can be exercised without the network.
func moxDeckToFile(d moxDeck, id string) (*deckFile, error) {
	out := &deckFile{Name: d.Name, Format: d.Format, Source: d.PublicURL}
	if out.Source == "" {
		out.Source = "https://moxfield.com/decks/" + id
	}

	seen := map[string]bool{}
	for _, b := range moxBoardOrder {
		// Moxfield keys a board's cards by an opaque id, so the order they
		// come back in isn't stable; the deck file is sorted on write, which
		// makes that moot.
		for _, e := range d.Boards[b.board].Cards {
			name := strings.TrimSpace(e.Card.Name)
			if name == "" || seen[strings.ToLower(name)] {
				continue
			}
			seen[strings.ToLower(name)] = true

			qty := e.Quantity
			if qty < 1 {
				qty = 1
			}
			out.Entries = append(out.Entries, deckEntry{
				Qty:     qty,
				Name:    name,
				Tags:    parseTags(strings.Join(d.AuthorTags[name], ",")),
				Section: b.section,
			})
		}
	}

	if len(out.Entries) == 0 {
		return nil, fmt.Errorf("deck %q has no cards", id)
	}
	return out, nil
}

// loadDeck fetches a deck and swaps Moxfield's card stubs for full Scryfall
// cards. Only the command zone and the mainboard are loaded.
func loadDeck(id string) (deckInfo, []deckCard, error) {
	d, err := fetchMoxfield(id)
	if err != nil {
		return deckInfo{}, nil, err
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
