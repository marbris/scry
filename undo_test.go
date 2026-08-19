package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUndoAnAdd(t *testing.T) {
	m := editableDeck(t)
	before := len(m.deckCards)
	sel, _ := m.active().selected()

	m = press(m, "a")
	if len(m.deckCards) != before+1 {
		t.Fatal("the card was not added")
	}

	m = press(m, "u")
	if len(m.deckCards) != before {
		t.Errorf("undo left %d cards, want %d", len(m.deckCards), before)
	}
	if m.deckIndexOf(sel.Card.Name) >= 0 {
		t.Error("the added card is still in the deck")
	}
	if !strings.Contains(m.notice, "undo") {
		t.Errorf("notice = %q", m.notice)
	}
	// The undone state still has to be written — undo is an edit like any
	// other, not a way to leave the file disagreeing with the screen.
	if !m.deckDirty {
		t.Error("undo did not mark the deck for saving")
	}
}

func TestUndoWalksBack(t *testing.T) {
	m := editableDeck(t)
	start := len(m.deckCards)

	m = press(m, "a")
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(m, "a")
	if len(m.deckCards) != start+2 {
		t.Fatalf("expected two additions, got %d cards", len(m.deckCards))
	}

	// Each u goes back one step rather than toggling between two states.
	m = press(m, "u")
	if len(m.deckCards) != start+1 {
		t.Fatalf("first undo left %d cards", len(m.deckCards))
	}
	m = press(m, "u")
	if len(m.deckCards) != start {
		t.Fatalf("second undo left %d cards", len(m.deckCards))
	}

	m = press(m, "u")
	if !strings.Contains(m.notice, "nothing to undo") {
		t.Errorf("notice at the end of the stack = %q", m.notice)
	}
}

func TestUndoRemovalAndQuantity(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)

	selectCard(&m.deckPane, "Plains")
	m = press(m, "+")
	if m.deckCards[m.deckIndexOf("Plains")].Qty != 8 {
		t.Fatal("+ did not add a copy")
	}
	m = press(m, "u")
	if got := m.deckCards[m.deckIndexOf("Plains")].Qty; got != 7 {
		t.Errorf("undo left %d Plains, want 7", got)
	}

	selectCard(&m.deckPane, "Sol Ring")
	m = press(m, "x")
	if m.deckIndexOf("Sol Ring") >= 0 {
		t.Fatal("x did not remove it")
	}
	m = press(m, "u")
	i := m.deckIndexOf("Sol Ring")
	if i < 0 {
		t.Fatal("undo did not bring the card back")
	}
	// And it comes back whole — quantity, tags and all.
	if len(m.deckCards[i].Tags) != 1 || m.deckCards[i].Tags[0] != "ramp" {
		t.Errorf("the restored card lost its Tags: %v", m.deckCards[i].Tags)
	}
}

func TestUndoACommanderToggle(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Sol Ring")

	m = press(m, "c")
	if !m.deckCards[m.deckIndexOf("Sol Ring")].Commander {
		t.Fatal("c did not mark it")
	}
	m = press(m, "u")
	if m.deckCards[m.deckIndexOf("Sol Ring")].Commander {
		t.Error("undo did not unmark it")
	}
}

func TestUndoABulkTagIsOneStep(t *testing.T) {
	// Tagging five cards is one action, so one u puts all five back — not
	// five presses to walk out of one.
	m := taggableDeck(t)
	m = press(m, "v")
	m = press(m, "T")
	for _, r := range "reviewed" {
		m = press(m, string(r))
	}
	m = press(m, "\r")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	var tagged int
	for _, dc := range m.deckCards {
		for _, tag := range dc.Tags {
			if tag == "reviewed" {
				tagged++
			}
		}
	}
	if tagged != 5 {
		t.Fatalf("%d cards were tagged, want 5", tagged)
	}

	m = press(m, "u")
	for _, dc := range m.deckCards {
		for _, tag := range dc.Tags {
			if tag == "reviewed" {
				t.Errorf("%s is still tagged after one undo", dc.Card.Name)
			}
		}
	}
	// Sol Ring's own tag was there before and must survive the undo.
	if tags := tagsOf(m, "Sol Ring"); len(tags) != 1 || tags[0] != "ramp" {
		t.Errorf("undo clobbered an existing tag: %v", tags)
	}
}

func TestUndoOfNothingIsNotAnEdit(t *testing.T) {
	// A tag that changes nothing shouldn't leave a step on the stack for u
	// to walk back through.
	m := taggableDeck(t)
	selectCard(&m.deckPane, "Sol Ring")
	m = press(m, "T")
	for _, r := range "ramp" { // it already has this
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.undo) != 0 {
		t.Errorf("a tag that changed nothing left %d undo steps", len(m.undo))
	}
	m = press(m, "u")
	if !strings.Contains(m.notice, "nothing to undo") {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestUndoNeedsAnEditableDeck(t *testing.T) {
	gitRepo(t)
	t.Setenv("HOME", t.TempDir())

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 190, Height: 40})
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	m = press(m, "u")
	if !strings.Contains(m.notice, "no deck open") {
		t.Errorf("notice = %q", m.notice)
	}
}

// ── In-deck and commander flags ─────────────────────────────────

func TestSearchResultsShowWhatTheDeckHolds(t *testing.T) {
	m := editableDeck(t)

	// Add the selected search result, and it gains the in-deck flag.
	sel, _ := m.active().selected()
	m = press(m, "a")

	inDeck := m.deckMembership()
	if !inDeck[strings.ToLower(sel.Card.Name)] {
		t.Fatalf("%q is not counted as in the deck", sel.Card.Name)
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, "▪") {
		t.Errorf("no in-deck flag on screen:\n%s", view)
	}
}

func TestCommandersAreFlaggedInTheDeck(t *testing.T) {
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Sol Ring")
	m = press(m, "c")

	if i := m.deckIndexOf("Sol Ring"); i < 0 || !m.deckCards[i].Commander {
		t.Fatal("the commander was not recorded")
	}
	if !strings.Contains(stripANSI(m.View()), "★") {
		t.Error("no commander flag on screen")
	}
}

func TestGutterKeepsRowsAlignedWhateverTheFlags(t *testing.T) {
	// Both gutter columns are always drawn, so a row doesn't shift sideways
	// when a card joins the deck or becomes a commander.
	card := ScryfallCard{Name: "Serra Angel", TypeLine: "Creature — Angel", ManaCost: "{3}{W}{W}"}
	l := list.New([]list.Item{cardItem{Card: card}}, compactDelegate{}, 60, 4)
	key := map[string]bool{"serra angel": true}

	widths := map[string]int{}
	for name, tc := range map[string]struct {
		d  compactDelegate
		it cardItem
	}{
		"plain":     {compactDelegate{}, cardItem{Card: card}},
		"marked":    {compactDelegate{marks: key}, cardItem{Card: card}},
		"in other":  {compactDelegate{inOther: key}, cardItem{Card: card}},
		"commander": {compactDelegate{}, cardItem{Card: card, Commander: true}},
		"both":      {compactDelegate{marks: key}, cardItem{Card: card, Commander: true}},
	} {
		var b strings.Builder
		tc.d.Render(&b, l, 0, tc.it)
		widths[name] = visibleLen(b.String())
	}

	first := widths["plain"]
	for name, w := range widths {
		if w != first {
			t.Errorf("%s rows are %d columns, plain rows are %d", name, w, first)
		}
	}
}
