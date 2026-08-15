package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewDeck(t *testing.T) {
	gitRepo(t)

	slug, d, err := newDeck("Ghen reanimator", "")
	if err != nil {
		t.Fatal(err)
	}
	if slug != "ghen-reanimator" {
		t.Errorf("slug = %q", slug)
	}
	if d.Name != "Ghen reanimator" {
		t.Errorf("name = %q", d.Name)
	}
	if d.Format != defaultFormat {
		t.Errorf("format = %q, want %q by default", d.Format, defaultFormat)
	}
	if len(d.Entries) != 0 {
		t.Errorf("a new deck should be empty, got %d entries", len(d.Entries))
	}

	if _, _, err := saveDeckVersioned(slug, d); err != nil {
		t.Fatal(err)
	}
	// An empty deck still round-trips, and is still a file with a header.
	back, err := readDeck(slug)
	if err != nil {
		t.Fatalf("an empty deck must read back: %v", err)
	}
	if back.Name != d.Name || back.Format != d.Format {
		t.Errorf("header lost: %+v", back)
	}
}

func TestNewDeckRejectsNonsense(t *testing.T) {
	gitRepo(t)

	for _, name := range []string{"", "   ", "!!!"} {
		if _, _, err := newDeck(name, ""); err == nil {
			t.Errorf("newDeck(%q) should have failed", name)
		}
	}

	// And won't quietly write over one you already have.
	slug, d, err := newDeck("Ghen", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := saveDeckVersioned(slug, d); err != nil {
		t.Fatal(err)
	}
	if _, _, err := newDeck("Ghen", ""); err == nil {
		t.Error("newDeck overwrote an existing deck")
	}
}

func TestAnEmptyDeckOpens(t *testing.T) {
	gitRepo(t)
	t.Setenv("HOME", t.TempDir())

	slug, d, err := newDeck("Ghen", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := saveDeckVersioned(slug, d); err != nil {
		t.Fatal(err)
	}

	// A deck you've just created has nothing in it. It still has to open,
	// or there'd be no way to put anything in it.
	info, cards, err := openLocalDeck(slug)
	if err != nil {
		t.Fatalf("an empty deck must open: %v", err)
	}
	if len(cards) != 0 {
		t.Errorf("got %d cards from an empty deck", len(cards))
	}
	if info.name != "Ghen" || !info.local() {
		t.Errorf("info = %+v", info)
	}
}

func TestNewDeckFromThePicker(t *testing.T) {
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{})

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlO})

	// n opens the prompt.
	m = press(m, "n")
	if !m.naming {
		t.Fatal("n did not open the name prompt")
	}
	if !strings.Contains(stripANSI(m.viewDeckPicker()), "New deck") {
		t.Error("the prompt isn't on screen")
	}

	for _, r := range "Ghen reanimator" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.naming {
		t.Error("the prompt stayed open after creating the deck")
	}
	if !deckExists("ghen-reanimator") {
		t.Fatal("the deck was not created")
	}
	// And you land in it, rather than back at a list to find it in.
	if m.deck == nil || m.deck.slug != "ghen-reanimator" {
		t.Errorf("did not open the new deck: %+v", m.deck)
	}
	if m.state != stateResults {
		t.Errorf("state = %v", m.state)
	}
}

func TestPickerNameClashIsRecoverable(t *testing.T) {
	m := pickerModel(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = press(m, "n")
	// pickerModel already has a deck filed under "ghen", and this
	// slugifies to the same name.
	for _, r := range "Ghen!" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.deckPickerErr == nil {
		t.Fatal("a name clash should have been reported")
	}
	// The prompt has to stay visible, or there's nothing to correct in.
	if !m.naming {
		t.Error("the prompt closed on a name clash")
	}
	view := stripANSI(m.viewDeckPicker())
	if !strings.Contains(view, "already have") {
		t.Errorf("the error isn't on screen:\n%s", view)
	}
	if !strings.Contains(view, "New deck") {
		t.Errorf("the prompt isn't on screen with the error:\n%s", view)
	}

	// Typing again clears it, and esc backs out.
	m = press(m, "2")
	if m.deckPickerErr != nil {
		t.Error("the error outlived the correction")
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.naming {
		t.Error("esc did not close the prompt")
	}
}

// ── Commanders ──────────────────────────────────────────────────

func TestCommanderToggle(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Sol Ring")

	m = press(m, "c")
	i := m.deckIndexOf("Sol Ring")
	if !m.deckCards[i].commander {
		t.Fatal("c did not mark the card as a commander")
	}
	if !strings.Contains(m.notice, "commander") {
		t.Errorf("notice = %q", m.notice)
	}

	// And off again.
	m = press(m, "c")
	if m.deckCards[m.deckIndexOf("Sol Ring")].commander {
		t.Error("c did not unmark it")
	}
	if !strings.Contains(m.notice, "no longer") {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestCommanderFromASearchAddsIt(t *testing.T) {
	// The way a new deck starts: find your commander, press c.
	m := editableDeck(t)
	sel, _ := m.active().selected()

	m = press(m, "c")
	i := m.deckIndexOf(sel.card.Name)
	if i < 0 {
		t.Fatal("c did not add the card")
	}
	if !m.deckCards[i].commander {
		t.Error("the card was added but not as a commander")
	}
}

func TestSeveralCommandersAndNone(t *testing.T) {
	// scry doesn't know the rules and isn't going to pretend to: how many
	// commanders a deck has is between you and your playgroup.
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)

	for _, name := range []string{"Sol Ring", "Plains"} {
		selectCard(&m.deckPane, name)
		m = press(m, "c")
	}

	var commanders int
	for _, dc := range m.deckCards {
		if dc.commander {
			commanders++
		}
	}
	if commanders != 2 {
		t.Fatalf("got %d commanders, want 2 — several must be allowed", commanders)
	}

	// They survive a save, in the file's command zone.
	m = drive(m, deckSaveTickMsg{seq: m.deckSeq})
	on, err := readDeck("ghen")
	if err != nil {
		t.Fatal(err)
	}
	var inZone int
	for _, e := range on.Entries {
		if e.Section == "commander" {
			inZone++
		}
	}
	if inZone != 2 {
		t.Errorf("the file has %d cards in the command zone, want 2:\n%s", inZone, on.String())
	}

	// And none is fine too.
	for _, name := range []string{"Sol Ring", "Plains"} {
		selectCard(&m.deckPane, name)
		m = press(m, "c")
	}
	for _, dc := range m.deckCards {
		if dc.commander {
			t.Error("a deck with no commanders should be allowed")
		}
	}
}

func TestCommandersSortToTheTop(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Plains")
	m = press(m, "c")

	first, ok := m.deckPane.list.Items()[0].(cardItem)
	if !ok || first.card.Name != "Plains" {
		t.Errorf("the commander is not at the top of the deck list: %+v", first)
	}
}
