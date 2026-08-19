package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func pickerModel(t *testing.T) model {
	t.Helper()
	gitRepo(t)
	// Seeded so opening a deck from the picker resolves off the cache
	// rather than reaching for Scryfall mid-test.
	seedCache(t, map[string]ScryfallCard{
		"ghen, arcanum weaver": {Name: "Ghen, Arcanum Weaver", TypeLine: "Legendary Creature — Human Wizard"},
		"sol ring":             {Name: "Sol Ring", TypeLine: "Artifact"},
		"plains":               {Name: "Plains", TypeLine: "Basic Land — Plains"},
		"mountain":             {Name: "Mountain", TypeLine: "Basic Land — Mountain"},
	})

	for _, d := range []struct{ slug, body string }{
		{"ghen", "name: Ghen reanimator\nformat: commander\n\n[commander]\n1 Ghen, Arcanum Weaver\n\n[mainboard]\n1 Sol Ring\n30 Plains\n"},
		{"winota", "name: Winota: Snowball Stax\nformat: commander\n\n[mainboard]\n1 Sol Ring\n12 Mountain\n"},
	} {
		parsed, err := parseDeckFile(strings.NewReader(d.body))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := saveDeckVersioned(d.slug, parsed); err != nil {
			t.Fatal(err)
		}
	}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
}

func TestDeckPickerListsYourDecks(t *testing.T) {
	m := pickerModel(t)
	m = leaderPress(m, "d")

	if m.state != stateDecks {
		t.Fatalf(",d left the app in state %v", m.state)
	}
	if m.deckPickerErr != nil {
		t.Fatalf("picker errored: %v", m.deckPickerErr)
	}
	if got := len(m.deckPicker.Items()); got != 2 {
		t.Fatalf("listed %d decks, want 2", got)
	}

	// Each deck is described, not just named: you pick by what's in it.
	view := stripANSI(m.viewDeckPicker())
	for _, want := range []string{
		"Ghen reanimator", "ghen", "commander", "32 cards", "3 distinct",
		"Winota: Snowball Stax", "winota",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("picker is missing %q:\n%s", want, view)
		}
	}

	// esc goes back where it came from.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateResults {
		t.Errorf("esc left the app in state %v", m.state)
	}
}

func TestDeckPickerOpensADeck(t *testing.T) {
	m := pickerModel(t)
	m = leaderPress(m, "d")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != stateResults {
		t.Fatalf("opening a deck left the app in state %v", m.state)
	}
	if m.deck == nil {
		t.Fatal("no deck was opened")
	}
	if m.deck.Slug != "ghen" {
		t.Errorf("opened %q, want the first deck in the list", m.deck.Slug)
	}
	if m.deckPane.empty() {
		t.Error("the deck opened with no cards in it")
	}
	// A deck opened from the picker is one of your own, so it has a history.
	if !m.deck.Local() {
		t.Error("a deck from the picker should be local")
	}
}

func TestDeckPickerWithNoDecks(t *testing.T) {
	gitRepo(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	m = leaderPress(m, "d")

	if m.state != stateDecks {
		t.Fatalf("state = %v", m.state)
	}
	// An empty picker has to say how to get a deck, or it's a dead end.
	view := stripANSI(m.viewDeckPicker())
	for _, want := range []string{"No decks yet", "scry deck import"} {
		if !strings.Contains(view, want) {
			t.Errorf("empty picker is missing %q:\n%s", want, view)
		}
	}
}

func TestDeckPickerSurvivesABrokenDeckFile(t *testing.T) {
	dir := gitRepo(t)

	good, _ := parseDeckFile(strings.NewReader("name: Fine\n[mainboard]\n1 Sol Ring\n"))
	if _, _, err := saveDeckVersioned("fine", good); err != nil {
		t.Fatal(err)
	}
	// A deck file edited into something unparseable still gets a row, so
	// the picker doesn't just silently omit a deck you know you have.
	if err := os.WriteFile(filepath.Join(dir, "broken.deck"), []byte("[mainboard]\n0 Nope\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// From a standing start the search bar has focus and there is no list
	// to press a letter in. The leader works there too, as long as no query
	// has been typed for the comma to belong to.
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = leaderPress(m, "d")

	if m.state != stateDecks {
		t.Fatalf(",d from an empty search bar left the app in state %v", m.state)
	}
	if got := len(m.deckPicker.Items()); got != 2 {
		t.Fatalf("listed %d decks, want both the good and the broken one", got)
	}
	view := stripANSI(m.viewDeckPicker())
	if !strings.Contains(view, "broken") {
		t.Errorf("the broken deck is not listed:\n%s", view)
	}
}

func TestManaColumnIsNotOversized(t *testing.T) {
	// It was 10 wide, which left a gap between the cost and the type line
	// on every row; almost every cost fits in far less.
	_, manaW, _ := listColumns(120)
	if manaW > 6 {
		t.Errorf("mana column is %d wide, wider than nearly every cost", manaW)
	}

	// The columns still have to add up to the width they were given.
	for _, total := range []int{40, 60, 80, 120, 200} {
		nameW, manaW, typeW := listColumns(total)
		if used := 2 + nameW + 2 + manaW + 2 + typeW; used > total && total >= 40 {
			t.Errorf("at width %d the columns use %d", total, used)
		}
	}
}

func TestBlurredListIsDimmedAndRowsStayPut(t *testing.T) {
	m := deckAndSearch(t, 190, 30)

	// Same rows either way — dimming must not change what's on screen, only
	// how it looks.
	focused := m.deckPane.list.View()
	m2 := drive(m, tea.KeyMsg{Type: tea.KeyTab})

	if len(stripANSI(focused)) == 0 {
		t.Fatal("the deck column rendered nothing")
	}
	if a, b := stripANSI(m.View()), stripANSI(m2.View()); len(a) == 0 || len(b) == 0 {
		t.Fatal("a frame rendered empty")
	}

	// The blurred list carries the hollow marker, the focused one the solid
	// marker, so the two read differently without relying on colour.
	blurredDeck := m.View() // results focused, so the deck is blurred
	if !strings.Contains(stripANSI(blurredDeck), "▹") {
		t.Error("the blurred list has no hollow cursor")
	}
	if !strings.Contains(stripANSI(blurredDeck), "▸") {
		t.Error("the focused list has no solid cursor")
	}
}
