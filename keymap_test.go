package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLeaderOpensTheThingsItNames(t *testing.T) {
	for _, tt := range []struct {
		key   string
		state state
	}{
		{"d", stateDecks},
		{"r", stateRules},
		{"s", stateHelp},
		{"k", stateKeys},
	} {
		m := pickerModel(t)
		m = leaderPress(m, tt.key)
		if m.state != tt.state {
			t.Errorf("%s%s left the app in state %v, want %v", leaderKey, tt.key, m.state, tt.state)
		}
	}
}

func TestLeaderShowsItsMenu(t *testing.T) {
	m := deckAndSearch(t, 160, 30)

	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(leaderKey)})
	if !m.leader {
		t.Fatal("the leader key did not arm the leader")
	}

	// The menu is on screen, so nothing has to be remembered.
	view := stripANSI(m.View())
	for _, c := range leaderCmds {
		if !strings.Contains(view, c.what) {
			t.Errorf("the leader bar doesn't offer %q:\n%s", c.what, view)
		}
	}
}

func TestLeaderBarFitsNarrowTerminals(t *testing.T) {
	// Clipped halfway through, the menu is worse than useless — it hides
	// exactly the entries you opened it to read.
	for _, w := range []int{80, 100, 120, 160, 200} {
		m := deckAndSearch(t, w, 24)
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(leaderKey)})

		bar := stripANSI(m.leaderBar(w))
		for i, line := range m.leaderBarLines(w) {
			if n := visibleLen(line); n > w {
				t.Errorf("at %d columns leader bar line %d is %d wide", w, i, n)
			}
		}
		if got := len(m.leaderBarLines(w)); got > 3 {
			t.Errorf("at %d columns the leader menu takes %d lines", w, got)
		}
		// Every command is still named, however narrow — by its full label
		// where there's room, by its short one where there isn't.
		for _, c := range leaderCmds {
			if !strings.Contains(bar, c.what) && !strings.Contains(bar, c.short) {
				t.Errorf("at %d columns the bar drops %q: %q", w, c.what, bar)
			}
		}
	}
}

func TestLeaderCancels(t *testing.T) {
	m := deckAndSearch(t, 160, 30)

	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(leaderKey)})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.leader {
		t.Error("esc did not cancel the leader")
	}
	if m.state != stateResults {
		t.Errorf("esc from the leader changed screen: %v", m.state)
	}

	// A key that names nothing cancels rather than doing something else.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(leaderKey)})
	before := m.state
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if m.leader {
		t.Error("an unknown key left the leader armed")
	}
	if m.state != before {
		t.Errorf("an unknown leader key did something: %v", m.state)
	}
}

func TestLeaderInTheSearchBarOnlyWhenEmpty(t *testing.T) {
	m := pickerModel(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")}) // to the search bar
	if !m.searchFocused() {
		t.Fatal("focus is not on the search bar")
	}
	m.searchInput.SetValue("")

	// With nothing typed, the leader works — this is the fresh-launch case,
	// where there's no list to press a letter in.
	m = leaderPress(m, "d")
	if m.state != stateDecks {
		t.Fatalf("the leader did not work from an empty search bar: %v", m.state)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})

	// With a query typed, a comma is a comma — card names have them.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	for _, r := range `name:"Ghen` {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(leaderKey)})
	if m.leader {
		t.Error("the leader armed while a query was being typed")
	}
	if !strings.HasSuffix(m.searchInput.Value(), ",") {
		t.Errorf("the comma was swallowed: %q", m.searchInput.Value())
	}
}

func TestQuitsFromTheMainScreen(t *testing.T) {
	m := deckAndSearch(t, 160, 30)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !quits(cmd) {
		t.Error("q did not quit from the main screen")
	}

	// And it writes a pending edit on the way out, like every other exit.
	m2 := editableDeck(t)
	m2 = press(m2, "a")
	next, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !quits(cmd) {
		t.Fatal("q did not quit")
	}
	if next.(model).deckDirty {
		t.Error("q quit without writing the pending edit")
	}
}

func TestKeyReferenceIsForTheScreenYouCameFrom(t *testing.T) {
	m := deckAndSearch(t, 160, 30)
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})

	if m.state != stateKeys {
		t.Fatalf("? left the app in state %v", m.state)
	}
	// The main screen's reference is longer than a terminal, so what's on
	// screen is the top of it and the rest is a scroll away. Which groups
	// fall below the fold depends on the height, so this checks the whole
	// of it across a scroll rather than asserting a particular line is
	// visible at a particular size.
	seen := stripANSI(m.viewKeyReference())
	if !strings.Contains(seen, "Move") {
		t.Errorf("the reference doesn't start at the top:\n%s", seen)
	}
	for i := 0; i < 40; i++ {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		seen += stripANSI(m.viewKeyReference())
	}
	for _, want := range []string{
		"Add the selected card", "Mark this card", "Undo the last change",
		"The panel", "Decks",
	} {
		if !strings.Contains(seen, want) {
			t.Errorf("the key reference never shows %q, even scrolled", want)
		}
	}

	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateResults {
		t.Errorf("esc left the app in state %v", m.state)
	}

	// From the history it describes the history, not the main screen.
	h := openHistoryOn(t)
	h = drive(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	hv := stripANSI(h.viewKeyReference())
	if !strings.Contains(hv, "Restore this version") {
		t.Errorf("the reference isn't for the history:\n%s", hv)
	}
	if strings.Contains(hv, "Add the selected card") {
		t.Errorf("the reference is showing another screen's keys:\n%s", hv)
	}
}

func TestKeyReferenceCoversEveryBinding(t *testing.T) {
	// Every key the main screen handles has to be in the reference, or the
	// reference is the thing that drifts next.
	handled := []string{
		"a", "x", "c", "w", "r", "s", "t", "v", "V", "T", "q", "i", "/",
		"tab", "esc", "space", "enter", "+", "-", "J/K", "ctrl+d", "ctrl+u",
	}

	var listed strings.Builder
	for _, g := range keysFor(stateResults) {
		for _, b := range g.bindings {
			listed.WriteString(b.keys + " ")
		}
	}
	text := listed.String()

	for _, key := range handled {
		if !strings.Contains(text, key) {
			t.Errorf("%q is handled but not in the key reference", key)
		}
	}
}

func TestLeaderMenuAndReferenceAgree(t *testing.T) {
	// Both are rendered from leaderCmds, and this is what says so.
	rows := leaderBindings()
	for _, c := range leaderCmds {
		var found bool
		for _, b := range rows {
			if b.keys == leaderKey+c.key && b.what == c.what {
				found = true
			}
		}
		if !found {
			t.Errorf("%s%s (%s) is in the menu but not the reference", leaderKey, c.key, c.what)
		}
	}
}

func TestCLIUsage(t *testing.T) {
	for _, want := range []string{"scry deck", "scry rules", "--help", "SCRY_DECKS_DIR"} {
		if !strings.Contains(cliUsage, want) {
			t.Errorf("scry --help doesn't mention %q", want)
		}
	}
}

func TestHintsAreSaidOnceEach(t *testing.T) {
	// ? used to be explained in three places at once — the header, the
	// panel's own hint, and the list's built-in help line — and in one of
	// them by a name it no longer went by.
	for _, tt := range []struct {
		name string
		m    model
	}{
		{"results", deckAndSearch(t, 160, 30)},
		{"search bar", drive(deckAndSearch(t, 160, 30), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})},
	} {
		frame := stripANSI(tt.m.View())
		for _, key := range []string{"?", "/ filter", "o / O sort"} {
			if n := strings.Count(frame, key); n > 1 {
				t.Errorf("%s: %q appears %d times in one frame:\n%s", tt.name, key, n, frame)
			}
		}
	}
}

func TestHintLineFollowsFocusNotTheDeck(t *testing.T) {
	// The search bar's hints used to live on a header line that a deck took
	// over, so opening a deck hid "tab: cycle" while you were still typing.
	for _, withDeck := range []bool{false, true} {
		m := initialModel()
		m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 30})
		m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
		if withDeck {
			m = drive(m, deckLoadedMsg{
				info:  deckInfo{name: "Ghen", slug: "ghen", total: 10},
				cards: deckFixture(),
			})
		}
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})

		hint := stripANSI(m.hintLine(160))
		for _, want := range []string{"tab", "query order", "history"} {
			if !strings.Contains(hint, want) {
				t.Errorf("deck=%v: the search bar hint is missing %q: %q", withDeck, want, hint)
			}
		}
	}
}

func TestHintLineKeepsTheKeysThatLeadElsewhere(t *testing.T) {
	m := deckAndSearch(t, 200, 30)
	for _, w := range []int{50, 60, 80, 120, 200} {
		hint := stripANSI(m.hintLine(w))
		if visibleLen(m.hintLine(w)) > w {
			t.Errorf("at %d columns the hint line is %d wide", w, visibleLen(m.hintLine(w)))
		}
		// However narrow it gets, it keeps the two hints that would have
		// told you about everything it had to drop.
		for _, want := range []string{", more", "? keys"} {
			if !strings.Contains(hint, want) {
				t.Errorf("at %d columns the hint drops %q: %q", w, want, hint)
			}
		}
	}
}

func TestEveryScreenHasAHintLine(t *testing.T) {
	// Each screen's hints come from its own table, so a screen that gains a
	// key gains the hint with it.
	for _, s := range []state{stateResults, stateRules, stateDeckHistory, stateDecks, stateMoxUser, stateHelp} {
		var hinted int
		for _, g := range keysFor(s) {
			for _, b := range g.bindings {
				if b.hint != "" {
					hinted++
				}
			}
		}
		if hinted == 0 {
			t.Errorf("state %v has no hints at all", s)
		}
	}
}
