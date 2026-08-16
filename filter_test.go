package main

import (
	"strings"
	"testing"
)

func TestLiteralFilterIsNotFuzzy(t *testing.T) {
	targets := []string{
		"Dragonlord Ojutai Flying\nHexproof",  // 0
		"Goldspan Dragon Flying, haste",       // 1
		"Serra Angel Flying, vigilance",       // 2
		"Sol Ring {T}: Add {C}{C}.",           // 3
		"Plains ",                             // 4
		"Anguished Unmaking Destroy target …", // 5
	}

	indexes := func(term string) []int {
		var out []int
		for _, r := range literalFilter(term, targets) {
			out = append(out, r.Index)
		}
		return out
	}

	// The complaint this fixes: the fuzzy filter matched "flying" as a
	// subsequence — s…o…l across "Goldspan Dragon Flying" and so on — so
	// filtering a deck to a keyword returned most of the deck.
	got := indexes("flying")
	if len(got) != 3 {
		t.Errorf("flying matched %v, want the three cards that say it", got)
	}
	for _, i := range got {
		if !strings.Contains(strings.ToLower(targets[i]), "flying") {
			t.Errorf("flying matched %q, which doesn't contain it", targets[i])
		}
	}

	// A word nothing says matches nothing, rather than nearly everything.
	if got := indexes("artifact"); len(got) != 0 {
		t.Errorf("artifact matched %v, want nothing", got)
	}
	// The subsequence that used to match is no longer enough.
	if got := indexes("sdn"); len(got) != 0 {
		t.Errorf("a subsequence still matches: %v", got)
	}
}

func TestLiteralFilterTerms(t *testing.T) {
	targets := []string{
		"Serra Angel Flying, vigilance",
		"Goldspan Dragon Flying, haste",
	}
	indexes := func(term string) []int {
		var out []int
		for _, r := range literalFilter(term, targets) {
			out = append(out, r.Index)
		}
		return out
	}

	// Every term has to appear, so a second word narrows rather than widens.
	if got := indexes("flying"); len(got) != 2 {
		t.Errorf("flying = %v, want both", got)
	}
	if got := indexes("flying haste"); len(got) != 1 || got[0] != 1 {
		t.Errorf("flying haste = %v, want just the Dragon", got)
	}
	// Case doesn't matter.
	if got := indexes("FLYING"); len(got) != 2 {
		t.Errorf("FLYING = %v, want both", got)
	}
	// A quoted phrase is one term, spaces and all.
	if got := indexes(`"flying, haste"`); len(got) != 1 || got[0] != 1 {
		t.Errorf(`"flying, haste" = %v, want just the Dragon`, got)
	}
	if got := indexes(`"haste, flying"`); len(got) != 0 {
		t.Errorf("a quoted phrase matched out of order: %v", got)
	}
}

func TestLiteralFilterOrdersByWhereItMatched(t *testing.T) {
	// A card with the word in its name should come above one that only
	// mentions it deep in a rules paragraph.
	targets := []string{
		"Sol Ring Some long text that eventually says angel somewhere",
		"Serra Angel Flying",
	}
	ranks := literalFilter("angel", targets)
	if len(ranks) != 2 {
		t.Fatalf("got %d matches, want 2", len(ranks))
	}
	if ranks[0].Index != 1 {
		t.Errorf("the name match should come first, got index %d", ranks[0].Index)
	}
}

func TestFilterTerms(t *testing.T) {
	for in, want := range map[string]string{
		"flying":           "flying",
		"  flying  haste ": "flying|haste",
		`"first strike"`:   "first strike",
		`draw "a card"`:    "draw|a card",
		"":                 "",
		"   ":              "",
	} {
		got := strings.Join(filterTerms(in), "|")
		if got != want {
			t.Errorf("filterTerms(%q) = %q, want %q", in, got, want)
		}
	}
}
