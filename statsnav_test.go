package main

import (
	"github.com/charmbracelet/bubbles/list"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLandsAreOutOfTheCurveAndTheColours(t *testing.T) {
	// A deck is mostly lands, and nearly all of them cost nothing and count
	// as colourless. Left in, they put a column at zero taller than the rest
	// of the curve put together and a "Colorless" bar that reads as if the
	// deck were full of colourless spells. The type breakdown already says
	// how many lands there are, and says it better.
	items := []list.Item{
		cardItem{card: ScryfallCard{Name: "Command Tower", TypeLine: "Land", CMC: 0}, qty: 1},
		cardItem{card: ScryfallCard{Name: "Plains", TypeLine: "Basic Land — Plains", CMC: 0}, qty: 30},
		cardItem{card: ScryfallCard{Name: "Sol Ring", TypeLine: "Artifact", CMC: 1}, qty: 1},
		cardItem{card: ScryfallCard{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, qty: 1},
	}
	entries := toEntries(items)

	count := func(rows []statRow, label string) int {
		for _, r := range rows {
			if r.label != label {
				continue
			}
			var n int
			for _, ci := range entries {
				if r.match(ci) {
					n += ci.qty
				}
			}
			return n
		}
		t.Fatalf("no %q row", label)
		return 0
	}

	if got := count(cmcRows(), "0"); got != 0 {
		t.Errorf("%d cards at mana value 0, want none — lands should be out of it", got)
	}
	if got := count(cmcRows(), "1"); got != 1 {
		t.Errorf("mana value 1 = %d, want the Sol Ring", got)
	}
	if got := count(colorRows(), "Colorless"); got != 1 {
		t.Errorf("Colorless = %d, want just the Sol Ring", got)
	}
	if got := count(colorRows(), "White"); got != 1 {
		t.Errorf("White = %d", got)
	}
}

func TestStatGroupsSayTheyExcludeLands(t *testing.T) {
	// The reader has to be told, or the numbers look wrong.
	entries := toEntries([]list.Item{
		cardItem{card: ScryfallCard{Name: "Plains", TypeLine: "Basic Land — Plains"}, qty: 30},
		cardItem{card: ScryfallCard{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, qty: 1},
	})

	var titles []string
	for _, g := range statGroups(entries, entries) {
		titles = append(titles, g.title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Mana Value (excl. lands)", "Color (excl. lands)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no group titled %q; titles are %s", want, joined)
		}
	}
}

func TestPlainKeysWalkTheStatisticsWhileItIsUp(t *testing.T) {
	// The statistics are what you're reading while they're up, and moving
	// the cursor through cards whose details are hidden behind them does
	// nothing — so j/k drive the categories too, not just J/K.
	m := editableDeck(t)
	m = pressKey(m, tea.KeyTab)
	m = press(m, "s")

	if m.deckPane.statIndex != -1 {
		t.Fatalf("statIndex = %d before anything was pressed", m.deckPane.statIndex)
	}
	m = press(m, "j")
	if m.deckPane.statIndex != 0 || m.deckPane.statFilter == nil {
		t.Fatalf("j did not enter the categories: index %d", m.deckPane.statIndex)
	}
	m = press(m, "j")
	if m.deckPane.statIndex != 1 {
		t.Errorf("a second j left the index at %d", m.deckPane.statIndex)
	}
	m = press(m, "k")
	if m.deckPane.statIndex != 0 {
		t.Errorf("k left the index at %d", m.deckPane.statIndex)
	}

	// The arrows do the same thing.
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.deckPane.statIndex != 1 {
		t.Errorf("down left the index at %d", m.deckPane.statIndex)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.deckPane.statIndex != 0 {
		t.Errorf("up left the index at %d", m.deckPane.statIndex)
	}
}

func TestPlainKeysMoveTheListWithTheCardPanelUp(t *testing.T) {
	// And with the card panel up they move the list, as they always have —
	// there the panel is describing whichever card the cursor is on.
	m := deckAndSearch(t, 190, 40)
	if m.panel != panelCard {
		t.Fatalf("panel = %v", m.panel)
	}
	before := m.results.list.Index()
	m = press(m, "j")
	if m.results.list.Index() == before {
		t.Error("j did not move the list while the card panel was up")
	}
}

func TestBothListsFlagWhatTheOtherHolds(t *testing.T) {
	m := editableDeck(t)
	m = drive(m, searchResultMsg{cards: []ScryfallCard{
		{Name: "Sol Ring", TypeLine: "Artifact"},    // the deck runs this
		{Name: "Black Lotus", TypeLine: "Artifact"}, // it doesn't
	}, totalCards: 2})

	inDeck := m.deckMembership()
	if !inDeck["sol ring"] || inDeck["black lotus"] {
		t.Errorf("deck membership = %v", inDeck)
	}

	// And the other way round, which is the half that was missing: a deck
	// card the search just turned up.
	inResults := m.resultMembership()
	if !inResults["sol ring"] {
		t.Error("the deck's Sol Ring is not flagged as one the search found")
	}
	if inResults["plains"] {
		t.Error("a deck card the search didn't find is flagged as though it had")
	}

	// Both columns show it.
	view := stripANSI(m.View())
	if n := strings.Count(view, "▪"); n < 2 {
		t.Errorf("only %d rows are flagged; both lists should mark Sol Ring:\n%s", n, view)
	}
}

func TestCommanderFlagWinsTheSlot(t *testing.T) {
	// A deck card can be a commander and be in the search results at once.
	// There's one slot, and the commander is the more important of the two.
	d := compactDelegate{inOther: map[string]bool{"ghen": true}}
	both := cardItem{card: ScryfallCard{Name: "Ghen"}, commander: true}
	if got := stripANSI(d.gutter(both)); !strings.Contains(got, "★") {
		t.Errorf("gutter = %q, want the commander flag", got)
	}
}
