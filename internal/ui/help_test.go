package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"
)

func TestTheLeaderMenuShowsEveryEntry(t *testing.T) {
	// The entries you can't see are exactly the ones you opened the menu to
	// read, so it wraps rather than being cut off.
	for _, width := range []int{50, 70, 100, 160} {
		m := withCards(sized(width, 24), "f", sample(), sortArrival)
		m = drive(m, "space")

		bar := stripANSI(strings.Join(m.leaderBarLines(), " "))
		for _, c := range leaderMenu {
			if !strings.Contains(bar, c.what) {
				t.Errorf("at %d columns %q is missing from the menu:\n%s",
					width, c.what, bar)
			}
		}
	}
}

func TestTheMenuNeverPushesThePanelsOffTheScreen(t *testing.T) {
	// It used to reserve one row however many it took, and the frame grew
	// past the terminal — the panels lost their top edge.
	for _, width := range []int{40, 50, 70, 100, 200} {
		for _, height := range []int{10, 14, 24, 40} {
			m := withCards(sized(width, height), "f", sample(), sortArrival)
			m = drive(m, "space")

			lines := splitLines(m.View())
			if len(lines) > height {
				t.Errorf("%dx%d: the menu grew the frame to %d lines",
					width, height, len(lines))
			}
			for i, line := range lines {
				if w := visibleWidth(line); w > width {
					t.Errorf("%dx%d: line %d is %d columns", width, height, i, w)
				}
			}
		}
	}
}

func TestEveryMenuKeyPutsTheMenuAway(t *testing.T) {
	// Including the ones that go on to do something else.
	for _, c := range leaderMenu {
		m := withCards(sized(140, 30), "f", sample(), sortArrival)
		m = drive(m, "space")
		if !m.leader {
			t.Fatal("space did not raise the menu")
		}
		m = drive(m, c.key)
		if m.leader {
			t.Errorf("the menu stayed up after %q", c.key)
		}
	}
}

func TestTheReferenceDescribesThePanelYouAreIn(t *testing.T) {
	// A reference listing keys that do nothing here is worse than none.
	search := withCards(sized(140, 40), "f", sample(), sortArrival)
	searchKeys := keyText(search)

	own := withCards(sized(140, 40), "d", sample(), sortArrival)
	own.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	deckKeys := keyText(own)

	if !strings.Contains(deckKeys, "This deck") {
		t.Errorf("a deck of yours is headed:\n%s", deckKeys)
	}
	if !strings.Contains(searchKeys, "These cards") {
		t.Errorf("a search is headed:\n%s", searchKeys)
	}

	// Editing keys belong to a deck, not to a search.
	for _, only := range []string{"tag, add and tag", "undo"} {
		if !strings.Contains(deckKeys, only) {
			t.Errorf("%q is missing from a deck's keys", only)
		}
		if strings.Contains(searchKeys, only) {
			t.Errorf("%q is offered on a search, where it does nothing", only)
		}
	}
}

func TestTheDecksPanelHasItsOwnKeys(t *testing.T) {
	m := drive(sized(140, 40), "space", "d")
	got := keyText(m)

	if !strings.Contains(got, "Your decks") {
		t.Errorf("headed:\n%s", got)
	}
	for _, want := range []string{"follow a deck", "new deck", "rename"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing:\n%s", want, got)
		}
	}
}

func TestTheBarsKeysAreOnTheHintLineNotInTheReference(t *testing.T) {
	// ? is a printable key, so while you're typing it types. The bar's own
	// keys belong where you can see them without stopping.
	m := drive(sized(140, 40), "space", "f")
	if !strings.Contains(stripANSI(m.View()), "tab target") {
		t.Errorf("the hint line does not describe the bar:\n%s", stripANSI(m.View()))
	}

	m = drive(m, "?")
	if got := m.ws.current().search.Value(); got != "?" {
		t.Errorf("? did something other than type: bar holds %q", got)
	}
}

func TestTheReferenceFitsTheScreen(t *testing.T) {
	// It laid itself out in one column and ran off the bottom, which loses
	// exactly the part you were reaching for.
	for _, size := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {200, 50}, {70, 16}} {
		m := withCards(sized(size[0], size[1]), "d", sample(), sortArrival)
		m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
		m = drive(m, "?")

		lines := splitLines(m.View())
		if len(lines) > size[1] {
			t.Errorf("%dx%d: the reference is %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := visibleWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d is %d columns", size[0], size[1], i, w)
			}
		}
	}
}

func TestTheReferenceKeepsEverySectionWhole(t *testing.T) {
	// Splitting a block across two columns would be worse than a third
	// column.
	sections := []keySection{
		{"one", [][2]string{{"a", "x"}, {"b", "y"}}},
		{"two", [][2]string{{"c", "z"}}},
		{"three", [][2]string{{"d", "w"}, {"e", "v"}, {"f", "u"}}},
	}
	groups := packSections(sections, 5)
	if len(groups) < 2 {
		t.Fatalf("everything fitted in %d column(s) of 5 lines", len(groups))
	}

	total := 0
	for _, g := range groups {
		for _, s := range g {
			total++
			_ = s
		}
	}
	if total != len(sections) {
		t.Errorf("%d sections came out of %d", total, len(sections))
	}
}

// keyText is the reference as plain text.
func keyText(m Model) string {
	m = drive(m, "?")
	return stripANSI(m.View())
}
