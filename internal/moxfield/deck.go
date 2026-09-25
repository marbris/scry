package moxfield

import (
	"scry/internal/scryfall"

	"scry/internal/fetch"

	"scry/internal/deck"

	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Moxfield serves a deck's whole contents from one endpoint, keyed by the
// id in the deck's public URL. Every card entry carries a scryfall_id, so
// the deck itself is only a list of ids and quantities — the card data all
// comes from Scryfall, and deck cards behave like search results everywhere
// else in the app.
const deckURL = "https://api2.moxfield.com/v3/decks/all/%s"

// ── Moxfield types ──────────────────────────────────────────────

// Only the fields we use — the response also carries prices, comments and
// per-vendor purchase URLs for every card.
type Deck struct {
	Name      string           `json:"name"`
	Format    string           `json:"format"`
	PublicURL string           `json:"publicUrl"`
	Boards    map[string]Board `json:"boards"`
	// The author's own tags for their cards ("Ramp", "Removal"), keyed by
	// card name and held once for the whole deck rather than per entry. It
	// keeps tags for cards that have since left the deck, so names that
	// don't match anything are simply ignored.
	AuthorTags    map[string][]string `json:"authorTags"`
	CreatedByUser struct {
		UserName string `json:"userName"`
	} `json:"createdByUser"`
	// The summary fields the decks list shows beside a followed deck without
	// opening it. Moxfield sends these on the full deck too, the same names the
	// search endpoint uses, so a remote can carry its colours, size and age.
	ColorIdentity []string `json:"colorIdentity"`
	LastUpdated   string   `json:"lastUpdatedAtUtc"`
}

type Board struct {
	Count int              `json:"count"`
	Cards map[string]Entry `json:"cards"`
}

type Entry struct {
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

// URLID pulls the deck id out of a Moxfield URL. A bare id is not
// accepted here, so an ordinary one-word query is never mistaken for a deck.
func URLID(s string) (string, bool) {
	m := moxfieldURLRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// idLen is the shortest bare string taken for a deck id. Moxfield's
// are 22 characters; your own decks have names like "ghen" or "winota". A
// floor well above one and well below the other means a mistyped deck name
// is answered with "no deck called that" rather than a fruitless trip to
// Moxfield. A full URL is always taken as one, however short.
const idLen = 16

// Ref accepts either form `scry deck` takes: a full URL, or just the id
// out of one.
func Ref(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if id, ok := URLID(s); ok {
		return id, true
	}
	if len(s) >= idLen && moxfieldIDRe.MatchString(s) {
		return s, true
	}
	return "", false
}

// ── Loading ─────────────────────────────────────────────────────

func Fetch(id string) (Deck, error) {
	body, err := fetch.Get(fmt.Sprintf(deckURL, url.PathEscape(id)))
	if err != nil {
		if _, ok := err.(fetch.NotFound); ok {
			return Deck{}, fmt.Errorf("no public Moxfield deck %q", id)
		}
		return Deck{}, err
	}

	var d Deck
	if err := json.Unmarshal(body, &d); err != nil {
		return Deck{}, fmt.Errorf("moxfield: %w", err)
	}
	return d, nil
}

// Meta is the handful of facts the decks list shows beside a followed deck
// without opening it: its colours, its size, and when it last changed. Enough
// to tell one bookmark from another at a glance.
type Meta struct {
	Colors  []string
	Count   int
	Updated time.Time
}

// FetchMeta pulls just those summary facts for a deck. It's one request, and
// unlike Load it doesn't resolve every card on Scryfall — the decks list wants
// the shape of a deck, not its contents.
func FetchMeta(id string) (Meta, error) {
	d, err := Fetch(id)
	if err != nil {
		return Meta{}, err
	}
	return d.meta(), nil
}

// meta reads the summary out of a fetched deck. The count is the command zone
// and mainboard together — the cards the deck is, not its sideboard or the
// maybeboard it's still deciding on.
func (d Deck) meta() Meta {
	count := 0
	for _, board := range []string{"commanders", "mainboard"} {
		for _, e := range d.Boards[board].Cards {
			count += maxInt(e.Quantity, 1)
		}
	}
	var updated time.Time
	if t, err := time.Parse(time.RFC3339, d.LastUpdated); err == nil {
		updated = t
	}
	return Meta{Colors: d.ColorIdentity, Count: count, Updated: updated}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// boardOrder is the order boards are read in, and the deck file section
// each becomes. Moxfield carries more boards than these; the rest are a
// deck's history rather than the deck, and aren't imported.
var boardOrder = []struct{ board, section string }{
	{"commanders", "commander"},
	{"mainboard", "mainboard"},
}

// Import turns a Moxfield deck into a deck file. Cards are recorded
// by name rather than by printing: the printing Moxfield happens to hold
// isn't one you chose, and pinning all hundred lines would bury the diffs
// the format exists to keep readable.
func Import(id string) (*deck.File, error) {
	d, err := Fetch(id)
	if err != nil {
		return nil, err
	}
	return ToFile(d, id)
}

// ToFile is the conversion itself, kept apart from the fetch so it
// can be exercised without the network.
func ToFile(d Deck, id string) (*deck.File, error) {
	out := &deck.File{Name: deck.ImportName(d.Name), Format: d.Format, Source: d.PublicURL}
	if out.Source == "" {
		out.Source = "https://moxfield.com/decks/" + id
	}

	seen := map[string]bool{}
	for _, b := range boardOrder {
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
			out.Entries = append(out.Entries, deck.Entry{
				Qty:     qty,
				Name:    name,
				Tags:    deck.ParseTags(strings.Join(d.AuthorTags[name], ",")),
				Section: b.section,
			})
		}
	}

	if len(out.Entries) == 0 {
		return nil, fmt.Errorf("deck %q has no cards", id)
	}
	return out, nil
}

// consideringBoard is Moxfield's key for the "Considering" list — the cards an
// author is weighing but hasn't put in the deck. It's the maybeboard under an
// older name.
const consideringBoard = "maybeboard"

// ImportConsidering builds a deck file from a Moxfield deck's Considering list
// alone, so it can be taken as a local deck of its own beside the main copy.
// Nil, with no error, when the author is considering nothing — an empty list is
// not a failure, just nothing to copy.
func ImportConsidering(id string) (*deck.File, error) {
	d, err := Fetch(id)
	if err != nil {
		return nil, err
	}

	out := &deck.File{Name: deck.ImportName(d.Name) + " (considering)", Format: d.Format, Source: d.PublicURL}
	if out.Source == "" {
		out.Source = "https://moxfield.com/decks/" + id
	}

	seen := map[string]bool{}
	for _, e := range d.Boards[consideringBoard].Cards {
		name := strings.TrimSpace(e.Card.Name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out.Entries = append(out.Entries, deck.Entry{
			Qty:     maxInt(e.Quantity, 1),
			Name:    name,
			Tags:    deck.ParseTags(strings.Join(d.AuthorTags[name], ",")),
			Section: "mainboard",
		})
	}

	if len(out.Entries) == 0 {
		return nil, nil
	}
	return out, nil
}

// Load fetches a deck and swaps Moxfield's card stubs for full Scryfall
// cards. Only the command zone and the mainboard are loaded.
func Load(id string) (deck.Info, []deck.Card, error) {
	d, err := Fetch(id)
	if err != nil {
		return deck.Info{}, nil, err
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
		return deck.Info{}, nil, fmt.Errorf("deck %q has no cards", id)
	}

	ids := make([]string, len(stubs))
	for i, s := range stubs {
		ids[i] = s.id
	}
	byID, err := scryfall.Collection(ids)
	if err != nil {
		return deck.Info{}, nil, err
	}

	info := deck.Info{
		Name:   d.Name,
		Author: d.CreatedByUser.UserName,
		Format: d.Format,
		ID:     id,
		URL:    d.PublicURL,
	}
	if info.URL == "" {
		info.URL = "https://moxfield.com/decks/" + id
	}

	cards := make([]deck.Card, 0, len(stubs))
	for _, s := range stubs {
		// A card Scryfall no longer serves under that id is dropped rather
		// than shown as a blank row.
		c, ok := byID[s.id]
		if !ok {
			continue
		}
		cards = append(cards, deck.Card{Card: c, Qty: s.qty, Commander: s.commander, Tags: s.tags})
		info.Total += s.qty
		info.Unique++
	}
	if len(cards) == 0 {
		return deck.Info{}, nil, fmt.Errorf("none of deck %q's cards resolved on Scryfall", id)
	}

	return info, cards, nil
}
