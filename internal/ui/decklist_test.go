package ui

import (
	"strings"
	"testing"
	"time"

	"scry/internal/deck"
)

// seedDeck writes a deck into the isolated decks directory.
func seedDeck(t *testing.T, slug, body string) {
	t.Helper()
	d, err := deck.ParseFile(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := deck.Write(slug, d); err != nil {
		t.Fatal(err)
	}
}

func TestTheDecksPanelOpensOnYourDecks(t *testing.T) {
	// Not on an empty search bar: you opened it to see what you have.
	seedDeck(t, "ghen", "name: Ghen reanimator\nformat: commander\n[mainboard]\n1 Sol Ring\n")

	m := drive(sized(120, 30), "space", "d")
	p := m.ws.current()
	if p.searchOpen {
		t.Error("the decks panel opened on a search bar")
	}
	if _, ok := p.top().(*deckList); !ok {
		t.Fatalf("the panel is showing %T", p.top())
	}
	if !strings.Contains(stripANSI(m.View()), "Ghen reanimator") {
		t.Error("the deck is not listed")
	}
}

func TestLocalRemoteAndPeopleShareOneList(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\n[mainboard]\n1 Sol Ring\n")

	b := deck.LoadBookmarks()
	b.AddRemote(deck.Remote{Name: "Someone's brew", ID: "Y8dZ7"})
	b.AddUser("MarBri")
	if err := deck.SaveBookmarks(b); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deck.SaveBookmarks(deck.Bookmarks{}) })

	m := drive(sized(120, 30), "space", "d")
	view := stripANSI(m.View())

	for _, want := range []string{"Ghen", "Someone's brew", "MarBri"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q is missing from:\n%s", want, view)
		}
	}
}

func TestEachKindOfRowSaysWhatItIs(t *testing.T) {
	for _, c := range []struct {
		kind entryKind
		want string
	}{
		{entryLocal, "L"},
		{entryRemote, "R"},
		{entryUser, "U"},
	} {
		got := stripANSI(renderEntry(deckEntry{kind: c.kind, name: "thing"}, 30, false))
		if !strings.Contains(got, c.want) {
			t.Errorf("%v rendered %q, want a %s in it", c.kind, got, c.want)
		}
	}
}

func TestARowIsExactlyAsWideAsItIsToldToBe(t *testing.T) {
	e := deckEntry{
		kind: entryLocal, name: "Isshin, One Pillow Fort as One",
		count: 100, modified: time.Now().Add(-72 * time.Hour),
	}
	for width := 8; width <= 60; width++ {
		if got := textWidth(stripANSI(renderEntry(e, width, false))); got > width {
			t.Errorf("width %d rendered %d columns", width, got)
		}
	}
}

func TestTheTailIsGivenUpFromTheRight(t *testing.T) {
	// A name with no age beside it is still a name; an age with no name is
	// nothing.
	e := deckEntry{
		kind: entryLocal, name: "Isshin, One Pillow Fort as One",
		count: 100, modified: time.Now().Add(-72 * time.Hour),
	}

	wide := stripANSI(renderEntry(e, 46, false))
	if !strings.Contains(wide, "3d") || !strings.Contains(wide, "100") {
		t.Errorf("at 46 columns the whole tail should show: %q", wide)
	}

	narrow := stripANSI(renderEntry(e, 20, false))
	if strings.Contains(narrow, "3d") {
		t.Errorf("the age should have gone first: %q", narrow)
	}
	if !strings.Contains(narrow, "L") {
		t.Errorf("the kind should be the last thing standing: %q", narrow)
	}
}

func TestABrokenDeckStillGetsARow(t *testing.T) {
	// Silently omitting a deck you know you have is worse than showing it
	// with a warning.
	seedDeck(t, "fine", "name: Fine\n[mainboard]\n1 Sol Ring\n")
	if err := writeString(deck.Path("broken"), "[mainboard]\n0 Nope\n"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deck.Delete("broken") })

	m := drive(sized(120, 30), "space", "d")
	if !strings.Contains(stripANSI(m.View()), "unreadable") {
		t.Errorf("the broken deck is not flagged:\n%s", stripANSI(m.View()))
	}
}

func TestSortingTheDeckList(t *testing.T) {
	// The sort orders the decks within a folder; these all sit at the root.
	l := &deckList{order: byName, all: []deckEntry{
		{kind: entryLocal, name: "Zebra", slug: "zebra", count: 10},
		{kind: entryLocal, name: "Apple", slug: "apple", count: 5},
		{kind: entryLocal, name: "Mango", slug: "mango", count: 100},
	}}
	l.refresh()
	if l.rows[0].name != "Apple" {
		t.Errorf("by name starts with %s", l.rows[0].name)
	}

	l.order = bySize
	l.refresh()
	if l.rows[0].name != "Mango" {
		t.Errorf("by size starts with %s, want the biggest", l.rows[0].name)
	}
}

func TestThingsWithNoDateSortLastNotFirst(t *testing.T) {
	// A deck with no modification time shouldn't claim 1970 and lead the list.
	l := &deckList{order: byModified, all: []deckEntry{
		{kind: entryLocal, name: "undated", slug: "undated"},
		{kind: entryLocal, name: "dated", slug: "dated", modified: time.Now().Add(-time.Hour)},
	}}
	l.refresh()
	if l.rows[0].name != "dated" {
		t.Errorf("by last touched starts with %s", l.rows[0].name)
	}
}

func TestFollowedThingsLiveUnderTheMoxfieldFolder(t *testing.T) {
	l := &deckList{all: []deckEntry{
		{kind: entryLocal, name: "Ghen", slug: "ghen"},
		{kind: entryUser, name: "alice", user: "alice"},
		{kind: entryRemote, name: "someones brew", id: "abc"},
	}}
	l.refresh()

	// The moxfield folder is a row of its own, with the two followed things
	// indented under it and the local deck at the root.
	var folder *deckEntry
	for i := range l.rows {
		if l.rows[i].kind == entryFolder && l.rows[i].slug == moxFolder {
			folder = &l.rows[i]
		}
	}
	if folder == nil {
		t.Fatal("no moxfield folder")
	}
	if folder.count != 2 {
		t.Errorf("moxfield folder holds %d, want the remote and the user", folder.count)
	}
	for _, r := range l.rows {
		if (r.kind == entryRemote || r.kind == entryUser) && r.depth != 1 {
			t.Errorf("%s is at depth %d, want it under moxfield", r.name, r.depth)
		}
	}
}

func TestDecksGroupIntoFoldersByTheirSlug(t *testing.T) {
	l := &deckList{collapsed: map[string]bool{}, all: []deckEntry{
		{kind: entryLocal, name: "Mono Red", slug: "aggro/mono-red"},
		{kind: entryLocal, name: "Budget", slug: "aggro/budget"},
		{kind: entryLocal, name: "Loose", slug: "loose"},
	}}
	l.refresh()

	// A folder row for aggro, its two decks indented under it, then the root deck.
	if l.rows[0].kind != entryFolder || l.rows[0].slug != "aggro" || l.rows[0].count != 2 {
		t.Fatalf("first row isn't the aggro folder: %+v", l.rows[0])
	}
	for _, r := range l.rows {
		if r.kind == entryLocal && folderOf(r.slug) == "aggro" && r.depth != 1 {
			t.Errorf("%s is at depth %d, want it inside the folder", r.name, r.depth)
		}
		if r.slug == "loose" && r.depth != 0 {
			t.Errorf("the root deck is at depth %d, want 0", r.depth)
		}
	}
}

func TestFoldingAFolderHidesItsDecks(t *testing.T) {
	l := &deckList{collapsed: map[string]bool{}, all: []deckEntry{
		{kind: entryLocal, name: "Mono Red", slug: "aggro/mono-red"},
		{kind: entryLocal, name: "Loose", slug: "loose"},
	}}
	l.refresh()

	l.toggleFolder("aggro")
	for _, r := range l.rows {
		if r.name == "Mono Red" {
			t.Error("a collapsed folder still shows its deck")
		}
	}
	// The folder row itself stays, and the cursor is left on it.
	if e, _ := l.current(); e.kind != entryFolder || e.slug != "aggro" {
		t.Errorf("cursor left the folder it toggled: %+v", e)
	}

	l.toggleFolder("aggro")
	found := false
	for _, r := range l.rows {
		if r.name == "Mono Red" {
			found = true
		}
	}
	if !found {
		t.Error("reopening the folder didn't bring its deck back")
	}
}

func TestMovingADeckRenamesItForItsNewFolder(t *testing.T) {
	// The name carries the folder, so a move has to rewrite the prefix: a deck
	// in test2 mustn't still call itself test/….
	seedDeck(t, "mvsrc/thedeck", "name: mvsrc/thedeck\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("mvsrc/thedeck"); deck.Delete("mvdst/thedeck") })

	var m Model
	cmd := m.putDeck(deckMove{slug: "mvsrc/thedeck", name: "mvsrc/thedeck", cut: true}, "mvdst")
	if msg := cmd().(noticeMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}

	if deck.Exists("mvsrc/thedeck") {
		t.Error("the deck is still at its old slug")
	}
	d, err := deck.Read("mvdst/thedeck")
	if err != nil {
		t.Fatalf("not at the new slug: %v", err)
	}
	if d.Name != "mvdst/thedeck" {
		t.Errorf("name is %q, want mvdst/thedeck", d.Name)
	}
}

func TestMovingADeckToTheRootDropsItsFolderFromTheName(t *testing.T) {
	seedDeck(t, "rootmv/thedeck", "name: rootmv/thedeck\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("rootmv/thedeck"); deck.Delete("thedeck") })

	var m Model
	cmd := m.putDeck(deckMove{slug: "rootmv/thedeck", name: "rootmv/thedeck", cut: true}, "")
	if msg := cmd().(noticeMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}
	d, err := deck.Read("thedeck")
	if err != nil {
		t.Fatalf("not at the root: %v", err)
	}
	if d.Name != "thedeck" {
		t.Errorf("name is %q, want thedeck", d.Name)
	}
}

func TestCurrentFolderIsWhereAPutLands(t *testing.T) {
	l := &deckList{collapsed: map[string]bool{}, all: []deckEntry{
		{kind: entryLocal, name: "Mono Red", slug: "aggro/mono-red"},
		{kind: entryLocal, name: "Loose", slug: "loose"},
	}}
	l.refresh()

	// On the folder row → into the folder.
	l.cursor.at = 0
	if got := l.currentFolder(); got != "aggro" {
		t.Errorf("on the folder, currentFolder = %q, want aggro", got)
	}
	// On a deck inside a folder → that folder.
	for i, r := range l.rows {
		if r.slug == "aggro/mono-red" {
			l.cursor.at = i
		}
	}
	if got := l.currentFolder(); got != "aggro" {
		t.Errorf("on a deck in aggro, currentFolder = %q, want aggro", got)
	}
	// On a root deck → the root.
	for i, r := range l.rows {
		if r.slug == "loose" {
			l.cursor.at = i
		}
	}
	if got := l.currentFolder(); got != "" {
		t.Errorf("on a root deck, currentFolder = %q, want the root", got)
	}
}

func TestFilteringTheDeckList(t *testing.T) {
	l := &deckList{all: []deckEntry{
		{kind: entryLocal, name: "Ghen reanimator"},
		{kind: entryLocal, name: "Elf Ball"},
	}}
	l.refresh()
	l.setFilter("elf")
	if len(l.rows) != 1 || l.rows[0].name != "Elf Ball" {
		t.Errorf("got %v", l.rows)
	}
}

func TestDeletingADeckAsksFirst(t *testing.T) {
	// A deck file is the only row that loses work. Following and
	// unfollowing are free, so they just happen.
	seedDeck(t, "ghen", "name: Ghen\n[mainboard]\n1 Sol Ring\n")
	m := drive(sized(120, 30), "space", "d")
	m = drive(m, "d")

	l := m.ws.current().top().(*deckList)
	if l.confirming == nil {
		t.Fatal("d deleted a deck without asking")
	}
	if !strings.Contains(stripANSI(m.View()), "delete") {
		t.Error("nothing on screen asks the question")
	}

	m = drive(m, "n") // anything but y
	if l.confirming != nil {
		t.Error("the question is still standing")
	}
	if !deck.Exists("ghen") {
		t.Error("the deck went anyway")
	}
}

func TestTheConfirmationCannotBeAnsweredByAccident(t *testing.T) {
	// The pending deletion swallows the next key whatever it is, so a
	// keystroke meant for the list can't confirm it.
	seedDeck(t, "ghen", "name: Ghen\n[mainboard]\n1 Sol Ring\n")
	m := drive(sized(120, 30), "space", "d")
	m = drive(m, "d")

	l := m.ws.current().top().(*deckList)
	before := l.cursor.at
	m = drive(m, "j")
	if l.cursor.at != before {
		t.Error("j moved the cursor while a deletion was waiting")
	}
}

func TestFollowingAMoxfieldUser(t *testing.T) {
	t.Cleanup(func() { deck.SaveBookmarks(deck.Bookmarks{}) })

	msg := follow("MarBri")().(noticeMsg)
	if msg.err != nil {
		t.Fatalf("following failed: %v", msg.err)
	}
	if got := deck.LoadBookmarks().Users; len(got) != 1 || got[0] != "MarBri" {
		t.Errorf("bookmarks hold %v", got)
	}
}

func TestFollowingADeckURL(t *testing.T) {
	t.Cleanup(func() { deck.SaveBookmarks(deck.Bookmarks{}) })

	// A URL is recognised by its shape; the name is filled in from Moxfield
	// when it can be reached, and falls back to the id when it can't.
	msg := follow("https://moxfield.com/decks/AAAAAAAAAAAAAAAA")().(noticeMsg)
	if msg.err != nil {
		t.Fatalf("following failed: %v", msg.err)
	}
	if got := deck.LoadBookmarks().Remotes; len(got) != 1 || got[0].ID != "AAAAAAAAAAAAAAAA" {
		t.Errorf("bookmarks hold %v", got)
	}
}

func TestFollowingNonsenseSaysSo(t *testing.T) {
	msg := follow("!!! not a thing !!!")().(noticeMsg)
	if msg.err == nil {
		t.Error("nonsense was accepted")
	}
}

func TestBookmarksAreNotDuplicated(t *testing.T) {
	var b deck.Bookmarks
	b.AddUser("MarBri")
	b.AddUser("marbri") // Moxfield doesn't care about case, so neither do we
	if len(b.Users) != 1 {
		t.Errorf("users are %v", b.Users)
	}

	b.AddRemote(deck.Remote{Name: "one", ID: "abc"})
	b.AddRemote(deck.Remote{Name: "renamed", ID: "abc"})
	if len(b.Remotes) != 1 || b.Remotes[0].Name != "renamed" {
		t.Errorf("remotes are %v", b.Remotes)
	}
}

func TestNewDeckPromptsForAName(t *testing.T) {
	m := drive(sized(120, 30), "space", "d")
	m = drive(m, "n")
	if m.ws.current().asking != askNewDeck {
		t.Fatal("n did not ask for a name")
	}
	for _, r := range "Elf Ball" {
		m = drive(m, string(r))
	}
	m = drive(m, "enter")

	// The command runs off the main thread; run it here.
	if got := newDeckCmd("", "Elf Ball")().(noticeMsg); got.err != nil {
		t.Fatalf("making the deck failed: %v", got.err)
	}
	if !deck.Exists("elf-ball") {
		t.Error("the deck was not created")
	}
	t.Cleanup(func() { deck.Delete("elf-ball") })
}

func TestCopyingADeckDoesNotOverwriteIt(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")

	msg := copyEntry(deckEntry{kind: entryLocal, name: "Ghen", slug: "ghen"})().(noticeMsg)
	if msg.err != nil {
		t.Fatalf("copying failed: %v", msg.err)
	}
	if !deck.Exists("ghen") {
		t.Error("the original went missing")
	}
	if !deck.Exists("ghen-copy") {
		t.Errorf("no copy was made: %s", msg.text)
	}
	t.Cleanup(func() { deck.Delete("ghen-copy") })
}

func TestWideCharactersAreMeasuredByTheRoomTheyTake(t *testing.T) {
	// An emoji is one rune and two columns. Measured in runes, this row
	// overflows its panel, wraps, and leaves the panel a line taller than
	// the one beside it — which is exactly what a real deck name did.
	e := deckEntry{
		kind: entryLocal, count: 100,
		name: "👑-Marchesa d'Amati, First of her Name: Queens Gambit♟️",
	}
	for width := 10; width <= 60; width++ {
		got := stripANSI(renderEntry(e, width, false))
		if strings.Contains(got, "\n") {
			t.Fatalf("width %d wrapped: %q", width, got)
		}
		if w := textWidth(got); w > width {
			t.Errorf("width %d rendered %d columns: %q", width, w, got)
		}
	}
}

func TestWideCharactersInACardRowToo(t *testing.T) {
	c := deckCardNamed("Nalfeshnee 👹 // Abyssal 👹 Horror")
	for width := 8; width <= 50; width++ {
		got := stripANSI(row(c, sortMana, width))
		if w := textWidth(got); w > width {
			t.Errorf("width %d rendered %d columns: %q", width, w, got)
		}
	}
}

func TestTheFlagColumnSaysWhetherADeckIsLegal(t *testing.T) {
	for _, c := range []struct {
		name string
		v    deck.Legality
		want string
	}{
		{"legal", deck.Legality{Known: true, Legal: true}, "*"},
		{"illegal", deck.Legality{Known: true}, "!"},
		{"unchecked", deck.Legality{}, " "},
	} {
		got := stripANSI(renderEntry(deckEntry{
			kind: entryLocal, name: "Deck", count: 100, legal: c.v,
		}, 30, false))
		if !strings.Contains(got, "L"+c.want) {
			t.Errorf("%s rendered %q, want L%s", c.name, got, c.want)
		}
	}
}

func TestTheInfoPanelSaysWhyADeckIsIllegal(t *testing.T) {
	// A deck that is merely "illegal" tells you nothing you can act on.
	l := &deckList{all: []deckEntry{{
		kind: entryLocal, name: "Too Big", count: 157,
		legal: deck.Legality{
			Format: "commander", Known: true,
			Problems: []deck.Problem{
				{Text: "157 cards, needs exactly 100"},
				{Text: "outside the commander's colours", Cards: []string{"Llanowar Elves"}},
			},
		},
	}}}
	l.refresh()

	info := stripANSI(strings.Join(l.info(60), "\n"))
	for _, want := range []string{"not legal in commander", "needs exactly 100", "Llanowar Elves"} {
		if !strings.Contains(info, want) {
			t.Errorf("%q is missing from:\n%s", want, info)
		}
	}
}

func TestAnEmptyDeckSaysZeroRatherThanNothing(t *testing.T) {
	// Blank reads as "we haven't looked"; an empty deck is a fact.
	got := stripANSI(renderEntry(deckEntry{kind: entryLocal, name: "Elf Ball", count: 0}, 30, false))
	if !strings.Contains(got, "0") {
		t.Errorf("got %q", got)
	}
}

func TestADecksColoursAreShownAsPips(t *testing.T) {
	// "Is this the Mardu deck or the Simic one" is the question a column of
	// deck names can't answer.
	got := stripANSI(renderEntry(deckEntry{
		kind: entryLocal, name: "Ghen", count: 100,
		colours: []string{"R", "W", "B"},
	}, 40, false))
	if !strings.Contains(got, "WBR") {
		t.Errorf("got %q, want the colours in WUBRG order", got)
	}
}

func TestPipsAreAlwaysInWUBRGOrder(t *testing.T) {
	// So two Mardu decks read the same, whatever order their cards happened
	// to be resolved in.
	if got := manaPips([]string{"G", "W", "U"}); got != "WUG" {
		t.Errorf("got %q", got)
	}
	if got := manaPips(nil); got != "" {
		t.Errorf("got %q for a colourless deck", got)
	}
}

func TestTheKindLetterLinesUpDownTheList(t *testing.T) {
	// A user carries only their letter, where a local deck carries a whole
	// tail — but U still has to sit under L and R, so the columns are sized
	// once for the list and held fixed per row.
	rows := []deckEntry{
		{kind: entryLocal, name: "Local", count: 100, colours: []string{"G"},
			legal: deck.Legality{Known: true, Legal: true}},
		{kind: entryRemote, name: "Remote", count: 60, colours: []string{"U"}},
		{kind: entryUser, name: "Person"},
	}
	const width = 40
	cols := measureDeckCols(rows, width)
	pos := width - deckTailWidth(cols) // where the kind letter must land

	for _, e := range rows {
		line := []rune(stripANSI(renderEntryCols(e, cols, width, false)))
		if textWidth(string(line)) != width {
			t.Fatalf("row %q is %d wide, want %d", e.name, textWidth(string(line)), width)
		}
		if got := string(line[pos]); got != e.kind.letter() {
			t.Errorf("%s row has %q at the kind column, want %q", e.name, got, e.kind.letter())
		}
	}
}

func TestAColourlessOrRemoteRowShowsNoPips(t *testing.T) {
	got := stripANSI(renderEntry(deckEntry{kind: entryRemote, name: "Someone's"}, 30, false))
	if strings.ContainsAny(got, "WUBRG") && !strings.Contains(got, "Someone") {
		t.Errorf("got %q", got)
	}
}

func TestTheRowStaysExactlyAsWideWithPipsAndColour(t *testing.T) {
	// The tail is assembled from styled parts now, so its width has to be
	// measured with the escapes stripped out.
	e := deckEntry{
		kind: entryLocal, name: "Isshin, One Pillow Fort as One", count: 100,
		colours: []string{"W", "B", "R"},
		legal:   deck.Legality{Known: true, Legal: true},
	}
	for width := 10; width <= 60; width++ {
		got := stripANSI(renderEntry(e, width, false))
		if strings.Contains(got, "\n") {
			t.Fatalf("width %d wrapped: %q", width, got)
		}
		if textWidth(got) > width {
			t.Errorf("width %d rendered %d columns: %q", width, textWidth(got), got)
		}
	}
}

// TestRenamingWithASlashMovesIntoAFolder covers the decks-panel rename that
// reads a slash as a folder: the deck's file moves under that folder, created
// if it wasn't there, so a deck lands in a folder by being renamed rather than
// only when it's made.
func TestRenamingWithASlashMovesIntoAFolder(t *testing.T) {
	seedDeck(t, "brew", "name: brew\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.DeleteCommitted("brew"); deck.DeleteCommitted("aggro/brew") })

	e := deckEntry{kind: entryLocal, name: "brew", slug: "brew"}
	if msg, ok := renameCmd(e, "aggro/brew")().(noticeMsg); !ok || msg.err != nil {
		t.Fatalf("rename returned %#v", msg)
	}

	if deck.Exists("brew") {
		t.Error("the deck is still at its old root slug")
	}
	if !deck.Exists("aggro/brew") {
		t.Fatal("the deck was not moved into the aggro folder")
	}
	d, err := deck.Read("aggro/brew")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "aggro/brew" {
		t.Errorf("name header is %q, want %q", d.Name, "aggro/brew")
	}
}

// TestRenamingWithoutAFolderChangeKeepsTheSlug is the other half: a plain
// title change stays in place, so the file — and the git history keyed to its
// path — is left where it was.
func TestRenamingWithoutAFolderChangeKeepsTheSlug(t *testing.T) {
	seedDeck(t, "old-title", "name: old title\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.DeleteCommitted("old-title") })

	e := deckEntry{kind: entryLocal, name: "old title", slug: "old-title"}
	if msg, ok := renameCmd(e, "new title")().(noticeMsg); !ok || msg.err != nil {
		t.Fatalf("rename returned %#v", msg)
	}

	if !deck.Exists("old-title") {
		t.Error("the slug changed on a rename that didn't change the folder")
	}
	d, err := deck.Read("old-title")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "new title" {
		t.Errorf("name header is %q, want %q", d.Name, "new title")
	}
}
