package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A trimmed Moxfield response: two boards, author tags, and the quirks that
// matter — a quantity above one, a card tagged in mixed case, and a tag for
// a card that has since left the deck.
const moxfieldFixture = `{
  "name": "Winota: Snowball Stax",
  "format": "commander",
  "publicUrl": "https://moxfield.com/decks/Y8dZ7",
  "createdByUser": {"userName": "someone"},
  "authorTags": {
    "Sol Ring": ["Ramp", "ARTIFACT"],
    "Winota, Joiner of Forces": ["wincon"],
    "A Card No Longer Here": ["ramp"]
  },
  "boards": {
    "commanders": {"count": 1, "cards": {
      "a1": {"quantity": 1, "card": {"scryfall_id": "id-winota", "name": "Winota, Joiner of Forces"}}
    }},
    "mainboard": {"count": 3, "cards": {
      "b1": {"quantity": 1, "card": {"scryfall_id": "id-sol", "name": "Sol Ring"}},
      "b2": {"quantity": 7, "card": {"scryfall_id": "id-plains", "name": "Plains"}},
      "b3": {"quantity": 0, "card": {"scryfall_id": "id-fire", "name": "Fire // Ice"}}
    }},
    "sideboard": {"count": 1, "cards": {
      "c1": {"quantity": 1, "card": {"scryfall_id": "id-x", "name": "Should Not Be Imported"}}
    }}
  }
}`

// moxToDeckFile runs the real conversion over fixture JSON, with only the
// fetch left out.
func moxToDeckFile(t *testing.T, body string) *deckFile {
	t.Helper()
	var d moxDeck
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	out, err := moxDeckToFile(d, "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMoxfieldImportConversion(t *testing.T) {
	d := moxToDeckFile(t, moxfieldFixture)

	if d.Name != "Winota: Snowball Stax" || d.Format != "commander" {
		t.Errorf("header = %q / %q", d.Name, d.Format)
	}
	if d.Source != "https://moxfield.com/decks/Y8dZ7" {
		t.Errorf("source = %q", d.Source)
	}

	// Only the command zone and mainboard come across — a sideboard is not
	// part of a Commander deck.
	if len(d.Entries) != 4 {
		t.Fatalf("imported %d cards, want 4: %+v", len(d.Entries), d.Entries)
	}

	byName := map[string]deckEntry{}
	for _, e := range d.Entries {
		byName[e.Name] = e
	}

	if e := byName["Winota, Joiner of Forces"]; e.Section != "commander" {
		t.Errorf("commander landed in %q", e.Section)
	}
	if e := byName["Sol Ring"]; e.Section != "mainboard" {
		t.Errorf("Sol Ring landed in %q", e.Section)
	}
	if _, ok := byName["Should Not Be Imported"]; ok {
		t.Error("the sideboard came across")
	}

	// Moxfield's own tags are kept, normalised the way tags are everywhere.
	if got := byName["Sol Ring"].Tags; len(got) != 2 || got[0] != "artifact" || got[1] != "ramp" {
		t.Errorf("Sol Ring tags = %v, want [artifact ramp]", got)
	}
	// A tag for a card that isn't in the deck brings nothing with it.
	if _, ok := byName["A Card No Longer Here"]; ok {
		t.Error("a stale author tag conjured up a card")
	}

	if got := byName["Plains"].Qty; got != 7 {
		t.Errorf("Plains qty = %d, want 7", got)
	}
	// Moxfield occasionally reports a zero quantity; one is the sane read.
	if got := byName["Fire // Ice"].Qty; got != 1 {
		t.Errorf("zero quantity became %d, want 1", got)
	}

	// Nothing is pinned to a printing — that is the point of importing by
	// name, and what keeps the diffs readable.
	for _, e := range d.Entries {
		if e.Set != "" || e.Collector != "" {
			t.Errorf("%q was pinned to a printing: %+v", e.Name, e)
		}
	}
}

func TestImportedDeckIsWellFormed(t *testing.T) {
	t.Setenv("SCRY_DECKS_DIR", t.TempDir())

	d := moxToDeckFile(t, moxfieldFixture)
	if err := writeDeck("winota", d); err != nil {
		t.Fatal(err)
	}

	back, err := readDeck("winota")
	if err != nil {
		t.Fatalf("an imported deck must read back: %v", err)
	}
	if len(back.Entries) != len(d.Entries) {
		t.Errorf("read back %d entries, wrote %d", len(back.Entries), len(d.Entries))
	}

	total, unique := back.Counts()
	if total != 10 || unique != 4 {
		t.Errorf("counts = (%d, %d), want (10, 4)", total, unique)
	}

	// The file a person would open.
	text := back.String()
	for _, want := range []string{
		"name: Winota: Snowball Stax",
		"format: commander",
		"source: https://moxfield.com/decks/Y8dZ7",
		"[commander]",
		"1 Winota, Joiner of Forces [wincon]",
		"[mainboard]",
		"1 Sol Ring [artifact, ramp]",
		"7 Plains",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("imported deck file is missing %q:\n%s", want, text)
		}
	}
}

func TestDeckFileFromOpenDeck(t *testing.T) {
	// What w does: turn the deck on screen back into a file, without
	// going near the network.
	info := deckInfo{
		Name:   "Winota: Snowball Stax",
		Format: "commander",
		URL:    "https://moxfield.com/decks/Y8dZ7",
	}
	cards := []deckCard{
		{Card: ScryfallCard{Name: "Winota, Joiner of Forces"}, Qty: 1, Commander: true, Tags: []string{"wincon"}},
		{Card: ScryfallCard{Name: "Sol Ring"}, Qty: 1, Tags: []string{"ramp"}},
		{Card: ScryfallCard{Name: "Plains"}, Qty: 7},
	}

	d := deckFileFrom(info, cards)
	if d.Name != info.Name || d.Format != info.Format || d.Source != info.URL {
		t.Errorf("header lost: %+v", d)
	}
	if len(d.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(d.Entries))
	}

	var commanders int
	for _, e := range d.Entries {
		if e.Section == "commander" {
			commanders++
			if e.Name != "Winota, Joiner of Forces" {
				t.Errorf("wrong card in the command zone: %q", e.Name)
			}
		}
	}
	if commanders != 1 {
		t.Errorf("got %d commanders, want 1", commanders)
	}

	if !strings.Contains(d.String(), "7 Plains") {
		t.Errorf("quantities lost:\n%s", d.String())
	}
	if !strings.Contains(d.String(), "1 Sol Ring [ramp]") {
		t.Errorf("tags lost:\n%s", d.String())
	}
}

func TestDeckRefDoesNotSwallowDeckNames(t *testing.T) {
	// A mistyped deck name must not be taken for a Moxfield id, or the only
	// answer you get is a trip to Moxfield for a deck that was never there.
	for _, name := range []string{"ghen", "winota", "nosuchdeck", "my-deck-1"} {
		if id, ok := deckRef(name); ok {
			t.Errorf("deckRef(%q) = %q, should not look like a Moxfield id", name, id)
		}
	}

	// Real references still resolve.
	for ref, want := range map[string]string{
		"pdxwlkCOVkSQB2-6FYBvog":                            "pdxwlkCOVkSQB2-6FYBvog",
		"https://moxfield.com/decks/pdxwlkCOVkSQB2-6FYBvog": "pdxwlkCOVkSQB2-6FYBvog",
		// A URL is trusted however short the id in it is.
		"https://moxfield.com/decks/abc": "abc",
	} {
		got, ok := deckRef(ref)
		if !ok || got != want {
			t.Errorf("deckRef(%q) = %q, %v; want %q, true", ref, got, ok, want)
		}
	}
}

func TestDeckInfoLocal(t *testing.T) {
	// A deck read from a file can be edited; one being browsed off Moxfield
	// is somebody else's.
	if !(deckInfo{Slug: "ghen"}).Local() {
		t.Error("a deck with a slug should be local")
	}
	if (deckInfo{URL: "https://moxfield.com/decks/AbC123"}).Local() {
		t.Error("a deck with only a URL should not be local")
	}
}

func TestOpenLocalDeckResolvesThroughTheCache(t *testing.T) {
	t.Setenv("SCRY_DECKS_DIR", t.TempDir())
	seedCache(t, map[string]ScryfallCard{
		"winota, joiner of forces": {Name: "Winota, Joiner of Forces", TypeLine: "Legendary Creature — Human Soldier"},
		"sol ring":                 {Name: "Sol Ring", TypeLine: "Artifact"},
		"plains":                   {Name: "Plains", TypeLine: "Basic Land — Plains"},
	})

	d, err := parseDeckFile(strings.NewReader(`name: Winota
format: commander

[commander]
1 Winota, Joiner of Forces [wincon]

[mainboard]
1 Sol Ring [ramp]
7 Plains

[maybeboard]
1 Some Card I Am Considering
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeDeck("winota", d); err != nil {
		t.Fatal(err)
	}

	info, cards, err := openLocalDeck("winota")
	if err != nil {
		t.Fatalf("openLocalDeck: %v", err)
	}

	if info.Slug != "winota" || !info.Local() {
		t.Errorf("info = %+v, want a local deck", info)
	}
	if info.Name != "Winota" || info.Format != "commander" {
		t.Errorf("info header = %+v", info)
	}
	if info.Total != 9 || info.Unique != 3 {
		t.Errorf("counts = (%d, %d), want (9, 3)", info.Total, info.Unique)
	}

	// The maybeboard is a shortlist, so it isn't resolved or shown, and its
	// unresolvable card must not be reported as a problem.
	if len(cards) != 3 {
		t.Fatalf("got %d cards, want 3 (the maybeboard should be left out)", len(cards))
	}
	for _, c := range cards {
		if c.Card.Name == "Some Card I Am Considering" {
			t.Error("the maybeboard was loaded into the deck")
		}
	}

	var commanders int
	for _, c := range cards {
		if c.Commander {
			commanders++
		}
	}
	if commanders != 1 {
		t.Errorf("got %d commanders, want 1", commanders)
	}
}

func TestLocalDeckInTheApp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SCRY_DECKS_DIR", t.TempDir())

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{
		info: deckInfo{
			Name: "Ghen reanimator", Format: "commander", Slug: "ghen",
			URL: "https://moxfield.com/decks/AbC123", Total: 100,
		},
		cards: deckFixture(),
	})

	// The search bar goes to Scryfall, so opening a deck leaves it alone.
	// It used to be filled with "deck ghen", which isn't a query and which
	// you had to clear before you could search for anything.
	if got := m.searchInput.Value(); got != "" {
		t.Errorf("opening a deck wrote %q into the search bar", got)
	}

	// w writes the open deck; with nothing changed there is nothing to write.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if !strings.Contains(m.notice, "up to date") {
		t.Errorf("w on an unchanged local deck said %q", m.notice)
	}

	// And a deck that already lives on disk has nothing to import.
	m = leaderPress(m, "i")
	if !strings.Contains(m.notice, "already saved") {
		t.Errorf(",i on a local deck said %q", m.notice)
	}
	if deckExists("ghen-reanimator") {
		t.Error("w wrote a second copy of a deck that was already local")
	}
}

func TestDeckLoadWithUnresolvedCardsStillOpens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SCRY_DECKS_DIR", t.TempDir())

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Cards that wouldn't resolve are worth a notice, but the deck still
	// has to open — the rest of it is fine.
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{Name: "Ghen reanimator", Slug: "ghen", Total: 100},
		cards: deckFixture(),
		err:   unresolvedError{Names: []string{"Nonesuch"}},
	})

	if m.deck == nil {
		t.Fatal("the deck did not open")
	}
	if m.err != nil {
		t.Errorf("an unresolved card became a fatal error: %v", m.err)
	}
	if !strings.Contains(m.notice, "Nonesuch") {
		t.Errorf("notice = %q, should name the card that went missing", m.notice)
	}

	// With nothing resolved at all there's no deck to show, so it is an error.
	m2 := initialModel()
	m2 = drive(m2, tea.WindowSizeMsg{Width: 160, Height: 40})
	m2 = drive(m2, deckLoadedMsg{err: unresolvedError{Names: []string{"Nonesuch"}}})
	if m2.err == nil {
		t.Error("a deck with no cards at all should be an error")
	}
}

func TestOpenLocalDeckMissingFile(t *testing.T) {
	t.Setenv("SCRY_DECKS_DIR", t.TempDir())

	_, _, err := openLocalDeck("nothing-here")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "no saved deck") {
		t.Errorf("unhelpful error for a missing deck: %v", err)
	}
}
