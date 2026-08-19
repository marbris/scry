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
