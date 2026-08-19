package ui

import (
	"strings"
	"testing"

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

func TestTheLeaderJumpsStraightToAPanel(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")
	m = drive(m, "space", "1")
	if m.ws.focused != 0 {
		t.Errorf("space 1 went to %d", m.ws.focused)
	}
	m = drive(m, "space", "3")
	if m.ws.focused != 2 {
		t.Errorf("space 3 went to %d", m.ws.focused)
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

func TestEscLeavesTheBarThenClosesThePanel(t *testing.T) {
	m := drive(sized(120, 40), "space", "f")
	m = drive(m, "a", "n", "g", "e", "l", "enter")
	if m.ws.current().searchOpen {
		t.Error("running a search left the bar open over the results")
	}

	m = drive(m, "esc")
	if m.ws.count() != 1 {
		t.Error("esc closed a panel that still had something to clear")
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

func TestTheRowShowsWhichPanelYouAreOn(t *testing.T) {
	m := openPanel(sized(120, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")

	view := stripANSI(m.View())
	if !strings.Contains(view, "2/2") {
		t.Errorf("the hint line does not say which panel of how many:\n%s", view)
	}
}

func TestOffScreenPanelsAreFlagged(t *testing.T) {
	// Eight panels on a narrow terminal means some are out of sight; the
	// row says so rather than letting them silently vanish.
	m := sized(100, 40)
	for i := 0; i < 8; i++ {
		m = openPanel(m, "f", "angel")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "↔") {
		t.Errorf("no sign that panels are off screen:\n%s", view)
	}
}
