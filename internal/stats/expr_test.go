package stats

import (
	"math"
	"testing"

	"scry/internal/deck"
	"scry/internal/mtg"
)

func find(t *testing.T, rows []Row, label string) Row {
	t.Helper()
	for _, r := range rows {
		if r.Label == label {
			return r
		}
	}
	t.Fatalf("no %q row", label)
	return Row{}
}

func TestExprFoldsFromTheLeftInTheOrderItWasBuilt(t *testing.T) {
	// The example from the notes: a creature, a black, an artifact (or), a
	// mana value of five — artifacts and black creatures, all costing five.
	creature := find(t, typeRows(), "Creature")
	artifact := find(t, typeRows(), "Artifact")
	black := find(t, colorRows(), "Black")
	five := find(t, cmcRows(), "5")

	var e Expr
	e, _ = e.Add(And, creature)
	e, _ = e.Add(And, black)
	e, _ = e.Add(Or, artifact)
	e, _ = e.Add(And, five)

	if got, want := e.String(), "((Creature ∧ Black) ∨ Artifact) ∧ 5"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}

	card := func(tl string, cmc float64, colors ...string) deck.Card {
		return deck.Card{Card: mtg.Card{Name: tl, TypeLine: tl, CMC: cmc, Colors: colors}, Qty: 1}
	}
	cases := []struct {
		c    deck.Card
		want bool
	}{
		{card("Creature — Horror", 5, "B"), true},
		{card("Creature — Horror", 4, "B"), false},
		{card("Creature — Angel", 5, "W"), false},
		{card("Artifact", 5), true},
		{card("Artifact", 3), false},
		{card("Enchantment", 5, "B"), false},
	}
	for _, c := range cases {
		if got := e.Match(c.c); got != c.want {
			t.Errorf("%s cmc %v %v: match = %v, want %v", c.c.Card.TypeLine, c.c.Card.CMC, c.c.Card.Colors, got, c.want)
		}
	}
}

func TestExprFirstOpDoesntMatterAndDuplicatesArentAdded(t *testing.T) {
	black := find(t, colorRows(), "Black")
	var a, o Expr
	a, _ = a.Add(And, black)
	o, _ = o.Add(Or, black)
	c := deck.Card{Card: mtg.Card{TypeLine: "Instant", Colors: []string{"U"}}}
	if a.Match(c) != o.Match(c) {
		t.Error("the first clause's op changed the result")
	}
	if _, added := a.Add(Or, black); added {
		t.Error("the same category was added twice")
	}
}

func TestWithoutDropsOnlyThatCategory(t *testing.T) {
	var e Expr
	e, _ = e.Add(And, find(t, cmcRows(), "1"))
	e, _ = e.Add(Or, find(t, cmcRows(), "2"))
	e, _ = e.Add(And, find(t, typeRows(), "Creature"))
	e = e.Without(find(t, cmcRows(), "1"))
	if got := e.String(); got != "2 ∧ Creature" {
		t.Errorf("after dropping 1: %q", got)
	}
}

func TestEmptyExprMatchesEverything(t *testing.T) {
	if !(Expr{}).Match(deck.Card{}) {
		t.Error("an empty narrowing narrowed")
	}
}

func TestAtLeast(t *testing.T) {
	near := func(got, want float64) bool { return math.Abs(got-want) < 0.001 }

	// 36 lands in 99: almost always one in seven.
	if got := AtLeast(99, 36, 1, HandSize); !near(got, 0.9628) {
		t.Errorf("P(≥1 of 36/99) = %.4f", got)
	}
	// A single copy in 60: 7/60.
	if got := AtLeast(60, 1, 1, HandSize); !near(got, 7.0/60) {
		t.Errorf("P(≥1 of 1/60) = %.4f", got)
	}
	// Four copies in 60, at least two: 1 - P(0) - P(1) = 0.0632.
	if got := AtLeast(60, 4, 2, HandSize); !near(got, 0.0632) {
		t.Errorf("P(≥2 of 4/60) = %.4f", got)
	}
	if got := AtLeast(60, 1, 2, HandSize); got != 0 {
		t.Errorf("two of a single copy = %v", got)
	}
	if got := AtLeast(5, 5, 4, HandSize); !near(got, 1) {
		t.Errorf("whole deck = %v", got)
	}
}
