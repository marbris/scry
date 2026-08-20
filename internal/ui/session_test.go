package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"
)

func TestASavedWorkspaceComesBack(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m := openPanel(sized(160, 30), "f", "t:elf")
	m = drive(m, "space", "d")
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 2 {
		t.Fatalf("came back to %d panels, want 2", back.ws.count())
	}
	if back.ws.panels[0].kind != KindFind || back.ws.panels[1].kind != KindDecks {
		t.Errorf("came back as %v, %v", back.ws.panels[0].kind, back.ws.panels[1].kind)
	}
}

func TestASearchComesBackAsItsQueryNotItsResults(t *testing.T) {
	// Results can always be fetched again; what was worth keeping is what
	// you asked for.
	m, p := typed(sized(140, 30), "t:elf c:g")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 1 {
		t.Fatalf("came back to %d panels", back.ws.count())
	}
	if got := back.ws.panels[0].title; got != "t:elf c:g" {
		t.Errorf("came back showing %q", got)
	}
}

func TestADeckOfYoursComesBack(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m, _ := openDeckPanel(t, sized(140, 30), "ghen", "Ghen", sample())
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 1 {
		t.Fatalf("came back to %d panels", back.ws.count())
	}
	if got := back.ws.panels[0].title; got != "ghen" {
		t.Errorf("came back showing %q", got)
	}
}

func TestADeckDeletedSinceDoesNotComeBack(t *testing.T) {
	seedDeck(t, "gone", "name: Gone\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	m, _ := openDeckPanel(t, sized(140, 30), "gone", "Gone", sample())
	m.saveSession()
	deck.Delete("gone")

	back, _ := NewRestored()
	if back.ws.count() != 0 {
		t.Errorf("came back to %d panels, want none", back.ws.count())
	}
}

func TestSomebodyElsesDeckDoesNotComeBack(t *testing.T) {
	// It isn't yours and might not be there tomorrow.
	m := withCards(sized(140, 30), "d", sample(), sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Borrowed", ID: "Y8dZ7"}
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 0 {
		t.Errorf("came back to %d panels, want none", back.ws.count())
	}
}

func TestAnEmptyPanelDoesNotComeBack(t *testing.T) {
	// An empty panel is a question you hadn't answered.
	m := drive(sized(140, 30), "space", "f")
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 0 {
		t.Errorf("came back to %d panels, want none", back.ws.count())
	}
}

func TestWithNothingSavedTheSplashIsWhatYouGet(t *testing.T) {
	if err := writeString(sessionPath(), "{}"); err != nil {
		t.Fatal(err)
	}
	m, _ := NewRestored()
	next, _ := m.Update(sizeOf(120, 30))
	m = next.(Model)

	if !m.ws.empty() {
		t.Fatal("something was restored from an empty session")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "space f") {
		t.Errorf("the splash is not what came up:\n%s", view)
	}
}

func TestACorruptSessionIsNotFatal(t *testing.T) {
	if err := writeString(sessionPath(), "{not json"); err != nil {
		t.Fatal(err)
	}
	m, _ := NewRestored()
	if !m.ws.empty() {
		t.Error("something was restored from a broken file")
	}
}

func TestTheEditingDeckComesBack(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m := openPanel(sized(180, 30), "f", "t:elf")
	m, _ = openDeckPanel(t, m, "ghen", "Ghen", sample())
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.editing != 1 {
		t.Errorf("the editing deck came back as panel %d", back.ws.editing)
	}
}
