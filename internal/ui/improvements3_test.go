package ui

import (
	"strings"
	"testing"

	"scry/internal/config"
	"scry/internal/deck"
	"scry/internal/mtg"

	tea "github.com/charmbracelet/bubbletea"
)

// alt is a key with alt held, the way the terminal reports it.
func alt(m Model, r rune) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true})
	return next.(Model)
}

func cardNames(l *cardList) []string {
	var out []string
	for _, c := range l.rows {
		out = append(out, c.Card.Name)
	}
	return out
}

func TestTheCycleRunsInTheNewOrderAndLeavesNameOut(t *testing.T) {
	want := []cardSort{sortArrival, sortMana, sortColor, sortType, sortPower,
		sortToughness, sortEDHREC, sortUSD, sortRarity, sortInclusion}
	s := sortArrival
	for i, w := range want {
		if s != w {
			t.Fatalf("step %d is %v, want %v", i, s, w)
		}
		s = s.next(1)
	}
	if s != sortArrival {
		t.Errorf("the cycle didn't wrap: %v", s)
	}
	for s := sortArrival.next(1); s != sortArrival; s = s.next(1) {
		if s == sortName {
			t.Error("name is in the cycle by default")
		}
	}
}

func TestEachOrderStartsBestFirst(t *testing.T) {
	for s, want := range map[cardSort]bool{
		sortPower: true, sortToughness: true, sortUSD: true, sortRarity: true,
		sortArrival: false, sortEDHREC: false, sortMana: false, sortColor: false,
		sortType: false, sortInclusion: false, sortName: false,
	} {
		if got := s.descending(); got != want {
			t.Errorf("%v starts descending=%v, want %v", s, got, want)
		}
	}
}

func TestAltDotTurnsSort1Round(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, ".") // mana value, ascending
	l := m.ws.current().cardsView()
	if got := l.rows[0].Card.Name; got != "Forest" {
		t.Fatalf("mana value ascending starts with %s", got)
	}
	if !strings.Contains(stripANSI(m.View()), "mana value ↑") {
		t.Error("the header doesn't say which way the order runs")
	}
	m = alt(m, '.')
	if got := l.rows[0].Card.Name; got != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("alt+. left %s on top, want the dearest card", got)
	}
	if !strings.Contains(stripANSI(m.View()), "mana value ↓") {
		t.Error("the header didn't turn round with the order")
	}
	// Moving on to the next order starts it the way it reads best, not the
	// way the last one was left.
	m = drive(m, ".")
	if l.order != sortColor || l.desc1 {
		t.Errorf("stepped on to %v descending=%v, want colour ascending", l.order, l.desc1)
	}
}

func TestAltCommaTurnsSort2Round(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, ".", ".", ".", ",") // sort 1 type, sort 2 mana value
	l := m.ws.current().cardsView()
	if l.order != sortType || l.order2 != sortMana {
		t.Fatalf("got %v / %v", l.order, l.order2)
	}
	before := cardNames(l)
	m = alt(m, ',')
	if !l.desc2 {
		t.Fatal("alt+, didn't turn sort 2 round")
	}
	after := cardNames(l)
	// The two creatures swap within the creature group: Dwynen costs 4,
	// the elves 1.
	if strings.Join(before, "|") == strings.Join(after, "|") {
		t.Errorf("turning sort 2 round moved nothing: %v", after)
	}
}

func TestBlanksStayAtTheBottomWhicheverWayTheOrderRuns(t *testing.T) {
	cards := []deck.Card{
		{Card: mtg.Card{Name: "Unranked"}},
		{Card: mtg.Card{Name: "Best", EDHRECRank: 1}},
		{Card: mtg.Card{Name: "Worse", EDHRECRank: 50}},
	}
	up := sortCards(cards, sortSpec{first: sortEDHREC})
	down := sortCards(cards, sortSpec{first: sortEDHREC, desc1: true})
	if got := up[2].Card.Name; got != "Unranked" {
		t.Errorf("ascending put %s last", got)
	}
	if down[0].Card.Name != "Worse" || down[2].Card.Name != "Unranked" {
		t.Errorf("descending gave %v %v %v", down[0].Card.Name, down[1].Card.Name, down[2].Card.Name)
	}
}

func TestScryfallOrderTurnedRoundIsTheListUpsideDown(t *testing.T) {
	got := sortCards(sample(), sortSpec{first: sortArrival, desc1: true})
	if got[0].Card.Name != "Forest" || got[3].Card.Name != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("got %v first and %v last", got[0].Card.Name, got[3].Card.Name)
	}
}

func TestInclusionPutsTheEditingDecksCardsFirstThenOtherLists(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)      // the list being sorted
	m = withCards(m, "f", sample()[3:], sortArrival)  // another list: Forest
	m = withCards(m, "d", sample()[2:3], sortArrival) // the deck: Sol Ring
	m.ws.current().cardsView().deck = &deck.Info{Name: "Deck", Slug: "deck", Format: "commander"}
	m.ws.editing = 2
	m = focusOn(m, 0)

	l := m.ws.panels[0].cardsView()
	for l.order != sortInclusion {
		m = drive(m, ".")
	}
	got := cardNames(l)
	if got[0] != "Sol Ring" || got[1] != "Forest" {
		t.Errorf("inclusion order is %v, want Sol Ring, then Forest, then the rest", got)
	}
}

func TestInclusionReSortsAsTheDeckChanges(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	m = withCards(m, "d", sample()[2:3], sortArrival) // Sol Ring
	m.ws.current().cardsView().deck = &deck.Info{Name: "Deck", Slug: "deck", Format: "commander"}
	m.ws.editing = 1
	m = focusOn(m, 0)

	l := m.ws.panels[0].cardsView()
	for l.order != sortInclusion {
		m = drive(m, ".")
	}
	// On the second row, the first card not in the deck.
	m = drive(m, "j")
	added, _ := l.current()
	next := l.rows[l.cursor.at+1]
	m = drive(m, "a")
	if got := cardNames(l)[:2]; got[0] != added.Card.Name && got[1] != added.Card.Name {
		t.Errorf("the added card didn't join the deck's group: %v", got)
	}
	if c, _ := l.current(); c.Card.Name != next.Card.Name {
		t.Errorf("the cursor is on %s, want the card that was below the one added (%s)", c.Card.Name, next.Card.Name)
	}
}

func TestANewSearchKeepsTheSorts(t *testing.T) {
	m, p := typed(sized(140, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)
	m = drive(m, ".", ",", ",")
	m = alt(m, '.')
	l := p.cardsView()
	order, order2, desc1, desc2 := l.order, l.order2, l.desc1, l.desc2

	m, _ = typed(drive(m, "i"), "t:goblin")
	m = drive(m, "enter")
	m = answer(m, p, sample()[1:], 3, nil)
	n := p.cardsView()
	if n == l {
		t.Fatal("the new answer didn't replace the list")
	}
	if n.order != order || n.order2 != order2 || n.desc1 != desc1 || n.desc2 != desc2 {
		t.Errorf("new search is %v/%v %v/%v, want %v/%v %v/%v",
			n.order, n.order2, n.desc1, n.desc2, order, order2, desc1, desc2)
	}
}

func TestTheSortConfigReordersAndTrimsTheCycle(t *testing.T) {
	defer SetSortConfig(nil)
	err := SetSortConfig(&config.Sort{
		Cycle:     []string{"scryfall", "name", "usd"},
		Direction: map[string]string{"usd": "asc", "name": "desc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sortArrival.next(1) != sortName || sortName.next(1) != sortUSD || sortUSD.next(1) != sortArrival {
		t.Error("the cycle isn't the one configured")
	}
	if sortMana.next(1) != sortArrival {
		t.Error("an order left out of the cycle should step to its start")
	}
	if sortUSD.descending() || !sortName.descending() {
		t.Error("the directions weren't applied")
	}
	if !sortPower.descending() {
		t.Error("an order the config doesn't mention lost its default direction")
	}

	if err := SetSortConfig(&config.Sort{Cycle: []string{"mana value", "bogus"}}); err == nil {
		t.Error("an unknown order name went unreported")
	}
	if firstSort() != sortMana {
		t.Error("the usable part of a half-broken cycle wasn't kept")
	}
}

func TestScryfallOrderColoursTheNamesByType(t *testing.T) {
	sol := mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}
	if got := nameColour(sol, sortArrival, sortArrival); got != typeColour(sol.TypeLine) {
		t.Errorf("Scryfall order painted an artifact %v", got)
	}
}
