package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMoxfieldUserName(t *testing.T) {
	// A pasted profile URL should work as well as typing the name.
	for in, want := range map[string]string{
		"https://moxfield.com/users/MarBri":       "MarBri",
		"https://www.moxfield.com/users/MarBri/":  "MarBri",
		"moxfield.com/users/MarBri?tab=decks":     "MarBri",
		"https://moxfield.com/users/MarBri#decks": "MarBri",
		"HTTPS://MOXFIELD.COM/users/MarBri":       "MarBri",
	} {
		got, ok := moxfieldUserName(in)
		if !ok || got != want {
			t.Errorf("moxfieldUserName(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}

	// A bare name isn't a URL and is passed through as typed.
	for _, in := range []string{"MarBri", "", "https://moxfield.com/decks/abc"} {
		if got, ok := moxfieldUserName(in); ok {
			t.Errorf("moxfieldUserName(%q) = %q, want no match", in, got)
		}
	}
}

func TestDeckAge(t *testing.T) {
	// Nothing to go on reads as nothing, rather than as "today".
	for _, in := range []string{"", "not a date"} {
		if got := (moxUserDeck{Updated: in}).age(); got != "" {
			t.Errorf("age(%q) = %q, want empty", in, got)
		}
	}
	if got := (moxUserDeck{Updated: "1970-01-01T00:00:00Z"}).age(); !strings.HasSuffix(got, "years ago") {
		t.Errorf("a very old deck reads as %q", got)
	}

	// The plural is right at one, which is where a naive version says
	// "1 years ago".
	// The plural has to be right at one, which is where a naive version
	// says "1 years ago". Dated from now so the test doesn't rot.
	oneYear := time.Now().AddDate(-1, 0, -2).UTC().Format(time.RFC3339)
	if got := (moxUserDeck{Updated: oneYear}).age(); got != "1 year ago" {
		t.Errorf("a year old reads as %q", got)
	}
	oneMonth := time.Now().AddDate(0, 0, -32).UTC().Format(time.RFC3339)
	if got := (moxUserDeck{Updated: oneMonth}).age(); got != "1 month ago" {
		t.Errorf("a month old reads as %q", got)
	}
}

func TestMoxUserPromptAndBack(t *testing.T) {
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{})

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = leaderPress(m, "m")

	if m.state != stateMoxUser || !m.moxUserAsking {
		t.Fatalf("state = %v, asking = %v", m.state, m.moxUserAsking)
	}
	// The prompt says what it can and can't see, since "where are my other
	// decks" is the obvious first question.
	view := stripANSI(m.viewMoxUser())
	if !strings.Contains(view, "public") {
		t.Errorf("the prompt doesn't mention that only public decks show:\n%s", view)
	}

	// esc with nothing listed goes back rather than to an empty list.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateResults {
		t.Errorf("esc left the app in state %v", m.state)
	}
}

func TestMoxUserListsAndFlagsWhatYouHave(t *testing.T) {
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{})

	// One of these is already imported, under the name it would get.
	d, err := parseDeckFile(strings.NewReader("name: Hinata\n[mainboard]\n1 Sol Ring\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := saveDeckVersioned("hinata", d); err != nil {
		t.Fatal(err)
	}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = leaderPress(m, "m")
	m = drive(m, moxUserDecksMsg{user: "MarBri", decks: []moxUserDeck{
		{Name: "Hinata", Format: "commander", PublicID: "abc", Cards: 100,
			Colors: []string{"W", "U", "R"}, Updated: "2025-12-27T22:56:23.673Z"},
		{Name: "Maarika Force Block", Format: "commander", PublicID: "def", Cards: 100},
	}})

	if m.moxUserErr != nil {
		t.Fatalf("err: %v", m.moxUserErr)
	}
	if got := len(m.moxUserList.Items()); got != 2 {
		t.Fatalf("listed %d decks, want 2", got)
	}
	if m.moxUserAsking {
		t.Error("still asking after the decks arrived")
	}

	// A deck you already have is flagged, so re-importing isn't a surprise.
	first, _ := m.moxUserList.Items()[0].(moxDeckItem)
	second, _ := m.moxUserList.Items()[1].(moxDeckItem)
	if !first.imported {
		t.Error("the deck already on disk is not flagged as yours")
	}
	if second.imported {
		t.Error("a deck you don't have is flagged as yours")
	}

	view := stripANSI(m.viewMoxUser())
	for _, want := range []string{"MarBri · 2 decks", "Hinata", "commander", "100 cards", "WUR", "yours"} {
		if !strings.Contains(view, want) {
			t.Errorf("the list is missing %q:\n%s", want, view)
		}
	}
}

func TestMoxUserErrorReturnsToThePrompt(t *testing.T) {
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{})

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = leaderPress(m, "m")
	m = drive(m, moxUserDecksMsg{err: errNoSuchUser})

	// A name that doesn't exist is a typo more often than not, so the
	// prompt comes back with the message rather than an empty list.
	if !m.moxUserAsking {
		t.Error("an error did not return to the prompt")
	}
	if m.moxUserErr == nil {
		t.Fatal("the error was dropped")
	}
	view := stripANSI(m.viewMoxUser())
	if !strings.Contains(view, "no public decks") {
		t.Errorf("the error isn't on screen:\n%s", view)
	}

	// Typing again clears it.
	m = press(m, "x")
	if m.moxUserErr != nil {
		t.Error("the error outlived the correction")
	}
}

func TestMoxUserImportOpensTheDeck(t *testing.T) {
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{
		"sol ring": {Name: "Sol Ring", TypeLine: "Artifact"},
	})

	// Stand in for the import, which is exercised against the real API
	// elsewhere; what matters here is where you land afterwards.
	d, err := parseDeckFile(strings.NewReader("name: Hinata\n[mainboard]\n1 Sol Ring\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := saveDeckVersioned("hinata", d); err != nil {
		t.Fatal(err)
	}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = leaderPress(m, "m")
	m = drive(m, moxUserDecksMsg{user: "MarBri", decks: []moxUserDeck{
		{Name: "Hinata", PublicID: "abc", Cards: 100},
	}})
	m = drive(m, deckImportedMsg{slug: "hinata"})

	if m.state != stateResults {
		t.Errorf("state = %v, want the deck on screen", m.state)
	}
	if m.deck == nil || m.deck.slug != "hinata" {
		t.Fatalf("the imported deck was not opened: %+v", m.deck)
	}
	if !m.deck.local() {
		t.Error("the imported deck should be one of yours")
	}
}

var errNoSuchUser = &userError{`no public decks for "nobody"`}

type userError struct{ s string }

func (e *userError) Error() string { return e.s }
