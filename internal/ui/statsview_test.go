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

// statGroupTitles is the groups in the order they're drawn.
func statGroupTitles(m Model) []string {
	var out []string
	for _, g := range m.statGroups() {
		out = append(out, g.Title)
	}
	return out
}

// pointStat puts the highlight on a category by name.
func pointStat(t *testing.T, m Model, group, label string) Model {
	t.Helper()
	for _, r := range statRows(m.statGroups()) {
		if r.Group == group && r.Label == label {
			m.pointAt(r)
			return m
		}
	}
	t.Fatalf("no %s %s row", group, label)
	return m
}

func TestWalkingTheBarsDoesntNarrow(t *testing.T) {
	// Reading the bars and filtering by them are separate acts: you can walk
	// every category without the list moving under you.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s", "j", "j", "j")

	if l.count() != 6 {
		t.Errorf("walking the bars narrowed the list to %d", l.count())
	}
	if c, _ := l.current(); c.Card.Name != "Llanowar Elves" {
		t.Errorf("j moved the list's cursor to %s, not the bars'", c.Card.Name)
	}
	if m.statCursor(m.statGroups()) != 3 {
		t.Errorf("the highlight is on row %d, want 3", m.statCursor(m.statGroups()))
	}
}

func TestSGoesBackToTheList(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s", "a") // narrow to the first category: tagged ramp
	narrowed := l.count()
	if narrowed != 3 {
		t.Fatalf("adding ramp left %d cards, want 3", narrowed)
	}

	m = drive(m, "s")
	if m.info.mode == infoStats {
		t.Fatal("s did not close the statistics")
	}
	if l.count() != narrowed {
		t.Errorf("closing the statistics undid the filter: %d cards", l.count())
	}
	// The keys are the list's again.
	m = drive(m, "j")
	if c, _ := l.current(); c.Card.Name == l.rows[0].Card.Name {
		t.Error("j didn't move the list after leaving the statistics")
	}
}

func TestShiftJAndKTurnTheGroupsOver(t *testing.T) {
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	full := []string{"Tags", "Type", "Color (excl. lands)", "Mana Value (excl. lands)", "Rarity", "Price (USD)"}
	have := statGroupTitles(m)
	// The sample has no prices, so that group is absent; the order holds.
	want := []string{}
	for _, t := range full {
		for _, h := range have {
			if h == t {
				want = append(want, t)
			}
		}
	}
	if strings.Join(have, ",") != strings.Join(want, ",") {
		t.Fatalf("opened in the order %v", have)
	}

	m = drive(m, "J")
	if got := statGroupTitles(m); strings.Join(got, ",") != strings.Join(append(want[1:], want[0]), ",") {
		t.Errorf("J: %v", got)
	}
	m = drive(m, "J")
	if got := statGroupTitles(m); strings.Join(got, ",") != strings.Join(append(want[2:], want[:2]...), ",") {
		t.Errorf("J J: %v", got)
	}
	m = drive(m, "K", "K", "K")
	last := len(want) - 1
	if got := statGroupTitles(m); strings.Join(got, ",") != strings.Join(append([]string{want[last]}, want[:last]...), ",") {
		t.Errorf("K from the start: %v", got)
	}
	// The highlight goes to the new top.
	if row, _ := m.statUnder(); row.Group != statRows(m.statGroups())[0].Group {
		t.Errorf("highlight on %s %s, not the top group", row.Group, row.Label)
	}
}

func TestAddingCategoriesFoldsAndAndOr(t *testing.T) {
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s")

	m = pointStat(t, m, "Type", "Creature")
	m = drive(m, "a")
	if l.count() != 3 {
		t.Fatalf("creatures: %d", l.count())
	}
	m = pointStat(t, m, "Mana Value", "1")
	m = drive(m, "a")
	if l.count() != 2 {
		t.Errorf("creatures ∧ 1: %d, want the two elves", l.count())
	}
	m = pointStat(t, m, "Type", "Artifact")
	m = drive(m, "o")
	if l.count() != 3 {
		t.Errorf("(creatures ∧ 1) ∨ artifact: %d, want the elves and the Sol Ring", l.count())
	}
	if got := l.statFilter.String(); got != "(Creature ∧ 1) ∨ Artifact" {
		t.Errorf("filter reads %q", got)
	}
	if !strings.Contains(stripANSI(m.View()), "(Creature ∧ 1) ∨ Artifact") {
		t.Error("the filter isn't shown")
	}

	// x takes out the highlighted category alone: Artifact goes, Creature
	// from the same group stays.
	m = drive(m, "x")
	if got := l.statFilter.String(); got != "Creature ∧ 1" {
		t.Errorf("after x on Artifact: %q", got)
	}
	if l.count() != 2 {
		t.Errorf("creatures ∧ 1: %d cards", l.count())
	}
	// b clears the rest.
	m = drive(m, "b")
	if len(l.statFilter) != 0 || l.count() != 6 {
		t.Errorf("b left %q, %d cards", l.statFilter.String(), l.count())
	}
}

func TestEscStepsBackAFilterAtATime(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "/", "e", "l", "enter", "s", "a")
	if l.filter == "" || len(l.statFilter) == 0 {
		t.Fatal("not narrowed both ways")
	}

	m = drive(m, "esc")
	if len(l.statFilter) != 0 || l.filter == "" {
		t.Error("the first esc should take the categories and leave the text filter")
	}
	m = drive(m, "esc")
	if l.filter != "" || m.info.mode != infoStats {
		t.Error("the second esc should take the text filter and stay in the statistics")
	}
	m = drive(m, "esc")
	if m.info.mode == infoStats {
		t.Error("the third esc should leave the statistics")
	}
	if m.ws.count() != 1 {
		t.Error("esc closed the panel")
	}
}

func TestPStepsThroughTheOdds(t *testing.T) {
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s", "p")
	if m.stats.odds != 1 {
		t.Fatalf("odds = %d", m.stats.odds)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "P(≥1 in opening 7)") || !strings.Contains(view, "%") {
		t.Errorf("no odds on show:\n%s", view)
	}
	m = drive(m, "p", "p", "p", "p")
	if m.stats.odds != 0 {
		t.Errorf("five presses left odds at %d, want back to counts", m.stats.odds)
	}
	m = drive(m, "P")
	if m.stats.odds != 4 {
		t.Errorf("P from counts: %d, want 4", m.stats.odds)
	}
}

func TestTheOddsAreForTheWholeList(t *testing.T) {
	// Twelve Forests in eighteen cards, seven drawn: all but certain.
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s", "p")
	for _, line := range m.renderStats(60) {
		plain := stripANSI(line)
		if strings.Contains(plain, "Land ") && !strings.Contains(plain, "100%") {
			t.Errorf("land odds: %q", plain)
		}
	}
}

func TestStatisticsFollowTheFocusedList(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	m = withCards(m, "d", deckSample(), sortArrival)
	m = drive(m, "s")

	counted, _ := m.statCards()
	if len(counted) != len(deckSample()) {
		t.Fatalf("counting %d", len(counted))
	}
	m = drive(m, "h")
	if m.info.mode != infoStats {
		t.Fatal("h left the statistics")
	}
	counted, _ = m.statCards()
	if len(counted) != len(sample()) {
		t.Errorf("after h the statistics count %d, want the search's %d", len(counted), len(sample()))
	}
}

func TestSpaceSCountsTheEditingDeck(t *testing.T) {
	m, search, target := editing(t)
	m = focusOn(m, 0)
	m = drive(m, "space", "s")
	if !m.stats.editing {
		t.Fatal("space s did not go to the editing deck")
	}
	counted, _ := m.statCards()
	if len(counted) != len(target.all) {
		t.Errorf("counted %d, want the editing deck's %d", len(counted), len(target.all))
	}
	m = drive(m, "a")
	if len(target.statFilter) == 0 || len(search.statFilter) != 0 {
		t.Error("the filter should land on the editing deck alone")
	}
}

func TestSpaceBClearsEveryList(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	search := m.ws.panels[0].cardsView()
	m = drive(m, "s", "a", "s")
	m = withCards(m, "d", deckSample(), sortArrival)
	target := m.ws.panels[1].cardsView()
	m = drive(m, "s", "a", "s")
	if len(search.statFilter) == 0 || len(target.statFilter) == 0 {
		t.Fatal("not both narrowed")
	}
	m = drive(m, "space", "b")
	if len(search.statFilter) != 0 || len(target.statFilter) != 0 {
		t.Error("space b left a list narrowed")
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
	// j past the bottom used to move a cursor you could no longer see.
	m := withCards(sized(90, 16), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	const room = 8
	if m.statOffset(room) != 0 {
		t.Fatalf("started scrolled to %d", m.statOffset(room))
	}
	for i := 0; i < 12; i++ {
		m = drive(m, "j")
	}
	if m.statOffset(room) == 0 {
		t.Error("walking down twelve categories never scrolled")
	}

	// And the highlighted category is inside the window.
	line := statLine(m.statGroups(), m.statCursor(m.statGroups()))
	at := m.statOffset(room)
	if line < at || line >= at+room {
		t.Errorf("category on line %d, showing %d..%d", line, at, at+room)
	}
}

func TestScrollingComesBackUpAgain(t *testing.T) {
	m := withCards(sized(90, 16), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for i := 0; i < 12; i++ {
		m = drive(m, "j")
	}
	for i := 0; i < 12; i++ {
		m = drive(m, "k")
	}
	if got := m.statOffset(8); got != 0 {
		t.Errorf("came back to the top still scrolled to %d", got)
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
