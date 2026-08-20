package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"
	"scry/internal/mtg"
	"scry/internal/stats"
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

	before := len(statRows(m.statGroups()))
	m = drive(m, "J", "J", "J") // narrow to a small category
	after := len(statRows(m.statGroups()))

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

func TestThePanelScrollsToKeepTheCategoryInView(t *testing.T) {
	// J past the bottom used to move a cursor you could no longer see.
	m := withCards(sized(90, 16), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	const room = 8
	if m.statOffset(room) != 0 {
		t.Fatalf("started scrolled to %d", m.statOffset(room))
	}
	for i := 0; i < 12; i++ {
		m = drive(m, "J")
	}
	if m.statOffset(room) == 0 {
		t.Error("walking down twelve categories never scrolled")
	}

	// And the highlighted category is inside the window.
	line := statLine(m.statGroups(), m.stats.row)
	at := m.statOffset(room)
	if line < at || line >= at+room {
		t.Errorf("category on line %d, showing %d..%d", line, at, at+room)
	}
}

func TestScrollingComesBackUpAgain(t *testing.T) {
	m := withCards(sized(90, 16), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for i := 0; i < 12; i++ {
		m = drive(m, "J")
	}
	for i := 0; i < 12; i++ {
		m = drive(m, "K")
	}
	if got := m.statOffset(8); got != 0 {
		t.Errorf("came back to the top still scrolled to %d", got)
	}
}

func TestCtrlJMovesAWholeGroup(t *testing.T) {
	// Five groups of a dozen rows is a lot of J to reach the curve.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	starts := groupStarts(m.statGroups())
	if len(starts) < 3 {
		t.Skipf("only %d groups", len(starts))
	}

	m = drive(m, "ctrl+j")
	if m.stats.row != starts[0] {
		t.Errorf("landed on row %d, want the first group at %d", m.stats.row, starts[0])
	}
	m = drive(m, "ctrl+j")
	if m.stats.row != starts[1] {
		t.Errorf("landed on row %d, want the second group at %d", m.stats.row, starts[1])
	}
}

func TestCtrlKComesBackAGroupAtATime(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	starts := groupStarts(m.statGroups())
	if len(starts) < 2 {
		t.Skip("not enough groups")
	}

	m = drive(m, "ctrl+j", "ctrl+j")
	m = drive(m, "ctrl+k")
	if m.stats.row != starts[0] {
		t.Errorf("landed on row %d, want %d", m.stats.row, starts[0])
	}

	// Past the first group is the whole list, the same place K off the top
	// lands.
	m = drive(m, "ctrl+k")
	if m.stats.row != -1 {
		t.Errorf("row is %d, want the narrowing cleared", m.stats.row)
	}
}

func TestJumpingByGroupNarrowsTheListToo(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s")

	before := l.count()
	m = drive(m, "ctrl+j")
	if l.count() == before {
		t.Error("jumping to a category did not narrow the list")
	}
}

func TestStatLineCountsHeadingsAndGaps(t *testing.T) {
	// The panel scrolls by rendered lines, not by category, so this has to
	// agree with what renderStats draws.
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	groups := m.statGroups()
	lines := m.renderStats(30)
	rows := statRows(groups)

	for row := 0; row < len(rows); row++ {
		at := statLine(groups, row)
		if at >= len(lines) {
			t.Fatalf("category %d maps to line %d of %d", row, at, len(lines))
		}
		if !strings.Contains(stripANSI(lines[at]), rows[row].Label) {
			t.Errorf("category %d (%s) maps to line %d, which says %q",
				row, rows[row].Label, at, stripANSI(lines[at]))
		}
	}
}

// searchSample is what a Scryfall search puts in a list: cards with no
// quantity, because nobody has chosen how many.
func searchSample() []deck.Card {
	var out []deck.Card
	for _, c := range deckSample() {
		c.Qty, c.Tags = 0, nil
		out = append(out, c)
	}
	return out
}

func TestSearchResultsHaveStatisticsToo(t *testing.T) {
	// They counted to nothing, because a search result has no quantity and
	// the bars summed quantities — so every row's base was zero, every row
	// was dropped, and the panel said "nothing to count" over a full screen.
	m := withCards(sized(120, 30), "f", searchSample(), sortArrival)
	m = drive(m, "s")

	groups := m.statGroups()
	if len(groups) == 0 {
		t.Fatal("a search of six cards produced no statistics at all")
	}
	body := strings.Join(m.renderStats(60), "\n")
	if strings.Contains(stripANSI(body), "nothing to count") {
		t.Error(`the panel still says "nothing to count"`)
	}

	// Three creatures among the six, counted one apiece.
	var creatures *stats.Row
	for i, r := range statRows(groups) {
		if r.Group == "Type" && r.Label == "Creature" {
			creatures = &statRows(groups)[i]
		}
	}
	if creatures == nil {
		t.Fatal("no Creature row")
	}
	if creatures.Base != 3 || creatures.Count != 3 {
		t.Errorf("Creature counted %d of %d, want 3 of 3", creatures.Count, creatures.Base)
	}
}

func TestADeckStillCountsItsCopies(t *testing.T) {
	// The floor at one must not flatten a real deck's twelve Forests.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for _, r := range statRows(m.statGroups()) {
		if r.Group == "Type" && r.Label == "Land" {
			if r.Base != 12 {
				t.Errorf("twelve Forests counted as %d", r.Base)
			}
			return
		}
	}
	t.Fatal("no Land row")
}
