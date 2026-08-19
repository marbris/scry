package deck

import (
	"scry/internal/mtg"

	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseDeckLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want Entry
	}{
		{
			name: "bare name defaults to one",
			line: "Sol Ring",
			want: Entry{Qty: 1, Name: "Sol Ring"},
		},
		{
			name: "quantity",
			line: "7 Plains",
			want: Entry{Qty: 7, Name: "Plains"},
		},
		{
			name: "tags",
			line: "1 Sol Ring [ramp, artifact]",
			want: Entry{Qty: 1, Name: "Sol Ring", Tags: []string{"artifact", "ramp"}},
		},
		{
			name: "pinned printing",
			line: "1 Sol Ring (c21) 263",
			want: Entry{Qty: 1, Name: "Sol Ring", Set: "c21", Collector: "263"},
		},
		{
			name: "pinned printing and tags",
			line: "1 Sol Ring (c21) 263 [ramp]",
			want: Entry{Qty: 1, Name: "Sol Ring", Set: "c21", Collector: "263", Tags: []string{"ramp"}},
		},
		{
			name: "collector number with a letter",
			line: "1 Arcane Signet (eld) 331p",
			want: Entry{Qty: 1, Name: "Arcane Signet", Set: "eld", Collector: "331p"},
		},
		{
			// The reason the printing group is shaped the way it is: this
			// card's name really does end in a parenthesis.
			name: "name ending in parentheses is not a printing",
			line: "1 Erase (Not the Urza's Legacy One)",
			want: Entry{Qty: 1, Name: "Erase (Not the Urza's Legacy One)"},
		},
		{
			name: "double-faced name keeps its slashes",
			line: "1 Fire // Ice [removal]",
			want: Entry{Qty: 1, Name: "Fire // Ice", Tags: []string{"removal"}},
		},
		{
			name: "name with a comma",
			line: "1 Ghen, Arcanum Weaver [wincon]",
			want: Entry{Qty: 1, Name: "Ghen, Arcanum Weaver", Tags: []string{"wincon"}},
		},
		{
			name: "tags are lowercased, sorted and de-duplicated",
			line: "1 Sol Ring [Ramp,  ramp , Artifact]",
			want: Entry{Qty: 1, Name: "Sol Ring", Tags: []string{"artifact", "ramp"}},
		},
		{
			name: "empty tag list is no tags",
			line: "1 Sol Ring []",
			want: Entry{Qty: 1, Name: "Sol Ring"},
		},
		{
			name: "set code is normalised to lower case",
			line: "1 Sol Ring (C21) 263",
			want: Entry{Qty: 1, Name: "Sol Ring", Set: "c21", Collector: "263"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEntry(tt.line)
			if err != nil {
				t.Fatalf("parseEntry(%q): %v", tt.line, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseEntry(%q)\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseDeckLineRejects(t *testing.T) {
	for _, line := range []string{"", "0 Sol Ring"} {
		if _, err := parseEntry(line); err == nil {
			t.Errorf("parseEntry(%q) should have failed", line)
		}
	}
}

const sampleDeck = `# a note to self
# and a second line

name: Ghen, Arcanum Weaver
format: commander
source: https://moxfield.com/decks/AbC123
colour: rw

[commander]
1 Ghen, Arcanum Weaver [wincon]

[mainboard]
1 Sol Ring (c21) 263 [ramp, artifact]
1 Smothering Tithe [ramp]
7 Plains

[maybeboard]
1 Anguished Unmaking [removal]
`

func TestParseDeckFile(t *testing.T) {
	d, err := ParseFile(strings.NewReader(sampleDeck))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if d.Name != "Ghen, Arcanum Weaver" {
		t.Errorf("name = %q", d.Name)
	}
	if d.Format != "commander" {
		t.Errorf("format = %q", d.Format)
	}
	if d.Source != "https://moxfield.com/decks/AbC123" {
		t.Errorf("source = %q", d.Source)
	}
	if len(d.Notes) != 2 {
		t.Errorf("notes = %v, want the two leading comment lines", d.Notes)
	}
	if want := [][2]string{{"colour", "rw"}}; !reflect.DeepEqual(d.Extra, want) {
		t.Errorf("extra = %v, want %v", d.Extra, want)
	}
	if len(d.Entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(d.Entries))
	}

	// Sections stick to the cards under them.
	bySection := map[string]int{}
	for _, e := range d.Entries {
		bySection[e.Section]++
	}
	if bySection["commander"] != 1 || bySection["mainboard"] != 3 || bySection["maybeboard"] != 1 {
		t.Errorf("sections = %v", bySection)
	}

	// The maybeboard is a shortlist, not part of the deck.
	total, unique := d.Counts()
	if total != 10 || unique != 4 {
		t.Errorf("counts() = (%d, %d), want (10, 4)", total, unique)
	}
}

// sortedEntries puts a deck's entries in a fixed order so two decks can be
// compared by content regardless of the order they were read in.
func sortedEntries(d *File) []Entry {
	out := append([]Entry(nil), d.Entries...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func TestDeckFileRoundTrip(t *testing.T) {
	d, err := ParseFile(strings.NewReader(sampleDeck))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	out := d.String()
	again, err := ParseFile(strings.NewReader(out))
	if err != nil {
		t.Fatalf("reparsing our own output: %v", err)
	}

	// Writing what we read back must not drift, or every deck open would
	// show up as a git diff.
	if second := again.String(); second != out {
		t.Errorf("round trip is not stable:\n--- first ---\n%s\n--- second ---\n%s", out, second)
	}
	// Order is deliberately not preserved — String() writes each section
	// alphabetically — so compare the entries as a set.
	if !reflect.DeepEqual(sortedEntries(d), sortedEntries(again)) {
		t.Errorf("entries changed across a round trip:\n%+v\nvs\n%+v",
			sortedEntries(d), sortedEntries(again))
	}
	if !reflect.DeepEqual(d.Notes, again.Notes) {
		t.Errorf("notes changed across a round trip: %v vs %v", d.Notes, again.Notes)
	}
	if !reflect.DeepEqual(d.Extra, again.Extra) {
		t.Errorf("unknown header keys changed across a round trip")
	}
}

func TestDeckFileCanonicalOrder(t *testing.T) {
	// The same deck, written in a different order with differently-cased
	// tags, must serialise to the same bytes.
	a := `name: T
[mainboard]
1 Sol Ring [Ramp]
1 Arcane Signet [ramp]
`
	b := `name: T
[mainboard]
1 Arcane Signet [RAMP]
1 Sol Ring [ramp]
`
	da, err := ParseFile(strings.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	db, err := ParseFile(strings.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if da.String() != db.String() {
		t.Errorf("orderings differ:\n%s\nvs\n%s", da.String(), db.String())
	}
}

func TestParseDeckFileNoHeader(t *testing.T) {
	// A bare decklist pasted in, with no header and no sections at all.
	d, err := ParseFile(strings.NewReader("1 Sol Ring\n1 Smothering Tithe\n"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(d.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(d.Entries))
	}
	for _, e := range d.Entries {
		if e.Section != "mainboard" {
			t.Errorf("%q landed in %q, want mainboard", e.Name, e.Section)
		}
	}
}

func TestParseDeckFileReportsLineNumbers(t *testing.T) {
	_, err := ParseFile(strings.NewReader("[mainboard]\n1 Sol Ring\n0 Broken\n"))
	if err == nil {
		t.Fatal("expected an error for a zero quantity")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error should point at line 3, got: %v", err)
	}
}

func TestDeckStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SCRY_DECKS_DIR", dir)

	if decks, err := List(); err != nil || len(decks) != 0 {
		t.Fatalf("List() on an empty dir = %v, %v", decks, err)
	}
	if Exists("ghen") {
		t.Error("Exists() true before anything was written")
	}

	d, err := ParseFile(strings.NewReader(sampleDeck))
	if err != nil {
		t.Fatal(err)
	}
	if err := Write("ghen", d); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "ghen.deck")); err != nil {
		t.Fatalf("deck file not where it should be: %v", err)
	}
	if !Exists("ghen") {
		t.Error("Exists() false after a write")
	}

	decks, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decks, []string{"ghen"}) {
		t.Errorf("List() = %v", decks)
	}

	back, err := Read("ghen")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if back.String() != d.String() {
		t.Errorf("what came back off disk isn't what went on:\n%s\nvs\n%s", back.String(), d.String())
	}

	// Writing over a deck replaces it rather than appending.
	if err := Write("ghen", d); err != nil {
		t.Fatal(err)
	}
	if decks, _ := List(); len(decks) != 1 {
		t.Errorf("a second write left %d decks", len(decks))
	}

	if err := Delete("ghen"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if Exists("ghen") {
		t.Error("deck still there after Delete")
	}
}

func TestReadDeckNamesItselfAfterItsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SCRY_DECKS_DIR", dir)

	if err := os.WriteFile(filepath.Join(dir, "untitled.deck"),
		[]byte("[mainboard]\n1 Sol Ring\n"), 0644); err != nil {
		t.Fatal(err)
	}
	d, err := Read("untitled")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "untitled" {
		t.Errorf("name = %q, want the slug as a fallback", d.Name)
	}
}

func TestEntryKey(t *testing.T) {
	// Case must not split an entry in the cache.
	if a, b := entryKey(Entry{Name: "Sol Ring"}), entryKey(Entry{Name: "sol ring"}); a != b {
		t.Errorf("entryKey is case sensitive: %q vs %q", a, b)
	}
	// A pinned printing is a different card to the unpinned name.
	pinned := entryKey(Entry{Name: "Sol Ring", Set: "c21", Collector: "263"})
	if pinned == entryKey(Entry{Name: "Sol Ring"}) {
		t.Error("a pinned printing should key differently to a bare name")
	}
}

func TestIndexCardsFindsEitherFace(t *testing.T) {
	idx := indexCards([]mtg.Card{
		{Name: "Fire // Ice", Set: "apc", CollectorNumber: "128"},
		{Name: "Sol Ring", Set: "c21", CollectorNumber: "263"},
	})

	for _, want := range []string{"Fire // Ice", "Fire", "Ice"} {
		if _, ok := idx.lookup(Entry{Name: want}); !ok {
			t.Errorf("lookup(%q) missed", want)
		}
	}
	if _, ok := idx.lookup(Entry{Name: "Sol Ring", Set: "c21", Collector: "263"}); !ok {
		t.Error("lookup by pinned printing missed")
	}
	if _, ok := idx.lookup(Entry{Name: "Sol Ring", Set: "lea", Collector: "1"}); ok {
		t.Error("lookup matched a printing that wasn't there")
	}
}
