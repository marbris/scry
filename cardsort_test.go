package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func sortFixture() []list.Item {
	return []list.Item{
		cardItem{Card: ScryfallCard{
			Name: "Sol Ring", TypeLine: "Artifact", CMC: 1, EDHRECRank: 1,
		}},
		cardItem{Card: ScryfallCard{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5,
			Colors: []string{"W"}, EDHRECRank: 4000,
		}},
		cardItem{Card: ScryfallCard{
			Name: "Command Tower", TypeLine: "Land", CMC: 0, EDHRECRank: 2,
		}},
		cardItem{Card: ScryfallCard{
			Name: "Anguished Unmaking", TypeLine: "Instant", CMC: 3,
			Colors: []string{"W", "B"}, EDHRECRank: 300,
		}},
		cardItem{Card: ScryfallCard{
			Name: "Brainstorm", TypeLine: "Instant", CMC: 1,
			Colors: []string{"U"},
		}},
	}
}

func names(items []list.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.(cardItem).Card.Name
	}
	return out
}

func TestSortOrders(t *testing.T) {
	tests := []struct {
		sort cardSort
		want []string
	}{
		{sortNone, []string{
			"Sol Ring", "Serra Angel", "Command Tower", "Anguished Unmaking", "Brainstorm",
		}},
		{sortName, []string{
			"Anguished Unmaking", "Brainstorm", "Command Tower", "Serra Angel", "Sol Ring",
		}},
		{sortMana, []string{
			"Command Tower", "Brainstorm", "Sol Ring", "Anguished Unmaking", "Serra Angel",
		}},
		{sortType, []string{
			// Creature, Land, Instant, Artifact — the decklist precedence.
			"Serra Angel", "Command Tower", "Anguished Unmaking", "Brainstorm", "Sol Ring",
		}},
		{sortColor, []string{
			// W, U, then multicolour, then colourless.
			"Serra Angel", "Brainstorm", "Anguished Unmaking", "Command Tower", "Sol Ring",
		}},
		{sortEDHREC, []string{
			// Unranked last rather than first, which a plain numeric sort
			// would do with rank zero.
			"Sol Ring", "Command Tower", "Anguished Unmaking", "Serra Angel", "Brainstorm",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.sort.name("arrival"), func(t *testing.T) {
			got := names(sortItems(sortFixture(), tt.sort))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("\n got %v\nwant %v", got, tt.want)
			}
		})
	}
}

func TestSortIsStableAndNonDestructive(t *testing.T) {
	items := sortFixture()
	before := names(items)

	sorted := sortItems(items, sortMana)
	if after := names(items); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("sorting reordered the input: %v", after)
	}
	if len(sorted) != len(items) {
		t.Errorf("sorting dropped items: %d of %d", len(sorted), len(items))
	}

	// Same input, same output, every time.
	for i := 0; i < 10; i++ {
		if got := names(sortItems(sortFixture(), sortColor)); strings.Join(got, ",") !=
			strings.Join(names(sortItems(sortFixture(), sortColor)), ",") {
			t.Fatal("sorting is not deterministic")
		}
	}
}

func TestCommandersSortWithEverythingElse(t *testing.T) {
	// A commander is a card. It comes first in decklist order because
	// that's how the deck list is built, not because sorting pins it there
	// — sorting by mana value means by mana value.
	items := []list.Item{
		cardItem{Card: ScryfallCard{Name: "Ghen", CMC: 3}, Commander: true},
		cardItem{Card: ScryfallCard{Name: "Sol Ring", CMC: 1}},
	}
	if got := names(sortItems(items, sortMana)); got[0] != "Sol Ring" {
		t.Errorf("the commander was pinned above a cheaper Card: %v", got)
	}
	if got := names(sortItems(items, sortName)); got[0] != "Ghen" {
		t.Errorf("by name Ghen should come first: %v", got)
	}
}

func TestSortCycles(t *testing.T) {
	s := sortNone
	seen := map[cardSort]bool{}
	for range cardSorts {
		seen[s] = true
		s = s.next(1)
	}
	if len(seen) != len(cardSorts) {
		t.Errorf("cycling reached %d of %d sorts", len(seen), len(cardSorts))
	}
	if s != sortNone {
		t.Error("cycling forward did not wrap back to the start")
	}
	// And backwards.
	if sortNone.next(-1) != cardSorts[len(cardSorts)-1] {
		t.Error("cycling backwards from the start did not wrap to the end")
	}
}

// ── In the app ──────────────────────────────────────────────────

func TestSortKeyReordersTheFocusedList(t *testing.T) {
	m := deckAndSearch(t, 190, 40)

	// Focus is on the results, so o sorts those and leaves the deck alone.
	deckBefore := names(m.deckPane.list.Items())
	m = press(m, "o")

	if m.results.sort == sortNone {
		t.Fatal("o did not change the results' sort")
	}
	if m.deckPane.sort != sortNone {
		t.Error("o also sorted the deck")
	}
	if got := names(m.deckPane.list.Items()); strings.Join(got, ",") != strings.Join(deckBefore, ",") {
		t.Error("the deck was reordered by sorting the results")
	}
	// The header says which order it's in, for as long as it's in it —
	// there's no transient message saying the same thing.
	if !strings.Contains(stripANSI(m.resultsHeader()), m.results.sortName()) {
		t.Error("the header doesn't name the new sort")
	}

	// And the other way round.
	m = pressKey(m, tea.KeyTab)
	m = press(m, "o")
	if m.deckPane.sort == sortNone {
		t.Fatal("o did not change the deck's sort")
	}
	// Each pane keeps its own, so the two can be read different ways at once.
	if m.results.sort == sortNone {
		t.Error("sorting the deck reset the results' sort")
	}
}

func TestSortSurvivesANewSearch(t *testing.T) {
	// The sort is a way of reading a list, not a property of one particular
	// set of results.
	m := deckAndSearch(t, 190, 40)
	m = press(m, "o")
	want := m.results.sort

	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	if m.results.sort != want {
		t.Errorf("a new search reset the sort to %v", m.results.sort)
	}
}

func TestSortKeepsTheCursorOnItsCard(t *testing.T) {
	m := deckAndSearch(t, 190, 40)
	m = pressKey(m, tea.KeyTab)
	selectCard(&m.deckPane, "Ancient Tomb")

	m = press(m, "o")
	sel, ok := m.deckPane.selected()
	if !ok || sel.Card.Name != "Ancient Tomb" {
		t.Errorf("sorting moved the cursor off the Card: %+v", sel)
	}
}

func TestSortShowsInTheHeader(t *testing.T) {
	m := deckAndSearch(t, 190, 40)

	// Nothing to say while the list is in the order it arrived in.
	if strings.Contains(stripANSI(m.resultsHeader()), "↕") {
		t.Error("the header names a sort when none is applied")
	}

	m = press(m, "o")
	header := stripANSI(m.resultsHeader())
	if !strings.Contains(header, m.results.sortName()) {
		t.Errorf("the header doesn't name the sort:\n%s", header)
	}
}

func TestSortComposesWithTheStatisticsFilter(t *testing.T) {
	// Narrowing to a category and sorting are independent, and applying one
	// must not drop the other.
	m := deckAndSearch(t, 190, 40)
	m = pressKey(m, tea.KeyTab)
	m = press(m, "o") // by name

	m = press(m, "s")
	m = press(m, "J") // narrow to the first category
	if m.deckPane.statFilter == nil {
		t.Fatal("the category filter did not apply")
	}
	if m.deckPane.sort == sortNone {
		t.Error("narrowing dropped the sort")
	}

	// What's on screen is both: the category's cards, in sorted order.
	got := names(m.deckPane.list.Items())
	if len(got) == 0 {
		t.Fatal("nothing left after narrowing")
	}
	sorted := names(sortItems(m.deckPane.list.Items(), m.deckPane.sort))
	if strings.Join(got, ",") != strings.Join(sorted, ",") {
		t.Errorf("the narrowed list isn't sorted:\n got %v\nwant %v", got, sorted)
	}
}
