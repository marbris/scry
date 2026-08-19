package main

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRememberQuery(t *testing.T) {
	var h []string
	h = rememberQuery(h, "t:dragon")
	h = rememberQuery(h, "t:angel")
	if strings.Join(h, "|") != "t:dragon|t:angel" {
		t.Errorf("history = %v, want oldest first", h)
	}

	// Running an old search again moves it to the end rather than adding a
	// second copy, so walking back never steps through the same query twice.
	h = rememberQuery(h, "t:dragon")
	if strings.Join(h, "|") != "t:angel|t:dragon" {
		t.Errorf("history = %v, want the repeat moved to the end", h)
	}

	// Blank searches aren't searches.
	h = rememberQuery(h, "   ")
	if len(h) != 2 {
		t.Errorf("a blank query was remembered: %v", h)
	}

	// It doesn't grow without limit.
	for i := 0; i < queryHistoryMax*2; i++ {
		h = rememberQuery(h, string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	if len(h) > queryHistoryMax {
		t.Errorf("history grew to %d, cap is %d", len(h), queryHistoryMax)
	}
}

func TestQueryHistorySurvivesTheSession(t *testing.T) {
	isolate(t)

	if got := loadQueryHistory(); got != nil {
		t.Errorf("a fresh install has history: %v", got)
	}
	if err := saveQueryHistory([]string{"t:dragon", "t:angel"}); err != nil {
		t.Fatal(err)
	}
	if got := loadQueryHistory(); strings.Join(got, "|") != "t:dragon|t:angel" {
		t.Errorf("history did not come back: %v", got)
	}

	// Rubbish on disk is no history rather than a crash.
	if err := writeString(queryHistoryPath(), "{not json"); err != nil {
		t.Fatal(err)
	}
	if got := loadQueryHistory(); got != nil {
		t.Errorf("a corrupt history file loaded as %v", got)
	}
}

func TestUpWalksTheQueryHistory(t *testing.T) {
	isolate(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.queryHistory = []string{"t:dragon", "t:angel", "c:rw"}
	m = m.setFocus(focusSearch)

	up := func(mm model) model { return drive(mm, tea.KeyMsg{Type: tea.KeyUp}) }
	down := func(mm model) model { return drive(mm, tea.KeyMsg{Type: tea.KeyDown}) }

	// Up walks back from the newest.
	m = up(m)
	if got := m.searchInput.Value(); got != "c:rw" {
		t.Fatalf("first up = %q, want the newest query", got)
	}
	m = up(m)
	if got := m.searchInput.Value(); got != "t:angel" {
		t.Fatalf("second up = %q", got)
	}
	m = up(m)
	m = up(m) // past the end, which stays put rather than wrapping
	if got := m.searchInput.Value(); got != "t:dragon" {
		t.Fatalf("walking past the oldest = %q", got)
	}

	// Down walks forward again.
	m = down(m)
	if got := m.searchInput.Value(); got != "t:angel" {
		t.Errorf("down = %q", got)
	}
}

func TestHistoryKeepsWhatYouWereTyping(t *testing.T) {
	isolate(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.queryHistory = []string{"t:dragon"}
	m = m.setFocus(focusSearch)

	for _, r := range "half typ" {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.searchInput.Value(); got != "t:dragon" {
		t.Fatalf("up = %q", got)
	}

	// Coming back past the newest restores the draft, so glancing at an old
	// search doesn't cost you the one you were composing.
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.searchInput.Value(); got != "half typ" {
		t.Errorf("coming back = %q, want what was being typed", got)
	}
}

func TestTypingLeavesTheHistory(t *testing.T) {
	isolate(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.queryHistory = []string{"t:dragon", "t:angel"}
	m = m.setFocus(focusSearch)

	m = drive(m, tea.KeyMsg{Type: tea.KeyUp})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	// Editing a recalled query is editing, not still being somewhere in the
	// list — so the next up starts again from the newest.
	if m.historyAt != historyIdle {
		t.Errorf("typing left the walk in progress at %d", m.historyAt)
	}
	if got := m.searchInput.Value(); got != "t:angelx" {
		t.Errorf("value = %q", got)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.searchInput.Value(); got != "t:angel" {
		t.Errorf("up after editing = %q, want the newest query", got)
	}
}

func TestRunningASearchRemembersIt(t *testing.T) {
	isolate(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m.setFocus(focusSearch)
	m.searchInput.SetValue("t:dragon c:r")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if len(m.queryHistory) != 1 || m.queryHistory[0] != "t:dragon c:r" {
		t.Fatalf("history = %v", m.queryHistory)
	}
	// And it's on disk for the next session.
	if got := loadQueryHistory(); len(got) != 1 || got[0] != "t:dragon c:r" {
		t.Errorf("history not written: %v", got)
	}
}

func TestAPastedDeckURLIsNotAQuery(t *testing.T) {
	isolate(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m.setFocus(focusSearch)
	m.searchInput.SetValue("https://moxfield.com/decks/j-0aJlxuOUm9FnKRvJcfZw")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// It loads a deck rather than searching, so it doesn't belong in a list
	// of searches.
	if got := next.(model).queryHistory; len(got) != 0 {
		t.Errorf("a deck URL was remembered as a query: %v", got)
	}
}

func TestHistoryWithNothingInIt(t *testing.T) {
	isolate(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m.setFocus(focusSearch)
	m.searchInput.SetValue("half typed")

	m = drive(m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.searchInput.Value(); got != "half typed" {
		t.Errorf("up with no history changed the bar to %q", got)
	}
}

// writeString drops a file straight onto disk for the tests that need one
// in a particular state.
func writeString(path, body string) error {
	return os.WriteFile(path, []byte(body), 0644)
}

func TestOpeningADeckLeavesTheSearchBarAlone(t *testing.T) {
	// The search bar goes to Scryfall. It used to be filled with the deck
	// reference — "deck ghen" — which isn't a query, would find nothing if
	// you pressed enter on it, and had to be cleared before you could
	// search for anything.
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{})

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m.setFocus(focusSearch)
	m.searchInput.SetValue("t:dragon c:r")

	m = drive(m, deckLoadedMsg{
		info:  deckInfo{Name: "Ghen", Slug: "ghen", Total: 10},
		cards: deckFixture(),
	})

	if got := m.searchInput.Value(); got != "t:dragon c:r" {
		t.Errorf("opening a deck changed the search bar to %q", got)
	}
	// And the deck is still identified, on the line that's for saying so.
	if !strings.Contains(stripANSI(m.resultsHeader()), "Ghen") {
		t.Error("the header doesn't name the open deck")
	}
}

func TestADeckIsNeverRememberedAsAQuery(t *testing.T) {
	gitRepo(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{Name: "Ghen", Slug: "ghen", Total: 10},
		cards: deckFixture(),
	})

	if len(m.queryHistory) != 0 {
		t.Errorf("opening a deck put %v in the query history", m.queryHistory)
	}
	if got := loadQueryHistory(); len(got) != 0 {
		t.Errorf("opening a deck wrote %v to the history file", got)
	}
}

func TestTheBarStartsOnYourLastSearch(t *testing.T) {
	isolate(t)
	if err := saveQueryHistory([]string{"t:angel", "t:dragon c:r"}); err != nil {
		t.Fatal(err)
	}

	m := initialModel().startWithLastQuery()
	if got := m.searchInput.Value(); got != "t:dragon c:r" {
		t.Errorf("the bar starts on %q, want the last search", got)
	}

	// A query given on the command line is what you just asked for, so it
	// wins over the last one.
	m2 := initialModel()
	m2.initialQuery = "t:goblin"
	m2.searchInput.SetValue("t:goblin")
	if got := m2.startWithLastQuery().searchInput.Value(); got != "t:goblin" {
		t.Errorf("a command-line query was overwritten with %q", got)
	}

	// Nothing searched ever means nothing to show.
	isolate(t)
	if got := initialModel().startWithLastQuery().searchInput.Value(); got != "" {
		t.Errorf("a fresh install starts with %q in the bar", got)
	}
}
