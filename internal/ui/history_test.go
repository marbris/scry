package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"
	"scry/internal/mtg"
	"scry/internal/prints"
)

func historyCard() mtg.Card {
	return mtg.Card{
		ID: "x", OracleID: "oid", PrintsSearchURI: "https://example.invalid/prints",
		Name: "Test Bird", TypeLine: "Creature — Bird", OracleText: "Flying",
	}
}

func TestGVOnACardOpensItsPrintedText(t *testing.T) {
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")

	if m.info.mode != infoVersions {
		t.Fatalf("gv left the panel in mode %v", m.info.mode)
	}
	if _, ok := m.histories["oid"]; !ok {
		t.Error("nothing was started for this card")
	}
	if !strings.Contains(stripANSI(m.View()), "printed text") {
		t.Error("the panel does not say what it is showing")
	}
}

func TestACardWithNoPrintingsLinkSaysSo(t *testing.T) {
	card := historyCard()
	card.PrintsSearchURI = ""
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: card}}, sortArrival)
	m = drive(m, "g", "v")

	if !strings.Contains(stripANSI(m.View()), "no printing history") {
		t.Errorf("got:\n%s", stripANSI(m.View()))
	}
}

func TestCachedSetsAreShownWithoutAskingAndTheRestAreOffered(t *testing.T) {
	// Forty printings is forty multi-megabyte downloads. What is already on
	// disk costs nothing and is taken; the rest is offered.
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")

	next, _ := m.Update(printingsMsg{card: "oid", printings: []prints.Printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
		{Name: "Test Bird", Set: "BBB", SetName: "Beta", Released: "1994-04-11"},
	}})
	m = next.(Model)

	h := m.histories["oid"]
	if h.state != histWaiting {
		t.Fatalf("state is %v, want it waiting for the go-ahead", h.state)
	}
	if len(h.missing) != 2 {
		t.Errorf("%d sets are missing, want both", len(h.missing))
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, "not downloaded") || !strings.Contains(view, "y to fetch") {
		t.Errorf("the offer is not on screen:\n%s", view)
	}
}

func TestYIsTheGoAheadRatherThanAYank(t *testing.T) {
	// A card list takes y for yank, so the printed-text panel has to claim
	// it first or the offer could never be accepted.
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")
	next, _ := m.Update(printingsMsg{card: "oid", printings: []prints.Printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha"},
	}})
	m = next.(Model)

	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)

	if cmd == nil {
		t.Error("y did not start the download")
	}
	if len(m.register) > 0 {
		t.Error("y yanked instead of answering the question")
	}
	if m.histories["oid"].state != histFetching {
		t.Errorf("state is %v", m.histories["oid"].state)
	}
}

func TestRevisionsCollapseIdenticalWordings(t *testing.T) {
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")

	next, _ := m.Update(printingsMsg{card: "oid", printings: []prints.Printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
		{Name: "Test Bird", Set: "BBB", SetName: "Beta", Released: "1994-04-11"},
	}})
	m = next.(Model)

	h := m.histories["oid"]
	h.missing = nil // pretend the go-ahead was given
	for _, s := range []struct{ set, text string }{
		{"AAA", "Does not tap when attacking."},
		{"BBB", "Flying"},
	} {
		next, _ = m.Update(setTextMsg{card: "oid", set: s.set,
			cards: map[string]string{"test bird": s.text}})
		m = next.(Model)
	}

	if len(h.revisions) < 2 {
		t.Fatalf("got %d revisions, want the wording to have changed", len(h.revisions))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Alpha") {
		t.Errorf("the first printing is not named:\n%s", view)
	}
}

func TestADeckVersionShowsWhatItChanged(t *testing.T) {
	// A commit subject says "+3 cards"; the diff says which three.
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	base, _ := deck.Read("ghen")
	deck.SaveVersioned("ghen", base)
	changed, _ := deck.ParseFile(strings.NewReader(
		"name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n1 Llanowar Elves\n"))
	deck.SaveVersioned("ghen", changed)

	m := withCards(sized(140, 30), "d", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}}, sortArrival)
	l := m.ws.current().cardsView()
	l.deck = &deck.Info{Name: "Ghen", Slug: "ghen"}

	next, cmd := m.Update(loadVersions(m.ws.current().id, "ghen", "Ghen")())
	m = next.(Model)
	if cmd == nil {
		t.Fatal("the version list did not ask for a diff")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	view := stripANSI(m.View())
	if !strings.Contains(view, "+1 Llanowar Elves") {
		t.Errorf("the diff does not say what changed:\n%s", view)
	}
}

func TestTheDiffLeavesOutGitsOwnBookkeeping(t *testing.T) {
	// There is one file and you know which; the headers are noise.
	got := strings.Join(renderDiff(strings.Join([]string{
		"diff --git a/ghen.deck b/ghen.deck",
		"index 1234567..89abcde 100644",
		"--- a/ghen.deck",
		"+++ b/ghen.deck",
		"@@ -1,4 +1,5 @@",
		" [mainboard]",
		"+1 Llanowar Elves",
		"-1 Sol Ring",
	}, "\n"), 40), "\n")
	got = stripANSI(got)

	for _, unwanted := range []string{"diff --git", "index ", "@@", "+++", "---"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q survived:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "+1 Llanowar Elves") || !strings.Contains(got, "-1 Sol Ring") {
		t.Errorf("the changes were lost:\n%s", got)
	}
}
