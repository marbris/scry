package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"

	tea "github.com/charmbracelet/bubbletea"
)

// drive runs a sequence of keys through the model, the way a person would.
func drive(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.Msg
		switch k {
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// openPanel opens a panel and fills it, which is the only way to get back
// out to the list: an empty panel has nothing to escape to, so esc closes
// it. Running a search is what turns the bar back into a header.
func openPanel(m Model, key, query string) Model {
	m = drive(m, "space", key)
	// A decks panel opens straight onto your decks, so there is no bar to
	// type into and nothing to ask for.
	if !m.ws.current().searchOpen {
		return m
	}
	for _, r := range query {
		m = drive(m, string(r))
	}
	return drive(m, "enter")
}

func sized(w, h int) Model {
	m := New()
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func TestStartsOnTheSplashWithNoPanels(t *testing.T) {
	m := sized(120, 40)
	if !m.ws.empty() {
		t.Error("something was open before anything was asked for")
	}
	if view := m.View(); view == "" {
		t.Error("the splash drew nothing")
	}
}

func TestTheLeaderOpensEachKindOfPanel(t *testing.T) {
	for key, want := range map[string]Kind{
		"f": KindFind,
		"d": KindDecks,
		"r": KindRules,
		"n": KindNew,
	} {
		m := drive(sized(120, 40), "space", key)
		if m.ws.count() != 1 {
			t.Fatalf("space %s opened %d panels", key, m.ws.count())
		}
		if got := m.ws.current().kind; got != want {
			t.Errorf("space %s opened a %v panel, want %v", key, got, want)
		}
	}
}

func TestANewPanelOpensBesideTheOneYouWereOn(t *testing.T) {
	// Not at the end: you opened it while working on this panel, so it
	// belongs next to this panel.
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")

	kinds := []Kind{}
	for _, p := range m.ws.panels {
		kinds = append(kinds, p.kind)
	}
	if len(kinds) != 3 || kinds[1] != KindDecks || kinds[2] != KindRules {
		t.Errorf("panels came out %v", kinds)
	}
}

func TestFocusStepsAlongTheRowAndStopsAtTheEnds(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")
	if m.ws.focused != 2 {
		t.Fatalf("focus is on %d, want the panel just opened", m.ws.focused)
	}

	m = drive(m, "h", "h")
	if m.ws.focused != 0 {
		t.Errorf("two lefts landed on %d, want 0", m.ws.focused)
	}
	m = drive(m, "h")
	if m.ws.focused != 0 {
		t.Errorf("left from the leftmost panel wrapped to %d", m.ws.focused)
	}
	m = drive(m, "l", "l", "l")
	if m.ws.focused != 2 {
		t.Errorf("right ran past the end to %d", m.ws.focused)
	}
}

func TestClosingAPanelFocusesItsLeftNeighbour(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")
	m = drive(m, "space", "c")
	if m.ws.count() != 2 {
		t.Fatalf("closed to %d panels, want 2", m.ws.count())
	}
	if m.ws.focused != 1 {
		t.Errorf("focus went to %d, want the panel to the left", m.ws.focused)
	}
}

func TestOnlyKeepsTheFocusedPanel(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")
	m = drive(m, "h") // onto the decks panel
	m = drive(m, "space", "o")

	if m.ws.count() != 1 {
		t.Fatalf("only left %d panels", m.ws.count())
	}
	if m.ws.current().kind != KindDecks {
		t.Errorf("only kept the %v panel, want the focused one", m.ws.current().kind)
	}
}

func TestAPanelCanBeMovedAlongTheRow(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = drive(m, "space", "h")

	if m.ws.panels[0].kind != KindDecks {
		t.Errorf("panels are %v, %v; want decks moved to the left",
			m.ws.panels[0].kind, m.ws.panels[1].kind)
	}
	if m.ws.focused != 0 {
		t.Errorf("focus stayed at %d rather than travelling with the panel", m.ws.focused)
	}
}

func TestTabCyclesWhatANewPanelSearches(t *testing.T) {
	m := drive(sized(120, 40), "space", "n")
	if m.ws.current().kind != KindNew {
		t.Fatal("space n did not open an untyped panel")
	}
	m = drive(m, "tab")
	if got := m.ws.current().kind; got != KindFind {
		t.Errorf("first tab landed on %v, want find", got)
	}
	m = drive(m, "tab", "tab")
	if got := m.ws.current().kind; got != KindRules {
		t.Errorf("three tabs landed on %v, want rules", got)
	}
}

func TestTheSearchBarTakesTypingRatherThanCommands(t *testing.T) {
	// h, l and q are panel keys in a list and letters in a search bar.
	m := drive(sized(120, 40), "space", "f")
	m = drive(m, "h", "l", "q")
	if got := m.ws.current().search.Value(); got != "hlq" {
		t.Errorf("the bar holds %q, want the keys typed into it", got)
	}
	if m.ws.count() != 1 {
		t.Error("typing in the search bar changed the panels")
	}
}

func TestEscClosesAFilteredPanelRatherThanClearingIt(t *testing.T) {
	// esc is the way out now, not a filter-clearer: a filtered panel with
	// nothing transient in it closes on the first esc, and b is what would
	// have kept it open by clearing the filter instead.
	m := withCards(sized(120, 40), "f", sample(), sortArrival)
	m = drive(m, "/", "e", "l", "f", "enter")

	m = drive(m, "esc")
	if m.ws.count() != 0 {
		t.Error("esc did not close the panel; it should no longer stop to clear a filter")
	}
}

func TestTheLeaderMenuAppearsAndCancels(t *testing.T) {
	m := openPanel(sized(120, 40), "f", "angel")
	m = drive(m, "space")
	if !m.leader {
		t.Fatal("space did not raise the menu")
	}
	if view := m.View(); view == "" {
		t.Error("the menu drew nothing")
	}
	m = drive(m, "z") // names nothing
	if m.leader {
		t.Error("an unknown key left the leader waiting")
	}
	if m.ws.count() != 1 {
		t.Error("an unknown leader key did something")
	}
}

func TestTheFrameFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 50}, {60, 20}} {
		m := sized(size[0], size[1])
		m = openPanel(m, "f", "angel")
		m = openPanel(m, "d", "marbri")
		m = openPanel(m, "r", "flying")

		view := m.View()
		lines := splitLines(view)
		if len(lines) > size[1] {
			t.Errorf("%dx%d: drew %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := visibleWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d is %d columns", size[0], size[1], i, w)
			}
		}
	}
}

func TestClosingTheLastPanelLandsOnTheSplash(t *testing.T) {
	// Not straight out of the program: there is always one press between
	// you and the exit, and esc from the splash is it.
	m := drive(sized(120, 40), "space", "f")
	m = drive(m, "esc")

	if !m.ws.empty() {
		t.Fatal("the panel did not close")
	}
	if view := m.View(); view == "" {
		t.Error("nothing was drawn after the last panel closed")
	}
}

// ── The list, driven through the model ──────────────────────────

// withCards opens a panel and fills it, standing in for the search that
// phase 5 will run.
func withCards(m Model, key string, cards []deck.Card, order cardSort) Model {
	m = openPanel(m, key, "query")
	l := newCardList(cards, order, "")
	l.name = "query"
	m.ws.current().show(l)
	return m
}

func TestJAndKMoveTheCursor(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "j", "j")
	if c, _ := m.ws.current().cardsView().current(); c.Card.Name != "Sol Ring" {
		t.Errorf("two js landed on %s", c.Card.Name)
	}
	m = drive(m, "k")
	if c, _ := m.ws.current().cardsView().current(); c.Card.Name != "Llanowar Elves" {
		t.Errorf("k landed on %s", c.Card.Name)
	}
}

func TestGGAndShiftGGoToTheEnds(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "G")
	if c, _ := m.ws.current().cardsView().current(); c.Card.Name != "Forest" {
		t.Errorf("G landed on %s, want the last card", c.Card.Name)
	}
	// gg, not g: g is a prefix so that gd and gv can exist.
	m = drive(m, "g", "g")
	if m.ws.current().cardsView().cursor.at != 0 {
		t.Error("gg did not go back to the top")
	}
}

func TestOCyclesTheSortAndTheHeaderSaysSo(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "o")
	if got := m.ws.current().cardsView().order; got != sortMana {
		t.Errorf("o moved to %v, want mana value", got)
	}
	if !strings.Contains(stripANSI(m.View()), "mana value") {
		t.Error("the panel does not say what it is sorted by")
	}
	m = drive(m, "O", "O")
	if got := m.ws.current().cardsView().order; got != sortUSD {
		t.Errorf("O wrapped to %v", got)
	}
}

func TestSlashOpensTheFilterAndNarrowsAsYouType(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "/")
	if !m.ws.current().filtering {
		t.Fatal("/ did not open the filter")
	}
	m = drive(m, "e", "l", "f")
	if got := m.ws.current().cardsView().count(); got != 2 {
		t.Errorf("filtering to 'elf' left %d cards", got)
	}
	m = drive(m, "enter")
	if m.ws.current().filtering {
		t.Error("enter left the filter prompt open")
	}
}

func TestEscAbandonsTheFilterPrompt(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "/", "e", "l", "f", "esc")
	if m.ws.current().cardsView().count() != 4 {
		t.Error("esc left the half-typed narrowing in place")
	}
}

func TestTheEscCascadeInAList(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "/", "e", "l", "f", "enter")
	m = drive(m, "v") // pick one out

	m = drive(m, "esc") // the selection is transient, so it goes first
	if m.ws.current().cardsView().markCount() != 0 {
		t.Error("the first esc did not clear the selection")
	}
	// The filter is not transient: esc leaves it so you can carry on reading
	// the cards it left. b clears it, esc steps past it.
	if m.ws.current().cardsView().filter == "" {
		t.Error("esc cleared the filter, which it should leave for b")
	}
	m = drive(m, "esc") // nothing transient left, so the panel goes
	if m.ws.count() != 0 {
		t.Error("esc did not close the panel once the selection was gone")
	}
}

func TestBClearsTheFilterWithoutClosingThePanel(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "/", "e", "l", "f", "enter")
	if m.ws.current().cardsView().count() != 2 {
		t.Fatalf("the filter did not narrow the list: %d rows", m.ws.current().cardsView().count())
	}

	m = drive(m, "b")
	if got := m.ws.current().cardsView().filter; got != "" {
		t.Errorf("b left the filter %q in place", got)
	}
	if m.ws.count() != 1 {
		t.Error("b closed the panel instead of clearing the filter")
	}
	if m.ws.current().cardsView().count() != 4 {
		t.Error("clearing the filter did not put the whole list back")
	}
}

func TestEveryListFlagsWhatTheEditingDeckHolds(t *testing.T) {
	m := sized(160, 30)
	m = withCards(m, "f", sample(), sortArrival)
	m = withCards(m, "d", sample()[:2], sortArrival)

	// Pretend the second panel is the deck being built.
	m.ws.editing = 1

	members := m.membersFor(m.ws.panels[0].cardsView())
	if members["sol ring"] {
		t.Error("Sol Ring is not in the deck but was flagged")
	}
	if !members["llanowar elves"] {
		t.Error("Llanowar Elves is in the deck and should be flagged in the search")
	}
}

func TestTheEditingDeckFlagsWhatTheListsHaveTurnedUp(t *testing.T) {
	m := sized(160, 30)
	m = withCards(m, "f", sample()[2:], sortArrival) // Sol Ring, Forest
	m = withCards(m, "d", sample(), sortArrival)
	m.ws.editing = 1

	members := m.membersFor(m.ws.panels[1].cardsView())
	if !members["sol ring"] {
		t.Error("the deck should flag the card the search turned up")
	}
	if members["dwynen, gilt-leaf daen"] {
		t.Error("a card no list is showing was flagged in the deck")
	}
}
