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

func TestImportingFromTheMoxfieldBrowserOpensTheDeck(t *testing.T) {
	// Reported: from the deck picker, m, pick a deck, enter — and you came
	// back to the picker with nothing opened, nothing selected, and q doing
	// nothing. Two causes, both about where a screen goes when it closes.
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{"sol ring": {Name: "Sol Ring", TypeLine: "Artifact"}})

	d, err := parseDeckFile(strings.NewReader("name: Hinata\n[mainboard]\n1 Sol Ring\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := saveDeckVersioned("hinata", d); err != nil {
		t.Fatal(err)
	}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// The path that broke: into the picker first, then Moxfield from it.
	m = leaderPress(m, "d")
	m = press(m, "m")
	m = drive(m, moxUserDecksMsg{user: "MarBri", decks: []moxUserDeck{
		{Name: "Hinata", PublicID: "abc", Cards: 100},
	}})
	m = drive(m, deckImportedMsg{slug: "hinata"})

	if m.state != stateResults {
		t.Fatalf("state = %v, want the deck on screen — not the picker it came from", m.state)
	}
	if m.deck == nil || m.deck.slug != "hinata" {
		t.Fatalf("the imported deck was not opened: %+v", m.deck)
	}
	// Arriving at a deck is a destination, not a screen to step back out of.
	if len(m.backStack) != 0 {
		t.Errorf("the way back is still %v", m.backStack)
	}
}

func TestNestedScreensKeepTheirOwnWayBack(t *testing.T) {
	// One shared "previous state" meant opening the key reference from the
	// deck picker — or Moxfield from it — overwrote the picker's own way
	// back, and esc and q from the picker afterwards did nothing at all.
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{})

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})

	m = leaderPress(m, "d")
	if m.state != stateDecks {
		t.Fatalf("state = %v", m.state)
	}

	// Two levels down and back up again.
	m = press(m, "?")
	if m.state != stateKeys {
		t.Fatalf("? left the app in state %v", m.state)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateDecks {
		t.Fatalf("esc from the reference went to %v, want back to the picker", m.state)
	}

	// And the picker still knows its own way out.
	m = press(m, "q")
	if m.state != stateResults {
		t.Errorf("q from the picker went to %v, want the results it was opened from", m.state)
	}

	// The same through Moxfield, which is where it was noticed.
	m = leaderPress(m, "d")
	m = press(m, "m")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}) // out of the prompt
	if m.state != stateDecks {
		t.Fatalf("esc from Moxfield went to %v, want the picker", m.state)
	}
	m = press(m, "q")
	if m.state != stateResults {
		t.Errorf("q from the picker went to %v after nesting", m.state)
	}
}

func TestSearchAsksForDecksThatArentLegal(t *testing.T) {
	// Moxfield's search returns only format-legal decks unless told
	// otherwise, and for anyone who builds in the open that's a small
	// fraction: an account with 42 public decks answered with 11, and the
	// 31 it left out were the ones mid-build — 157 cards, or 3, or none
	// yet. Those are the decks you'd open the list to work on.
	if !strings.Contains(moxSearchURL, "showIllegal=true") {
		t.Error("the deck search doesn't ask for decks that aren't legal yet")
	}
	// And it asks for them a hundred at a time rather than a screenful,
	// since it pages through the rest.
	if moxUserPageSize < 100 {
		t.Errorf("page size is %d", moxUserPageSize)
	}
	if moxUserMaxPages < 2 {
		t.Errorf("only %d page(s) are ever fetched", moxUserMaxPages)
	}
}

func TestDecksThatArentLegalAreMarked(t *testing.T) {
	gitRepo(t)
	seedCache(t, map[string]ScryfallCard{})

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = leaderPress(m, "m")
	m = drive(m, moxUserDecksMsg{user: "MarBri", decks: []moxUserDeck{
		{Name: "Finished", Format: "commander", PublicID: "a", Cards: 100, Legal: true},
		{Name: "Half built", Format: "commander", PublicID: "b", Cards: 12},
	}})

	view := stripANSI(m.viewMoxUser())
	if !strings.Contains(view, "not legal") {
		t.Errorf("a deck that isn't legal yet isn't marked:\n%s", view)
	}
	// Only the one, though — most of a builder's decks are mid-build and
	// saying so on every row would be noise.
	if n := strings.Count(view, "not legal"); n != 1 {
		t.Errorf("%q appears %d times, want 1:\n%s", "not legal", n, view)
	}
}

func TestUserDecksAreOrderedLegalThenNewest(t *testing.T) {
	// Most of a builder's decks are half-built. Burying the finished ones
	// under thirty works-in-progress makes the list harder to use than it
	// needs to be.
	decks := []moxUserDeck{
		{Name: "old wip", Updated: "2023-01-01T00:00:00Z"},
		{Name: "new done", Updated: "2025-12-01T00:00:00Z", Legal: true},
		{Name: "new wip", Updated: "2025-12-27T00:00:00Z"},
		{Name: "old done", Updated: "2024-01-01T00:00:00Z", Legal: true},
		{Name: "undated wip"},
	}
	sortUserDecks(decks)

	var got []string
	for _, d := range decks {
		got = append(got, d.Name)
	}
	want := "new done|old done|new wip|old wip|undated wip"
	if strings.Join(got, "|") != want {
		t.Errorf("\n got %s\nwant %s", strings.Join(got, "|"), want)
	}
}
