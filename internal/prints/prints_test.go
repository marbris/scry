package prints

import (
	"testing"

	"scry/internal/mtg"
)

// ── Printed-text history ────────────────────────────────────────

func TestCleanOriginal(t *testing.T) {
	cases := map[string]string{
		"ocT: Add {G} to your mana pool.":       "{T}: Add {G} to your mana pool.",
		"Flying// Tap to add one mana.":         "Flying\nTap to add one mana.",
		"Assault deals 2 damage. // Create it.": "Assault deals 2 damage.\nCreate it.",
		"  Flying  ":                            "Flying",
	}
	for in, want := range cases {
		if got := cleanOriginal(in); got != want {
			t.Errorf("cleanOriginal(%q) = %q, want %q", in, got, want)
		}
	}
}

// The history lists wordings, not printings: twenty reprints with the same
// text are one entry, but a wording that comes back later is its own.
func TestBuildRevisionsCollapsesIdenticalPrintings(t *testing.T) {
	card := mtg.Card{Name: "Test Bird", OracleText: "Flying\n{T}: Add one mana of any color."}
	printings := []Printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha Set", Released: "1993-08-05"},
		{Name: "Test Bird", Set: "BBB", SetName: "Beta Set", Released: "1993-10-04"},
		{Name: "Test Bird", Set: "CCC", SetName: "Third Set", Released: "1994-04-11"},
		{Name: "Test Bird", Set: "DDD", SetName: "Fourth Set", Released: "1995-04-01"},
		{Name: "Test Bird", Set: "EEE", SetName: "Fifth Set", Released: "2019-01-25"},
	}
	originals := map[string]map[string]string{
		"AAA": {"test bird": "Flying\nocT: Add one mana of any color to your mana pool."},
		"BBB": {"test bird": "Flying\n{T}: Add one mana of any color to your mana pool."}, // same but for the symbol encoding
		"CCC": {"test bird": "Flying (Reminder.)\n{T}: Add one mana of any color to your mana pool."},
		"DDD": {"test bird": "Flying\n{T}: Add one mana of any color to your mana pool."}, // back to the earlier wording
		"EEE": {"test bird": "Flying\n{T}: Add one mana of any color."},
	}

	revs := BuildRevisions(card, printings, originals)
	if len(revs) != 4 {
		for _, r := range revs {
			t.Logf("  %s (%d printings): %q", r.SetCode, r.Printings, r.Text)
		}
		t.Fatalf("got %d wordings, want 4", len(revs))
	}

	if revs[0].SetCode != "AAA" || revs[0].Printings != 2 {
		t.Errorf("first wording = %s covering %d printings, want AAA covering 2",
			revs[0].SetCode, revs[0].Printings)
	}
	if revs[1].SetCode != "CCC" {
		t.Errorf("second wording = %s, want CCC", revs[1].SetCode)
	}
	if revs[2].SetCode != "DDD" {
		t.Errorf("third wording = %s, want DDD (the wording returned)", revs[2].SetCode)
	}
	if !revs[3].Current || revs[3].SetCode != "EEE" {
		t.Errorf("last wording = %s current=%v, want EEE marked current",
			revs[3].SetCode, revs[3].Current)
	}
}

// When no printing carries the current oracle wording it's still shown last.
func TestBuildRevisionsAppendsCurrentOracle(t *testing.T) {
	card := mtg.Card{Name: "Test Bird", OracleText: "Flying"}
	printings := []Printing{{Name: "Test Bird", Set: "AAA", SetName: "Alpha Set", Released: "1993-08-05"}}
	originals := map[string]map[string]string{"AAA": {"test bird": "Does not tap when attacking."}}

	revs := BuildRevisions(card, printings, originals)
	if len(revs) != 2 {
		t.Fatalf("got %d wordings, want 2", len(revs))
	}
	if !revs[1].Current || revs[1].Text != "Flying" {
		t.Errorf("last entry = %+v, want the current oracle text", revs[1])
	}
}

// Nothing is downloaded just by moving the cursor — only the key does that.
