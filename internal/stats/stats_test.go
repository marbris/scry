package stats

import (
	"strings"
	"testing"

	"scry/internal/deck"
	"scry/internal/mtg"
)

func TestLandsAreOutOfTheCurveAndTheColours(t *testing.T) {
	// A deck is mostly lands, and nearly all of them cost nothing and count
	// as colourless. Left in, they put a column at zero taller than the rest
	// of the curve put together and a "Colorless" bar that reads as if the
	// deck were full of colourless spells. The type breakdown already says
	// how many lands there are, and says it better.
	entries := []deck.Card{
		deck.Card{Card: mtg.Card{Name: "Command Tower", TypeLine: "Land", CMC: 0}, Qty: 1},
		deck.Card{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains", CMC: 0}, Qty: 30},
		deck.Card{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact", CMC: 1}, Qty: 1},
		deck.Card{Card: mtg.Card{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, Qty: 1},
	}

	count := func(rows []Row, label string) int {
		for _, r := range rows {
			if r.Label != label {
				continue
			}
			var n int
			for _, ci := range entries {
				if r.Match(ci) {
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

func TestUntaggedIsTheLeftoverAndOnlyWhenSomethingIsTagged(t *testing.T) {
	// A histogram whose one bar is "untagged" says nothing, so a deck with no
	// tags has no tag group; once anything is tagged, the untagged remainder
	// earns a bar — and it sits below every real tag, however big it is.
	tagsGroup := func(entries []deck.Card) *Group {
		for _, g := range Groups(entries, entries) {
			if g.Title == "Tags" {
				return &g
			}
		}
		return nil
	}

	// Nothing tagged: no tag group at all, not a lone untagged bar.
	none := []deck.Card{
		{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}, Qty: 1},
		{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 5},
	}
	if g := tagsGroup(none); g != nil {
		t.Errorf("a deck with no tags still has a tag group: %+v", g.Rows)
	}

	// Some tagged, some not: a bar for the tag and one for the remainder, with
	// untagged last even though it holds more cards.
	some := []deck.Card{
		{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}, Qty: 1, Tags: []string{"ramp"}},
		{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 20},
	}
	g := tagsGroup(some)
	if g == nil {
		t.Fatal("no tag group though a card is tagged")
	}
	if len(g.Rows) != 2 {
		t.Fatalf("want a tag row and an untagged row, got %+v", g.Rows)
	}
	if g.Rows[len(g.Rows)-1].Label != untaggedLabel {
		t.Errorf("untagged is not last: %+v", g.Rows)
	}
}

func TestStatGroupsSayTheyExcludeLands(t *testing.T) {
	// The reader has to be told, or the numbers look wrong.
	entries := []deck.Card{
		{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 30},
		{Card: mtg.Card{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, Qty: 1},
	}

	var titles []string
	for _, g := range Groups(entries, entries) {
		titles = append(titles, g.Title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Mana Value (excl. lands)", "Color (excl. lands)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no group titled %q; titles are %s", want, joined)
		}
	}
}
