package moxfield

import (
	"strings"
	"testing"
)

// moxDeck builds the shape Moxfield sends back.
func moxDeck(name string, commanders, mainboard map[string]int, tags map[string][]string) Deck {
	board := func(cards map[string]int) Board {
		out := Board{Cards: map[string]Entry{}}
		i := 0
		for n, q := range cards {
			e := Entry{Quantity: q}
			e.Card.Name = n
			out.Cards[string(rune('a'+i))] = e
			i++
		}
		return out
	}
	return Deck{
		Name: name, Format: "commander", PublicURL: "https://moxfield.com/decks/Y8dZ7",
		Boards: map[string]Board{
			"commanders": board(commanders),
			"mainboard":  board(mainboard),
		},
		AuthorTags: tags,
	}
}

func TestImportPutsTheCommanderInTheCommandZone(t *testing.T) {
	d, err := ToFile(moxDeck("Ghen",
		map[string]int{"Ghen, Arcanum Weaver": 1},
		map[string]int{"Sol Ring": 1, "Plains": 30},
		nil), "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}

	body := d.String()
	if !strings.Contains(body, "[commander]") {
		t.Fatalf("no command zone:\n%s", body)
	}
	commanders := d.Section("commander")
	if len(commanders) != 1 || commanders[0].Name != "Ghen, Arcanum Weaver" {
		t.Errorf("the command zone holds %v", commanders)
	}
}

func TestImportKeepsQuantities(t *testing.T) {
	d, err := ToFile(moxDeck("Lands",
		map[string]int{"Ghen, Arcanum Weaver": 1},
		map[string]int{"Plains": 30}, nil), "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.String(), "30 Plains") {
		t.Errorf("got:\n%s", d.String())
	}
}

func TestImportBringsTheAuthorsTags(t *testing.T) {
	// Somebody else's reasons for a card are the most useful thing about
	// importing their deck, and the part no other source has.
	d, err := ToFile(moxDeck("Ghen",
		map[string]int{"Ghen, Arcanum Weaver": 1},
		map[string]int{"Sol Ring": 1},
		map[string][]string{"Sol Ring": {"ramp", "fast mana"}}), "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}

	body := d.String()
	if !strings.Contains(body, "ramp") || !strings.Contains(body, "fast mana") {
		t.Errorf("the tags did not come across:\n%s", body)
	}
}

func TestAQuantityOfZeroBecomesOne(t *testing.T) {
	// Moxfield occasionally sends one, and a deck line saying "0 Sol Ring"
	// is a card that silently isn't there.
	d, err := ToFile(moxDeck("Ghen",
		map[string]int{"Ghen, Arcanum Weaver": 0},
		map[string]int{"Sol Ring": 0}, nil), "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range d.Entries {
		if e.Qty < 1 {
			t.Errorf("%s came in with quantity %d", e.Name, e.Qty)
		}
	}
}

func TestACardInTwoBoardsIsImportedOnce(t *testing.T) {
	// A commander listed in the mainboard as well shouldn't appear twice,
	// and the command zone wins because it is read first.
	d, err := ToFile(moxDeck("Ghen",
		map[string]int{"Ghen, Arcanum Weaver": 1},
		map[string]int{"Ghen, Arcanum Weaver": 1, "Sol Ring": 1}, nil), "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, e := range d.Entries {
		if e.Name == "Ghen, Arcanum Weaver" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the commander appears %d times", count)
	}
	if len(d.Section("commander")) != 1 {
		t.Error("the command zone lost it")
	}
}

func TestAnEmptyDeckIsAnError(t *testing.T) {
	if _, err := ToFile(moxDeck("Nothing", nil, nil, nil), "Y8dZ7"); err == nil {
		t.Error("a deck with no cards imported cleanly")
	}
}

func TestTheSourceIsRecordedSoTheDeckCanBeSyncedAgain(t *testing.T) {
	d, err := ToFile(moxDeck("Ghen",
		map[string]int{"Ghen, Arcanum Weaver": 1},
		map[string]int{"Sol Ring": 1}, nil), "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Source, "Y8dZ7") {
		t.Errorf("source is %q", d.Source)
	}
}

func TestADeckWithNoPublicURLGetsOneMadeFromItsID(t *testing.T) {
	md := moxDeck("Ghen", map[string]int{"Ghen, Arcanum Weaver": 1},
		map[string]int{"Sol Ring": 1}, nil)
	md.PublicURL = ""

	d, err := ToFile(md, "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != "https://moxfield.com/decks/Y8dZ7" {
		t.Errorf("source is %q", d.Source)
	}
}

func TestRefAcceptsTheShapesPeoplePaste(t *testing.T) {
	for in, want := range map[string]string{
		"https://moxfield.com/decks/AAAAAAAAAAAAAAAA":      "AAAAAAAAAAAAAAAA",
		"https://www.moxfield.com/decks/AAAAAAAAAAAAAAAA/": "AAAAAAAAAAAAAAAA",
		"moxfield.com/decks/AAAAAAAAAAAAAAAA?utm_source=x": "AAAAAAAAAAAAAAAA",
		"AAAAAAAAAAAAAAAA": "AAAAAAAAAAAAAAAA",
	} {
		got, ok := Ref(in)
		if !ok || got != want {
			t.Errorf("Ref(%q) = %q, %v", in, got, ok)
		}
	}

	for _, in := range []string{"", "ghen", "a deck name with spaces"} {
		if got, ok := Ref(in); ok {
			t.Errorf("Ref(%q) = %q, want it rejected", in, got)
		}
	}
}

func TestUserAcceptsAURLOrABareName(t *testing.T) {
	for in, want := range map[string]string{
		"https://moxfield.com/users/MarBri": "MarBri",
		"moxfield.com/users/MarBri/decks":   "MarBri",
		"MarBri":                            "MarBri",
		"mar_bri.2":                         "mar_bri.2",
	} {
		got, ok := User(in)
		if !ok || got != want {
			t.Errorf("User(%q) = %q, %v", in, got, ok)
		}
	}

	// Anything with a space or a slash in it was meant to be something else.
	for _, in := range []string{"", "not a thing", "a/b", strings.Repeat("x", 60)} {
		if got, ok := User(in); ok {
			t.Errorf("User(%q) = %q, want it rejected", in, got)
		}
	}
}

func TestAnImportedNameLosesItsSlashes(t *testing.T) {
	// Somebody's "Aggro/Mono Red" is a title, not a folder.
	d, err := ToFile(moxDeck("Aggro/Mono Red", nil, map[string]int{"Mountain": 30}, nil), "Y8dZ7")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Aggro Mono Red" {
		t.Errorf("name is %q", d.Name)
	}
}
