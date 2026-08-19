package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"
	"scry/internal/mtg"
)

func deckSample() []deck.Card {
	mk := func(name, tl, cost string, cmc float64, colors []string, rarity string, qty int, tags ...string) deck.Card {
		return deck.Card{Qty: qty, Tags: tags, Card: mtg.Card{
			Name: name, TypeLine: tl, ManaCost: cost, CMC: cmc,
			Colors: colors, Rarity: rarity,
		}}
	}
	return []deck.Card{
		mk("Llanowar Elves", "Creature — Elf Druid", "{G}", 1, []string{"G"}, "common", 1, "ramp"),
		mk("Elvish Mystic", "Creature — Elf Druid", "{G}", 1, []string{"G"}, "common", 1, "ramp"),
		mk("Sol Ring", "Artifact", "{1}", 1, nil, "uncommon", 1, "ramp"),
		mk("Beast Within", "Instant", "{2}{G}", 3, []string{"G"}, "uncommon", 1, "removal"),
		mk("Craterhoof Behemoth", "Creature — Beast", "{5}{G}{G}{G}", 8, []string{"G"}, "mythic", 1, "wincon"),
		mk("Forest", "Basic Land — Forest", "", 0, nil, "common", 12),
	}
}

func TestSShowsTheHistograms(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	view := stripANSI(m.View())
	for _, want := range []string{"Tags", "ramp", "Type", "Creature", "Mana Value"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q is missing from the statistics:\n%s", want, view)
		}
	}
}

func TestWalkingTheBarsNarrowsTheList(t *testing.T) {
	// The bar you are standing on is exactly the cards in front of you —
	// which is what makes this a way of reading a deck rather than a report
	// about it.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s")

	if l.count() != 6 {
		t.Fatalf("the list starts at %d cards", l.count())
	}

	m = drive(m, "J") // the first category: tagged "ramp"
	if l.count() != 3 {
		t.Errorf("narrowing to ramp left %d cards, want 3", l.count())
	}
	for _, c := range l.rows {
		if len(c.Tags) == 0 || c.Tags[0] != "ramp" {
			t.Errorf("%s is not tagged ramp", c.Card.Name)
		}
	}
}

func TestTheBarsCountWhatTheListShows(t *testing.T) {
	// A category with nothing left in it stays put and reads zero, rather
	// than vanishing from under the cursor.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	before := len(statRows(m.stats.groups))
	m = drive(m, "J", "J", "J") // narrow to a small category
	after := len(statRows(m.stats.groups))

	if before != after {
		t.Errorf("the rows moved under the cursor: %d then %d", before, after)
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, " 0 ") {
		t.Errorf("no category read zero after narrowing:\n%s", view)
	}
}

func TestSteppingBackOffTheTopClearsTheNarrowing(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s", "J")
	if l.count() == 6 {
		t.Fatal("nothing was narrowed")
	}

	m = drive(m, "K")
	if l.count() != 6 {
		t.Errorf("stepping back off the top left %d cards", l.count())
	}
}

func TestLeavingStatisticsPutsTheListBack(t *testing.T) {
	// A narrowing you can no longer see the reason for is a bug you would
	// spend a while finding.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s", "J")
	m = drive(m, "s")

	if l.count() != 6 {
		t.Errorf("the list is still narrowed to %d cards", l.count())
	}
	if m.info.mode == infoStats {
		t.Error("s did not turn the statistics off")
	}
}

func TestEscClearsTheNarrowingBeforeAnythingElse(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s", "J")

	m = drive(m, "esc")
	if l.count() != 6 {
		t.Errorf("esc left the list narrowed to %d", l.count())
	}
	if m.ws.count() != 1 {
		t.Error("esc closed the panel instead of clearing the narrowing")
	}
}

func TestStatisticsAndTheTextFilterCompose(t *testing.T) {
	// Filter to a word, then narrow to a category: both, not either.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()

	m = drive(m, "/", "e", "l", "f", "enter")
	filtered := l.count()
	if filtered == 0 || filtered == 6 {
		t.Fatalf("the filter left %d cards", filtered)
	}

	m = drive(m, "s", "J")
	if l.count() > filtered {
		t.Errorf("narrowing widened the list: %d then %d", filtered, l.count())
	}
}

func TestGlobalStatisticsCountEveryList(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	m = withCards(m, "d", deckSample(), sortArrival)

	m = drive(m, "space", "s")
	if !m.stats.global {
		t.Fatal("space s did not go global")
	}

	counted, _ := m.statCards()
	if got := len(counted); got != len(sample())+len(deckSample()) {
		t.Errorf("counted %d cards, want both lists", got)
	}
}

func TestGlobalNarrowingReachesEveryList(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	search := m.ws.panels[0].cardsView()
	m = withCards(m, "d", deckSample(), sortArrival)
	target := m.ws.panels[1].cardsView()

	m = drive(m, "space", "s")
	m = drive(m, "J")

	if search.statFilter == nil || target.statFilter == nil {
		t.Error("the narrowing did not reach both lists")
	}
}

func TestTheBarsShareOneScale(t *testing.T) {
	// Scaling each group to its own widest bar would make a deck with two
	// of something look like a deck full of it.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	lines := m.renderStats(40)
	var lands, wincon string
	for _, line := range lines {
		plain := stripANSI(line)
		if strings.HasPrefix(plain, "Land") {
			lands = plain
		}
		if strings.HasPrefix(plain, "wincon") {
			wincon = plain
		}
	}
	if lands == "" || wincon == "" {
		t.Skip("those categories didn't appear")
	}
	if strings.Count(lands, "█") <= strings.Count(wincon, "█") {
		t.Errorf("twelve lands drew no bigger a bar than one wincon:\n%s\n%s", lands, wincon)
	}
}

func TestOneCardNeverReadsAsNone(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for _, line := range m.renderStats(40) {
		plain := stripANSI(line)
		if strings.HasPrefix(plain, "wincon") && !strings.Contains(plain, "█") {
			t.Errorf("a category with a card in it drew an empty bar: %q", plain)
		}
	}
}
