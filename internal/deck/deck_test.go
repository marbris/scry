package deck

import (
	"strings"
	"testing"

	"scry/internal/mtg"
)

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Winota: Snowball Stax":         "winota-snowball-stax",
		"Ghen, Arcanum Weaver":          "ghen-arcanum-weaver",
		"  spaces  everywhere  ":        "spaces-everywhere",
		"Isshin, Two Heavens as One!!!": "isshin-two-heavens-as-one",
		"👑-Marchesa d'Amati":            "marchesa-d-amati",
		"Deck 2":                        "deck-2",
		// A slash groups decks into folders, one slug per segment.
		"Aggro / Mono Red":   "aggro/mono-red",
		"projects/winota v2": "projects/winota-v2",
		// A stray traversal can't survive: ".." has nothing to slugify.
		"../escape": "escape",
		"/leading":  "leading",
		"a//b":      "a/b",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMainEntriesLeaveOutTheMaybeboard(t *testing.T) {
	// A shortlist isn't the deck, and counting it would make every deck
	// with one look illegal.
	d, err := ParseFile(strings.NewReader(strings.Join([]string{
		"name: Ghen", "format: commander", "",
		"[commander]", "1 Ghen, Arcanum Weaver", "",
		"[mainboard]", "1 Sol Ring", "",
		"[maybeboard]", "1 Mana Crypt", "",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}

	main := d.MainEntries()
	if len(main) != 2 {
		t.Fatalf("got %d entries: %v", len(main), main)
	}
	for _, e := range main {
		if e.Name == "Mana Crypt" {
			t.Error("the maybeboard came along")
		}
	}
}

func TestFileFromRoundTripsADeckWithoutFetchingAnything(t *testing.T) {
	// Saving what's on screen must never need the network — this is what
	// makes editing work on a train.
	info := Info{Name: "Ghen", Format: "commander", Slug: "ghen"}
	cards := []Card{
		{Qty: 1, Commander: true, Card: mtg.Card{Name: "Ghen, Arcanum Weaver"}},
		{Qty: 1, Tags: []string{"ramp"}, Card: mtg.Card{Name: "Sol Ring"}},
		{Qty: 7, Card: mtg.Card{Name: "Plains"}},
	}

	body := FileFrom(info, cards).String()
	for _, want := range []string{
		"name: Ghen", "[commander]", "1 Ghen, Arcanum Weaver",
		"1 Sol Ring [ramp]", "7 Plains",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%q is missing from:\n%s", want, body)
		}
	}
}

func TestApplyTagEdits(t *testing.T) {
	for _, c := range []struct {
		name           string
		tags, add, rem []string
		want           string
	}{
		{"adding", []string{"ramp"}, []string{"draw"}, nil, "draw,ramp"},
		{"removing", []string{"ramp", "draw"}, nil, []string{"draw"}, "ramp"},
		{"both at once", []string{"ramp"}, []string{"draw"}, []string{"ramp"}, "draw"},
		{"adding what's there", []string{"ramp"}, []string{"ramp"}, nil, "ramp"},
		{"removing what isn't", []string{"ramp"}, nil, []string{"draw"}, "ramp"},
		{"emptying", []string{"ramp"}, nil, []string{"ramp"}, ""},
	} {
		got := strings.Join(ApplyTagEdits(c.tags, c.add, c.rem), ",")
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTagsAreKeptSortedSoTheFileDoesNotChurn(t *testing.T) {
	// Two decks with the same tags should produce the same line, or git
	// records a change every time you touch a card.
	a := strings.Join(ApplyTagEdits(nil, []string{"wincon", "ramp", "draw"}, nil), ",")
	b := strings.Join(ApplyTagEdits(nil, []string{"draw", "wincon", "ramp"}, nil), ",")
	if a != b {
		t.Errorf("%q and %q", a, b)
	}
}

func TestBookmarksRoundTrip(t *testing.T) {
	isolate(t)

	var b Bookmarks
	b.AddRemote(Remote{Name: "Someone's brew", ID: "Y8dZ7"})
	b.AddUser("MarBri")
	if err := SaveBookmarks(b); err != nil {
		t.Fatal(err)
	}

	back := LoadBookmarks()
	if len(back.Remotes) != 1 || back.Remotes[0].ID != "Y8dZ7" {
		t.Errorf("remotes came back as %v", back.Remotes)
	}
	if len(back.Users) != 1 || back.Users[0] != "MarBri" {
		t.Errorf("users came back as %v", back.Users)
	}
}

func TestNoBookmarksFileMeansNoBookmarks(t *testing.T) {
	// Which is what a first run looks like, and isn't worth reporting.
	isolate(t)
	b := LoadBookmarks()
	if len(b.Remotes) != 0 || len(b.Users) != 0 {
		t.Errorf("got %v", b)
	}
}

func TestRemovingAndRenamingBookmarks(t *testing.T) {
	var b Bookmarks
	b.AddRemote(Remote{Name: "One", ID: "aaa"})
	b.AddRemote(Remote{Name: "Two", ID: "bbb"})
	b.AddUser("alice")
	b.AddUser("bob")

	b.RenameRemote("aaa", "Renamed")
	if b.Remotes[0].Name != "Renamed" {
		t.Errorf("remotes are %v", b.Remotes)
	}

	b.RemoveRemote("bbb")
	if len(b.Remotes) != 1 || b.Remotes[0].ID != "aaa" {
		t.Errorf("remotes are %v", b.Remotes)
	}

	// Moxfield doesn't care about case in a username, so neither does this.
	b.RemoveUser("ALICE")
	if len(b.Users) != 1 || b.Users[0] != "bob" {
		t.Errorf("users are %v", b.Users)
	}
}

func TestSummariesReadEveryDeckWithoutResolvingACard(t *testing.T) {
	// Opening the decks panel must not reach the network, or it is unusable
	// on a train.
	isolate(t)
	seed(t, "ghen", "name: Ghen reanimator\nformat: commander\n[commander]\n1 Ghen, Arcanum Weaver\n[mainboard]\n1 Sol Ring\n30 Plains\n")
	seed(t, "elf", "name: Elf Ball\nformat: commander\n[mainboard]\n1 Llanowar Elves\n")

	got, err := Summaries()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d summaries", len(got))
	}

	by := map[string]Summary{}
	for _, s := range got {
		by[s.Slug] = s
	}
	if by["ghen"].Name != "Ghen reanimator" {
		t.Errorf("name is %q", by["ghen"].Name)
	}
	if by["ghen"].Total != 32 || by["ghen"].Unique != 3 {
		t.Errorf("counts are %d/%d, want 32/3", by["ghen"].Total, by["ghen"].Unique)
	}
	if by["ghen"].Modified.IsZero() {
		t.Error("no modification time")
	}
}

func TestABrokenDeckFileStillGetsASummary(t *testing.T) {
	// Silently omitting a deck you know you have is worse than showing it
	// with a warning.
	isolate(t)
	seed(t, "fine", "name: Fine\n[mainboard]\n1 Sol Ring\n")
	if err := writeFile(Path("broken"), "[mainboard]\n0 Nope\n"); err != nil {
		t.Fatal(err)
	}

	got, err := Summaries()
	if err != nil {
		t.Fatal(err)
	}
	var broken *Summary
	for i := range got {
		if got[i].Slug == "broken" {
			broken = &got[i]
		}
	}
	if broken == nil {
		t.Fatal("the broken deck was dropped from the listing")
	}
	if !broken.Broken {
		t.Error("it isn't flagged as unreadable")
	}
}

func TestResolveCachedAnswersFromTheCacheOrSaysItCouldNot(t *testing.T) {
	seedCache(t, map[string]mtg.Card{
		"sol ring": {Name: "Sol Ring", TypeLine: "Artifact"},
	})

	cards, complete := ResolveCached([]Entry{
		{Qty: 1, Name: "Sol Ring", Section: "mainboard", Tags: []string{"ramp"}},
	})
	if !complete {
		t.Error("a fully cached deck was reported incomplete")
	}
	if len(cards) != 1 || cards[0].Card.Name != "Sol Ring" {
		t.Fatalf("got %v", cards)
	}
	if len(cards[0].Tags) != 1 || cards[0].Tags[0] != "ramp" {
		t.Errorf("tags were lost: %v", cards[0].Tags)
	}

	cards, complete = ResolveCached([]Entry{
		{Qty: 1, Name: "Sol Ring", Section: "mainboard"},
		{Qty: 1, Name: "Not In The Cache", Section: "mainboard"},
	})
	if complete {
		t.Error("a deck with an unknown card was reported complete")
	}
	if len(cards) != 1 {
		t.Errorf("got %d cards, want the one it could answer", len(cards))
	}
}

func TestResolveCachedMarksTheCommander(t *testing.T) {
	seedCache(t, map[string]mtg.Card{
		"ghen, arcanum weaver": {Name: "Ghen, Arcanum Weaver"},
	})
	cards, _ := ResolveCached([]Entry{
		{Qty: 1, Name: "Ghen, Arcanum Weaver", Section: "commander"},
	})
	if len(cards) != 1 || !cards[0].Commander {
		t.Errorf("got %v", cards)
	}
}

func TestNewDeckStartsEmptyAndNamed(t *testing.T) {
	isolate(t)
	slug, d, err := New("Elf Ball", "commander")
	if err != nil {
		t.Fatal(err)
	}
	if slug != "elf-ball" {
		t.Errorf("slug is %q", slug)
	}
	if d.Name != "Elf Ball" || d.Format != "commander" {
		t.Errorf("got %+v", d)
	}
	if len(d.Entries) != 0 {
		t.Errorf("a new deck came with %d cards", len(d.Entries))
	}
}
