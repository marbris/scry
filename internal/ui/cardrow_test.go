package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"
	"scry/internal/mtg"
)

func TestInitialismsFromTheDesign(t *testing.T) {
	// The three worked examples in the design doc, which between them cover
	// the awkward parts: an em dash between clauses, a hyphenated word, a
	// comma, and little words that must stay little.
	for in, want := range map[string]string{
		"Legendary Creature — Elf Faerie Noble": "LC-EFN",
	} {
		if got := initialism(in, false); got != want {
			t.Errorf("initialism(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Dwynen, Gilt-Leaf Daen":    "D,GLD.",
		"Miara, Thorn of the Glade": "M,TotG.",
	} {
		if got := initialism(in, true); got != want {
			t.Errorf("initialism(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInitialismKeepsCaseSoLittleWordsStayLittle(t *testing.T) {
	// "of the" becoming "OT" would read as two more nouns.
	if got := initialism("Sword of Feast and Famine", true); got != "SoFaF." {
		t.Errorf("got %q, want SoFaF.", got)
	}
}

func TestInitialismHandlesDoubleFacedNames(t *testing.T) {
	if got := initialism("Brutal Cathar // Moonrage Brute", true); got != "BC-MB." {
		t.Errorf("got %q, want BC-MB.", got)
	}
}

func TestManaCostLosesItsBraces(t *testing.T) {
	for in, want := range map[string]string{
		"{3}{B}{G}": "3BG",
		"{W}":       "W",
		"":          "",
		"{2}{W/U}":  "2W/U",
		"{X}{R}{R}": "XRR",
	} {
		got := manaCost(mtg.Card{ManaCost: in})
		if got != want {
			t.Errorf("manaCost(%q) = %q, want %q", in, got, want)
		}
	}
}

// row is the visible text of a rendered row, which is what the ladder is
// really about.
func row(c deck.Card, order cardSort, width int) string {
	return stripANSI(renderRow(c, order, rowState{}, width))
}

func TestTheRowIsExactlyAsWideAsItIsToldToBe(t *testing.T) {
	c := deck.Card{Card: mtg.Card{
		Name: "Dwynen, Gilt-Leaf Daen", TypeLine: "Legendary Creature — Elf Faerie Noble",
		ManaCost: "{2}{G}{G}",
	}}
	for _, order := range cardSorts {
		for width := 6; width <= 60; width++ {
			if got := textWidth(row(c, order, width)); got > width {
				t.Errorf("order %v at width %d rendered %d columns", order, width, got)
			}
		}
	}
}

func TestTheLadderGivesUpTheTypeLineBeforeTheName(t *testing.T) {
	c := deck.Card{Card: mtg.Card{
		Name:     "Dwynen, Gilt-Leaf Daen",
		TypeLine: "Legendary Creature — Elf Faerie Noble",
	}}

	// Wide: both in full.
	wide := row(c, sortType, 64)
	if !strings.Contains(wide, "Dwynen, Gilt-Leaf Daen") ||
		!strings.Contains(wide, "Legendary Creature — Elf Faerie Noble") {
		t.Errorf("at 64 columns the row should be unabbreviated:\n%q", wide)
	}

	// Narrower: the type line goes first, the name is still readable.
	mid := row(c, sortType, 34)
	if !strings.Contains(mid, "Dwynen, Gilt-Leaf Daen") {
		t.Errorf("the name was shortened before the type line:\n%q", mid)
	}
	if !strings.Contains(mid, "LC-EFN") {
		t.Errorf("the type line was not abbreviated:\n%q", mid)
	}

	// Narrower still: now the name goes too.
	narrow := row(c, sortType, 18)
	if !strings.Contains(narrow, "D,GLD.") {
		t.Errorf("the name was not abbreviated:\n%q", narrow)
	}
}

func TestAManaCostIsNeverAbbreviated(t *testing.T) {
	// A cost is already as short as it goes, and half a cost is a lie.
	c := deck.Card{Card: mtg.Card{Name: "Dwynen, Gilt-Leaf Daen", ManaCost: "{2}{G}{G}"}}
	for width := 12; width <= 40; width++ {
		if got := row(c, sortMana, width); !strings.Contains(got, "2GG") {
			t.Errorf("width %d lost the mana cost: %q", width, got)
		}
	}
}

func TestARankIsNeverAbbreviated(t *testing.T) {
	c := deck.Card{Card: mtg.Card{Name: "Dwynen, Gilt-Leaf Daen", EDHRECRank: 1234}}
	if got := row(c, sortEDHREC, 16); !strings.Contains(got, "1234") {
		t.Errorf("the rank was mangled: %q", got)
	}
}

func TestQuantityRidesWithTheName(t *testing.T) {
	c := deck.Card{Qty: 7, Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}}
	if got := row(c, sortMana, 30); !strings.Contains(got, "7x Plains") {
		t.Errorf("got %q, want the count in front of the name", got)
	}

	// A single copy says nothing, because every card in a singleton deck
	// would say 1x and none of them would be telling you anything.
	one := deck.Card{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}
	if got := row(one, sortMana, 30); strings.Contains(got, "1x") {
		t.Errorf("got %q, want no count for a single copy", got)
	}
}

func TestTheMarkerSaysOneThingAtATime(t *testing.T) {
	c := deck.Card{Card: mtg.Card{Name: "Sol Ring"}, Commander: true}

	if got, _ := marker(c, rowState{selected: true}); got != "▸" {
		t.Errorf("a selected commander shows %q, want the selection", got)
	}
	if got, _ := marker(c, rowState{}); got != "★" {
		t.Errorf("a commander shows %q", got)
	}
	plain := deck.Card{Card: mtg.Card{Name: "Sol Ring"}}
	if got, _ := marker(plain, rowState{member: true}); got != "•" {
		t.Errorf("a card in the editing deck shows %q", got)
	}
	if got, _ := marker(plain, rowState{}); got != " " {
		t.Errorf("an ordinary card shows %q, want nothing", got)
	}
}

func TestAVeryNarrowPanelStillShowsSomething(t *testing.T) {
	c := deck.Card{Card: mtg.Card{
		Name: "Miara, Thorn of the Glade", TypeLine: "Legendary Creature — Elf Scout",
	}}
	got := row(c, sortType, 8)
	if strings.TrimSpace(got) == "" {
		t.Error("an 8-column row came out blank")
	}
	if textWidth(got) > 8 {
		t.Errorf("%q is wider than 8", got)
	}
}

// deckCardNamed is a card with nothing but a name, for tests about layout
// rather than about cards.
func deckCardNamed(name string) deck.Card {
	return deck.Card{Card: mtg.Card{Name: name}}
}

func TestAnEmojiPresentationSequenceIsMeasuredAsTwoCells(t *testing.T) {
	// U+FE0F asks for the emoji form of the character before it, which
	// terminals draw double-width. runewidth measures the base alone and
	// says one, which overflows the row by a column.
	if got := textWidth("♟️"); got != 2 {
		t.Errorf("width of a chess pawn with the emoji selector is %d, want 2", got)
	}
	if got := textWidth("👑"); got != 2 {
		t.Errorf("width of a crown is %d, want 2", got)
	}
	if got := textWidth("abc"); got != 3 {
		t.Errorf("plain text measured %d", got)
	}
}

func TestTruncateAgreesWithTextWidth(t *testing.T) {
	// If the two use different measures, a row is cut to a width that then
	// measures as something else and the line wraps.
	for _, s := range []string{
		"👑-Marchesa d'Amati, First of her Name: Queens Gambit♟️",
		"Dwynen, Gilt-Leaf Daen",
		"日本語のカード名",
	} {
		for width := 1; width <= 40; width++ {
			if got := textWidth(truncate(s, width)); got > width {
				t.Errorf("truncate(%q, %d) measures %d", s, width, got)
			}
		}
	}
}
