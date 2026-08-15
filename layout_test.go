package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// deckAndSearch is the deckbuilding screen: a deck open and a search beside
// it.
func deckAndSearch(t *testing.T, w, h int) model {
	t.Helper()
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: w, Height: h})
	m = drive(m, deckLoadedMsg{
		info: deckInfo{
			name: "Ghen reanimator", slug: "ghen", format: "commander",
			total: 100, unique: 86,
		},
		cards: deckFixture(),
	})
	return drive(m, searchResultMsg{cards: testCards(), totalCards: 190})
}

func TestLayoutTiers(t *testing.T) {
	tests := []struct {
		w          int
		deckColumn bool
		vertical   bool
	}{
		{100, false, true},  // stacked: one list, panel underneath
		{140, false, false}, // two: one list beside the panel, tab swaps
		{160, true, false},  // three: results, deck, panel
		{200, true, false},
	}

	for _, tt := range tests {
		m := deckAndSearch(t, tt.w, 40)
		if got := m.deckColumn(); got != tt.deckColumn {
			t.Errorf("at %d columns deckColumn = %v, want %v", tt.w, got, tt.deckColumn)
		}
		if got := m.resultsLayout().vertical; got != tt.vertical {
			t.Errorf("at %d columns vertical = %v, want %v", tt.w, got, tt.vertical)
		}
	}
}

func TestLayoutRegionsFitTheWidth(t *testing.T) {
	for _, w := range []int{100, 140, 160, 190, 240} {
		m := deckAndSearch(t, w, 40)
		l := m.resultsLayout()

		if l.vertical {
			continue
		}
		// Each border between regions costs a column, and the regions plus
		// their borders have to add up to the terminal — a column over and
		// the rightmost one wraps onto the next line.
		total := l.listW + l.panelW + 1
		if l.deckW > 0 {
			total += l.deckW + 1
		}
		if total > w {
			t.Errorf("at %d columns the regions add up to %d: %+v", w, total, l)
		}
	}
}

func TestDeckColumnRowsDoNotWrap(t *testing.T) {
	// The deck column's frame costs it a border and a column of padding.
	// Sizing the list to the full column width made every row a line too
	// long, so each card wrapped onto two lines and the list ran off the
	// bottom of the screen.
	m := deckAndSearch(t, 190, 24)

	view := stripANSI(m.View())
	for _, name := range []string{"Ancient Tomb", "30x Mountain", "Test Walker"} {
		if !strings.Contains(view, name) {
			t.Errorf("deck column is missing %q:\n%s", name, view)
		}
	}

	// Every card in the fixture gets exactly one line in the column.
	l := m.resultsLayout()
	inner := l.deckW - deckChrome
	for i, line := range strings.Split(stripANSI(m.deckPane.list.View()), "\n") {
		if n := visibleLen(line); n > inner {
			t.Errorf("deck row %d is %d columns, the column holds %d: %q", i, n, inner, line)
		}
	}
}

func TestThreeColumnFrameFitsTerminal(t *testing.T) {
	data := loadTestRules(t)

	for _, size := range [][2]int{{160, 30}, {190, 24}, {240, 50}} {
		w, h := size[0], size[1]
		m := deckAndSearch(t, w, h)
		m.rules = data

		frameCheck(t, "three-column", m.View(), w, h)

		// And with each panel mode, from each list.
		for _, keys := range []string{"r", "s", "t", "\t", "\ts", "\tr"} {
			mk := m
			for _, r := range keys {
				if r == '\t' {
					mk = drive(mk, tea.KeyMsg{Type: tea.KeyTab})
					continue
				}
				mk = drive(mk, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			frameCheck(t, "three-column+"+keys, mk.View(), w, h)
		}
	}
}

func TestFocusFollowsTab(t *testing.T) {
	m := deckAndSearch(t, 190, 40)

	if m.focus != focusResults {
		t.Fatalf("focus = %v, want the results after a search", m.focus)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusDeck {
		t.Fatalf("tab did not reach the deck: %v", m.focus)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focus != focusResults {
		t.Fatalf("shift+tab did not come back: %v", m.focus)
	}

	// With no deck open there's nothing to tab to.
	solo := initialModel()
	solo = drive(solo, tea.WindowSizeMsg{Width: 190, Height: 40})
	solo = drive(solo, searchResultMsg{cards: testCards(), totalCards: 3})
	solo = drive(solo, tea.KeyMsg{Type: tea.KeyTab})
	if solo.focus != focusResults {
		t.Errorf("tab with no deck moved focus to %v", solo.focus)
	}
}

func TestPanelFollowsFocus(t *testing.T) {
	m := deckAndSearch(t, 190, 40)

	// The results are focused, so the panel shows the selected search result.
	first, ok := m.active().selected()
	if !ok {
		t.Fatal("nothing selected in the results")
	}
	if !strings.Contains(stripANSI(m.View()), first.card.Name) {
		t.Errorf("the panel is not showing %q", first.card.Name)
	}

	// Tabbing to the deck moves the panel onto the deck's selected card.
	m = drive(m, tea.KeyMsg{Type: tea.KeyTab})
	deckCard, ok := m.active().selected()
	if !ok {
		t.Fatal("nothing selected in the deck")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, deckCard.card.TypeLine) {
		t.Errorf("the panel did not follow focus to %q:\n%s", deckCard.card.Name, view)
	}
}

func TestDeckKeepsItsOwnStatisticsFilter(t *testing.T) {
	// Narrowing the deck by a category must not touch the search results
	// beside it, and vice versa — each list carries its own.
	m := deckAndSearch(t, 190, 40)
	resultsBefore := len(m.results.list.Items())

	m = drive(m, tea.KeyMsg{Type: tea.KeyTab}) // to the deck
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})

	if m.deckPane.statFilter == nil {
		t.Fatal("J did not narrow the deck to a category")
	}
	if m.results.statFilter != nil {
		t.Error("narrowing the deck also narrowed the search results")
	}
	if got := len(m.results.list.Items()); got != resultsBefore {
		t.Errorf("the results list changed from %d to %d cards", resultsBefore, got)
	}

	// The deck itself was narrowed.
	if len(m.deckPane.list.Items()) == len(m.deckPane.baseItems) {
		t.Error("the deck was not actually narrowed")
	}

	// esc puts it back, and only it.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.deckPane.statFilter != nil {
		t.Error("esc did not clear the deck's category")
	}
	if len(m.deckPane.list.Items()) != len(m.deckPane.baseItems) {
		t.Error("the deck was not restored")
	}
}

func TestDeckColumnAppearsOnlyWithADeck(t *testing.T) {
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 190, Height: 40})
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})

	if m.deckColumn() {
		t.Error("a deck column appeared with no deck open")
	}
	if l := m.resultsLayout(); l.deckW != 0 {
		t.Errorf("deckW = %d with no deck open", l.deckW)
	}
	// The two-region layout is unchanged: list and panel, nothing else.
	l := m.resultsLayout()
	if l.listW+l.panelW+1 != 190 {
		t.Errorf("without a deck the regions should still fill the width: %+v", l)
	}
}
