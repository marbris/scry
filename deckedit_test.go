package main

import (
	"github.com/charmbracelet/bubbles/list"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// editableDeck is a local deck open beside a search, ready to be edited.
func editableDeck(t *testing.T) model {
	t.Helper()
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{
		"sol ring": {Name: "Sol Ring", TypeLine: "Artifact"},
		"plains":   {Name: "Plains", TypeLine: "Basic Land — Plains"},
	})

	d, err := parseDeckFile(strings.NewReader("name: Ghen\nformat: commander\n\n[mainboard]\n1 Sol Ring [ramp]\n7 Plains\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := saveDeckVersioned("ghen", d); err != nil {
		t.Fatal(err)
	}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 190, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Ghen", slug: "ghen", format: "commander", total: 8, unique: 2},
		cards: []deckCard{{card: ScryfallCard{Name: "Sol Ring", TypeLine: "Artifact"}, qty: 1, tags: []string{"ramp"}}, {card: ScryfallCard{Name: "Plains", TypeLine: "Basic Land — Plains"}, qty: 7}},
	})
	return drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
}

// press applies a key without running the commands it returns. The delayed
// save is a tea.Tick, and the test driver runs commands inline — so driving
// an edit normally would block for the whole delay and then save, which is
// exactly the behaviour these tests are trying to observe the absence of.
func press(m model, k string) model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	return next.(model)
}

func pressKey(m model, t tea.KeyType) model {
	next, _ := m.Update(tea.KeyMsg{Type: t})
	return next.(model)
}

func TestAddCardFromSearch(t *testing.T) {
	m := editableDeck(t)

	// Focus is on the search results, so a adds whatever's selected there.
	before := len(m.deckCards)
	sel, _ := m.active().selected()
	m = press(m, "a")

	if len(m.deckCards) != before+1 {
		t.Fatalf("deck has %d cards, want %d", len(m.deckCards), before+1)
	}
	if m.deckIndexOf(sel.card.Name) < 0 {
		t.Errorf("%q is not in the deck", sel.card.Name)
	}
	if !strings.Contains(m.notice, sel.card.Name) {
		t.Errorf("notice = %q, should name the card added", m.notice)
	}
	// The deck column and its counts follow immediately.
	if m.deck.unique != before+1 {
		t.Errorf("unique = %d, want %d", m.deck.unique, before+1)
	}
	if len(m.deckPane.list.Items()) != before+1 {
		t.Errorf("the deck column shows %d cards", len(m.deckPane.list.Items()))
	}
	if !m.deckDirty {
		t.Error("the edit did not mark the deck unsaved")
	}
}

func TestAddingTwiceNeedsAsking(t *testing.T) {
	// Nearly every deck this is built for is singleton, so a second copy
	// has to be deliberate rather than a repeated keypress.
	m := editableDeck(t)
	m = press(m, "a")
	sel, _ := m.active().selected()
	count := len(m.deckCards)

	m = press(m, "a")
	if len(m.deckCards) != count {
		t.Errorf("a second a added another copy of %s", sel.card.Name)
	}
	if !strings.Contains(m.notice, "already in the deck") {
		t.Errorf("notice = %q", m.notice)
	}

	// + is how you ask.
	m = press(m, "+")
	i := m.deckIndexOf(sel.card.Name)
	if i < 0 || m.deckCards[i].qty != 2 {
		t.Errorf("+ did not add a second copy: %+v", m.deckCards[i])
	}
}

func TestQuantityAndRemoval(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab) // into the deck
	selectCard(&m.deckPane, "Plains")

	m = press(m, "-")
	if i := m.deckIndexOf("Plains"); m.deckCards[i].qty != 6 {
		t.Errorf("- left %d Plains, want 6", m.deckCards[i].qty)
	}
	m = press(m, "+")
	if i := m.deckIndexOf("Plains"); m.deckCards[i].qty != 7 {
		t.Errorf("+ left %d Plains, want 7", m.deckCards[i].qty)
	}

	// x takes the card out however many copies there are.
	m = press(m, "x")
	if m.deckIndexOf("Plains") >= 0 {
		t.Error("x did not remove Plains")
	}
	if m.deck.total != 1 {
		t.Errorf("total = %d, want 1 after removing seven Plains", m.deck.total)
	}
}

func TestDecrementingToZeroRemoves(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Sol Ring")

	m = press(m, "-") // one copy, so this takes it out
	if m.deckIndexOf("Sol Ring") >= 0 {
		t.Error("- on a single copy should have removed the card")
	}
}

func TestRemoveFromTheSearchSide(t *testing.T) {
	// A card you've just found can be pulled out of the deck without
	// hunting for it in the deck column.
	m := editableDeck(t)
	m = press(m, "a")
	sel, _ := m.active().selected()

	if m.deckIndexOf(sel.card.Name) < 0 {
		t.Fatal("the card was not added")
	}
	m = press(m, "x")
	if m.deckIndexOf(sel.card.Name) >= 0 {
		t.Error("x from the results did not remove the card from the deck")
	}
}

func TestEditingKeepsTheCursorPut(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Plains")

	m = press(m, "+")
	sel, ok := m.deckPane.selected()
	if !ok || sel.card.Name != "Plains" {
		t.Errorf("the cursor moved off the card being edited: %+v", sel)
	}
}

func TestEditsAreNotWrittenImmediately(t *testing.T) {
	m := editableDeck(t)
	m = press(m, "a")

	// The file is untouched until the delay is up — that's what collects a
	// burst of edits into one commit.
	on, err := readDeck("ghen")
	if err != nil {
		t.Fatal(err)
	}
	if len(on.Entries) != 2 {
		t.Errorf("the deck file changed before the save was due: %d entries", len(on.Entries))
	}
	if !m.deckDirty {
		t.Error("the deck should be marked unsaved")
	}
}

func TestABurstOfEditsIsOneCommit(t *testing.T) {
	m := editableDeck(t)
	before, err := deckHistory("ghen", 10)
	if err != nil {
		t.Fatal(err)
	}

	// Three edits in a row, each restarting the clock.
	m = press(m, "a")
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Plains")
	m = press(m, "+")
	m = press(m, "+")

	// The ticks the first two edits scheduled are stale and must do nothing.
	for seq := 1; seq < m.deckSeq; seq++ {
		m = drive(m, deckSaveTickMsg{seq: seq})
	}
	if !m.deckDirty {
		t.Fatal("a stale tick wrote the deck early")
	}

	// Only the newest one saves.
	m = drive(m, deckSaveTickMsg{seq: m.deckSeq})
	if m.deckDirty {
		t.Fatal("the deck was not saved")
	}

	after, err := deckHistory("ghen", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("three edits made %d commits, want 1", len(after)-len(before))
	}

	// And the file has all three in it.
	on, err := readDeck("ghen")
	if err != nil {
		t.Fatal(err)
	}
	if len(on.Entries) != 3 {
		t.Errorf("saved %d entries, want 3", len(on.Entries))
	}
	for _, e := range on.Entries {
		if e.Name == "Plains" && e.Qty != 9 {
			t.Errorf("Plains saved as %d, want 9", e.Qty)
		}
	}
}

func TestQuittingWritesAPendingEdit(t *testing.T) {
	// Bubbletea runs no commands after tea.Quit, so an edit made a moment
	// before quitting would be lost if the flush weren't inline.
	m := editableDeck(t)
	m = press(m, "a")
	if !m.deckDirty {
		t.Fatal("the edit did not mark the deck unsaved")
	}
	added, _ := m.active().selected()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !quits(cmd) {
		t.Fatal("ctrl+c did not quit")
	}
	if next.(model).deckDirty {
		t.Error("the deck is still marked unsaved after quitting")
	}

	on, err := readDeck("ghen")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range on.Entries {
		if e.Name == added.card.Name {
			found = true
		}
	}
	if !found {
		t.Errorf("quitting lost the edit — %q is not in the saved deck:\n%s",
			added.card.Name, on.String())
	}
}

func TestEditingKeepsTagsAndSections(t *testing.T) {
	m := editableDeck(t)
	m = press(m, "a")
	m = drive(m, deckSaveTickMsg{seq: m.deckSeq})

	on, err := readDeck("ghen")
	if err != nil {
		t.Fatal(err)
	}
	var solRing *deckEntry
	for i := range on.Entries {
		if on.Entries[i].Name == "Sol Ring" {
			solRing = &on.Entries[i]
		}
	}
	if solRing == nil {
		t.Fatal("Sol Ring vanished")
	}
	if len(solRing.Tags) != 1 || solRing.Tags[0] != "ramp" {
		t.Errorf("editing lost a tag: %v", solRing.Tags)
	}
	if on.Name != "Ghen" || on.Format != "commander" {
		t.Errorf("editing lost the header: %+v", on)
	}
}

func TestAMoxfieldDeckIsReadOnly(t *testing.T) {
	gitRepo(t)
	t.Setenv("HOME", t.TempDir())

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 190, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Someone else's", id: "Y8dZ7", url: "https://moxfield.com/decks/Y8dZ7"},
		cards: deckFixture(),
	})
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})

	before := len(m.deckCards)
	m = press(m, "a")
	if len(m.deckCards) != before {
		t.Error("a browsed Moxfield deck was edited")
	}
	if !strings.Contains(m.notice, ",i to make it yours") {
		t.Errorf("notice = %q, should say how to make it editable", m.notice)
	}
	if m.deckDirty {
		t.Error("a deck that can't be saved was marked unsaved")
	}
}

func TestEditingWithNoDeckOpen(t *testing.T) {
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 190, Height: 40})
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})

	m = press(m, "a")
	if !strings.Contains(m.notice, "no deck open") {
		t.Errorf("notice = %q", m.notice)
	}
	if m.deckDirty {
		t.Error("marked dirty with no deck open")
	}
}

func TestEditingAFilteredDeckKeepsTheFilter(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)

	m = press(m, "/")
	m = press(m, "Plains")
	m = pressKey(m, tea.KeyEnter)
	if m.deckPane.list.FilterState() == list.Unfiltered {
		t.Fatal("the filter never applied")
	}

	m = press(m, "+")
	// Rebuilding the list on every edit would drop the filter and make
	// editing a filtered deck unusable.
	if m.deckPane.list.FilterValue() != "Plains" {
		t.Errorf("editing dropped the typed filter: %q", m.deckPane.list.FilterValue())
	}
}
