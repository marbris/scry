package main

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// aSession sets up a temporary home with one deck in it.
func aSession(t *testing.T) {
	t.Helper()
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{"sol ring": {Name: "Sol Ring", TypeLine: "Artifact"}})

	d, err := parseDeckFile(strings.NewReader("name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := saveDeckVersioned("ghen", d); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	aSession(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, openLocalDeckCmd("ghen")())
	m = m.setFocus(focusSearch)
	m.searchInput.SetValue("t:dragon c:rw")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Quitting records where you were.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); !quits(cmd) {
		t.Fatal("q did not quit")
	}
	saved := loadSession()
	if saved.Deck != "ghen" || saved.Query != "t:dragon c:rw" {
		t.Fatalf("session = %+v", saved)
	}

	// And `scry` with no arguments comes back to it.
	back := initialModel().restore()
	if back.initialDeckSlug != "ghen" {
		t.Errorf("the deck was not restored: %q", back.initialDeckSlug)
	}
	if back.initialQuery != "t:dragon c:rw" {
		t.Errorf("the search was not restored: %q", back.initialQuery)
	}
	if back.searchInput.Value() != "t:dragon c:rw" {
		t.Errorf("the bar shows %q", back.searchInput.Value())
	}
	if !back.searching {
		t.Error("the restored search isn't marked as running")
	}
}

func TestSessionRemembersTheSearchThatRan(t *testing.T) {
	// Not whatever half-typed thing was left in the bar.
	aSession(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m.setFocus(focusSearch)
	m.searchInput.SetValue("t:dragon")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	m.searchInput.SetValue("t:goblin half-typed")

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if got := loadSession().Query; got != "t:dragon" {
		t.Errorf("session query = %q, want the one that ran", got)
	}
}

func TestAnArgumentWinsOverTheSession(t *testing.T) {
	aSession(t)
	if err := saveSession(session{Query: "t:dragon", Deck: "ghen"}); err != nil {
		t.Fatal(err)
	}

	// scry <query>
	q := initialModel()
	q.initialQuery = "t:goblin"
	q = q.restore()
	if q.initialQuery != "t:goblin" {
		t.Errorf("a command-line query became %q", q.initialQuery)
	}
	if q.initialDeckSlug != "" {
		t.Errorf("a command-line query also reopened %q", q.initialDeckSlug)
	}

	// scry deck <name>
	d := initialModel()
	d.initialDeckSlug = "other"
	d = d.restore()
	if d.initialDeckSlug != "other" || d.initialQuery != "" {
		t.Errorf("a named deck was overridden: deck=%q query=%q", d.initialDeckSlug, d.initialQuery)
	}
}

func TestSessionSkipsWhatIsNoLongerThere(t *testing.T) {
	aSession(t)
	if err := saveSession(session{Query: "t:dragon", Deck: "deleted-since"}); err != nil {
		t.Fatal(err)
	}

	// A deck deleted between sessions shouldn't open an error on startup.
	m := initialModel().restore()
	if m.initialDeckSlug != "" {
		t.Errorf("tried to reopen a deck that isn't there: %q", m.initialDeckSlug)
	}
	// The search still comes back.
	if m.initialQuery != "t:dragon" {
		t.Errorf("query = %q", m.initialQuery)
	}
}

func TestABrowsedMoxfieldDeckIsNotYourSession(t *testing.T) {
	// It isn't yours, and it might not be public tomorrow. The search still
	// belongs to the session.
	aSession(t)

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Someone else's", id: "Y8dZ7", url: "https://moxfield.com/decks/Y8dZ7"},
		cards: deckFixture(),
	})
	m.lastQuery = "t:dragon"

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	saved := loadSession()
	if saved.Deck != "" {
		t.Errorf("a borrowed deck was saved as the session: %q", saved.Deck)
	}
	if saved.Query != "t:dragon" {
		t.Errorf("query = %q", saved.Query)
	}
}

func TestAFreshInstallStartsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SCRY_DECKS_DIR", t.TempDir())

	m := initialModel().restore()
	if m.initialQuery != "" || m.initialDeckSlug != "" || m.searching {
		t.Errorf("a fresh install restored something: %+v", m.currentSession())
	}
}

func TestASessionFileThatWontParse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(sessionPath(), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	// A fresh start, which is what would have happened anyway.
	if got := loadSession(); got != (session{}) {
		t.Errorf("a corrupt session loaded as %+v", got)
	}
	m := initialModel().restore()
	if m.initialQuery != "" || m.initialDeckSlug != "" {
		t.Errorf("a corrupt session restored %+v", m.currentSession())
	}
}
