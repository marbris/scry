package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Moxfield serves a deck's whole contents from one endpoint, keyed by the
// id in the deck's public URL. Every card entry carries a scryfall_id, so
// the deck itself is only a list of ids and quantities — the card data all
// comes from Scryfall, and deck cards behave like search results everywhere
// else in the app.
const (
	moxfieldDeckURL = "https://api2.moxfield.com/v3/decks/all/%s"

	// Scryfall takes up to 75 identifiers per collection request.
	collectionChunk = 75

	// Scryfall asks for a pause between requests; a Commander deck is two.
	collectionDelay = 100 * time.Millisecond
)

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
