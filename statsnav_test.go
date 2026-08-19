package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestLandsAreOutOfTheCurveAndTheColours(t *testing.T) {
	// A deck is mostly lands, and nearly all of them cost nothing and count
	// as colourless. Left in, they put a column at zero taller than the rest
	// of the curve put together and a "Colorless" bar that reads as if the
	// deck were full of colourless spells. The type breakdown already says
	// how many lands there are, and says it better.
	items := []list.Item{
		cardItem{Card: ScryfallCard{Name: "Command Tower", TypeLine: "Land", CMC: 0}, Qty: 1},
		cardItem{Card: ScryfallCard{Name: "Plains", TypeLine: "Basic Land — Plains", CMC: 0}, Qty: 30},
		cardItem{Card: ScryfallCard{Name: "Sol Ring", TypeLine: "Artifact", CMC: 1}, Qty: 1},
		cardItem{Card: ScryfallCard{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, Qty: 1},
	}
	entries := toEntries(items)

	count := func(rows []statRow, label string) int {
		for _, r := range rows {
			if r.label != label {
				continue
			}
			var n int
			for _, ci := range entries {
				if r.match(ci) {
					n += ci.Qty
				}
			}
			return n
		}
		t.Fatalf("no %q row", label)
		return 0
	}

	if got := count(cmcRows(), "0"); got != 0 {
		t.Errorf("%d cards at mana value 0, want none — lands should be out of it", got)
	}
	if got := count(cmcRows(), "1"); got != 1 {
		t.Errorf("mana value 1 = %d, want the Sol Ring", got)
	}
	if got := count(colorRows(), "Colorless"); got != 1 {
		t.Errorf("Colorless = %d, want just the Sol Ring", got)
	}
	if got := count(colorRows(), "White"); got != 1 {
		t.Errorf("White = %d", got)
	}
}

func TestStatGroupsSayTheyExcludeLands(t *testing.T) {
	// The reader has to be told, or the numbers look wrong.
	entries := toEntries([]list.Item{
		cardItem{Card: ScryfallCard{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 30},
		cardItem{Card: ScryfallCard{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, Qty: 1},
	})

	var titles []string
	for _, g := range statGroups(entries, entries) {
		titles = append(titles, g.title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Mana Value (excl. lands)", "Color (excl. lands)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no group titled %q; titles are %s", want, joined)
		}
	}
}

func TestPlainKeysMoveTheListWhateverThePanelShows(t *testing.T) {
	// j/k move through cards, always. The statistics have their own keys —
	// J/K — so that walking the list and walking the categories never
	// compete for the same press.
	m := deckAndSearch(t, 190, 40)
	m = press(m, "s")
	if m.panel != panelStats {
		t.Fatalf("panel = %v", m.panel)
	}

	before := m.results.list.Index()
	m = press(m, "j")
	if m.results.list.Index() == before {
		t.Error("j did not move the list with the statistics up")
	}
	if m.results.statIndex != -1 {
		t.Errorf("j entered the categories: index %d", m.results.statIndex)
	}

	// J/K are what the categories answer to.
	m = press(m, "J")
	if m.results.statIndex != 0 || m.results.statFilter == nil {
		t.Fatalf("J did not enter the categories: index %d", m.results.statIndex)
	}
	m = press(m, "K")
	if m.results.statIndex != 0 {
		t.Errorf("K past the top left the index at %d", m.results.statIndex)
	}
}

func TestPlainKeysMoveTheListWithTheCardPanelUp(t *testing.T) {
	// And with the card panel up they move the list, as they always have —
	// there the panel is describing whichever card the cursor is on.
	m := deckAndSearch(t, 190, 40)
	if m.panel != panelCard {
		t.Fatalf("panel = %v", m.panel)
	}
	before := m.results.list.Index()
	m = press(m, "j")
	if m.results.list.Index() == before {
		t.Error("j did not move the list while the card panel was up")
	}
}

func TestBothListsFlagWhatTheOtherHolds(t *testing.T) {
	m := editableDeck(t)
	m = drive(m, searchResultMsg{cards: []ScryfallCard{
		{Name: "Sol Ring", TypeLine: "Artifact"},    // the deck runs this
		{Name: "Black Lotus", TypeLine: "Artifact"}, // it doesn't
	}, totalCards: 2})

	inDeck := m.deckMembership()
	if !inDeck["sol ring"] || inDeck["black lotus"] {
		t.Errorf("deck membership = %v", inDeck)
	}

	// And the other way round, which is the half that was missing: a deck
	// card the search just turned up.
	inResults := m.resultMembership()
	if !inResults["sol ring"] {
		t.Error("the deck's Sol Ring is not flagged as one the search found")
	}
	if inResults["plains"] {
		t.Error("a deck card the search didn't find is flagged as though it had")
	}

	// Both columns show it.
	view := stripANSI(m.View())
	if n := strings.Count(view, "▪"); n < 2 {
		t.Errorf("only %d rows are flagged; both lists should mark Sol Ring:\n%s", n, view)
	}
}

func TestCommanderFlagWinsTheSlot(t *testing.T) {
	// A deck card can be a commander and be in the search results at once.
	// There's one slot, and the commander is the more important of the two.
	d := compactDelegate{inOther: map[string]bool{"ghen": true}}
	both := cardItem{Card: ScryfallCard{Name: "Ghen"}, Commander: true}
	if got := stripANSI(d.gutter(both)); !strings.Contains(got, "★") {
		t.Errorf("gutter = %q, want the commander flag", got)
	}
}

func TestTheCardPanelIsTheHub(t *testing.T) {
	// r and t open from the card view and come back to it. From the
	// statistics they do nothing: swapping a panel you asked for for one
	// you didn't is worse than ignoring the key.
	m := deckAndSearch(t, 190, 40)
	if m.panel != panelCard {
		t.Fatalf("panel = %v at rest", m.panel)
	}

	for _, tt := range []struct {
		key  string
		want panelMode
	}{
		{"r", panelRules},
		{"r", panelCard},
		{"s", panelStats},
		{"r", panelStats}, // ignored, not swapped
		{"t", panelStats}, // ignored
		{"s", panelCard},
	} {
		m = press(m, tt.key)
		if m.panel != tt.want {
			t.Errorf("%q from there left the panel at %v, want %v", tt.key, m.panel, tt.want)
		}
	}
}

func TestEnterGoesBackToTheCard(t *testing.T) {
	// It used to open the rules browser, which isn't what enter on a card
	// suggests and made the panel hard to leave.
	m := deckAndSearch(t, 190, 40)

	for _, key := range []string{"r", "s"} {
		m = press(m, key)
		if m.panel == panelCard {
			t.Fatalf("%q did not open a panel", key)
		}
		m = pressKey(m, tea.KeyEnter)
		if m.panel != panelCard {
			t.Errorf("enter from %q left the panel at %v", key, m.panel)
		}
		if m.state != stateResults {
			t.Fatalf("enter left the app in state %v", m.state)
		}
	}
}

func TestTheRulesBrowserOpensFromTheRulesPanel(t *testing.T) {
	m := deckAndSearch(t, 190, 40)
	m.rules = loadTestRules(t)

	// R does nothing until the rules are what's on the panel.
	m = press(m, "R")
	if m.state != stateResults {
		t.Errorf("R from the card view opened %v", m.state)
	}

	m = press(m, "r")
	m = press(m, "R")
	if m.state != stateRules {
		t.Fatalf("R from the rules panel left the app in state %v", m.state)
	}
	// Scoped to the card, not the whole rulebook — that's on ,r. The
	// fixture card has flying and hexproof on it.
	if len(m.rulesList.Items()) == 0 {
		t.Error("the browser opened with nothing in it")
	}
	if len(m.rulesList.Items()) > 200 {
		t.Errorf("the browser opened on the whole rulebook: %d entries", len(m.rulesList.Items()))
	}
}
