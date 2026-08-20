package ui

import (
	"strings"
	"testing"

	"scry/internal/deck"
)

func TestTheLeaderMenuShowsEveryEntry(t *testing.T) {
	// The entries you can't see are exactly the ones you opened the menu to
	// read, so it wraps rather than being cut off.
	for _, width := range []int{50, 70, 100, 160} {
		m := withCards(sized(width, 24), "f", sample(), sortArrival)
		m = drive(m, "space")

		bar := stripANSI(strings.Join(m.leaderBarLines(), " "))
		for _, c := range leaderMenu {
			if !strings.Contains(bar, c.what) {
				t.Errorf("at %d columns %q is missing from the menu:\n%s",
					width, c.what, bar)
			}
		}
	}
}

func TestTheMenuNeverPushesThePanelsOffTheScreen(t *testing.T) {
	// It used to reserve one row however many it took, and the frame grew
	// past the terminal — the panels lost their top edge.
	for _, width := range []int{40, 50, 70, 100, 200} {
		for _, height := range []int{10, 14, 24, 40} {
			m := withCards(sized(width, height), "f", sample(), sortArrival)
			m = drive(m, "space")

			lines := splitLines(m.View())
			if len(lines) > height {
				t.Errorf("%dx%d: the menu grew the frame to %d lines",
					width, height, len(lines))
			}
			for i, line := range lines {
				if w := visibleWidth(line); w > width {
					t.Errorf("%dx%d: line %d is %d columns", width, height, i, w)
				}
			}
		}
	}
}

func TestEveryMenuKeyPutsTheMenuAway(t *testing.T) {
	// Including the ones that go on to do something else.
	for _, c := range leaderMenu {
		m := withCards(sized(140, 30), "f", sample(), sortArrival)
		m = drive(m, "space")
		if !m.leader {
			t.Fatal("space did not raise the menu")
		}
		m = drive(m, c.key)
		if m.leader {
			t.Errorf("the menu stayed up after %q", c.key)
		}
	}
}

func TestTheReferenceDescribesThePanelYouAreIn(t *testing.T) {
	// A reference listing keys that do nothing here is worse than none.
	search := withCards(sized(140, 40), "f", sample(), sortArrival)
	searchKeys := keyText(search)

	own := withCards(sized(140, 40), "d", sample(), sortArrival)
	own.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	// What opening a deck does, which this bypasses by building the list
	// directly. The editing keys are listed against the deck they change,
	// so there has to be one.
	own.ws.editing = own.ws.focused
	deckKeys := keyText(own)

	if !strings.Contains(deckKeys, "This deck") {
		t.Errorf("a deck of yours is headed:\n%s", deckKeys)
	}
	if !strings.Contains(searchKeys, "These cards") {
		t.Errorf("a search is headed:\n%s", searchKeys)
	}

	// Editing keys are listed against the deck they change, and not offered
	// at all with no deck to change: on a search with nothing being edited,
	// every one of them would do nothing but explain itself.
	for _, only := range []string{"tag, tag again", "undo"} {
		if !strings.Contains(deckKeys, only) {
			t.Errorf("%q is missing from a deck's keys:\n%s", only, deckKeys)
		}
		if strings.Contains(searchKeys, only) {
			t.Errorf("%q is offered on a search with no deck being edited", only)
		}
	}
	// And they name that deck, so you can see where the card is going from
	// a panel that isn't it.
	if !strings.Contains(deckKeys, "this deck") {
		t.Errorf("the editing keys don't say which deck they change:\n%s", deckKeys)
	}
}

func TestTheDecksPanelHasItsOwnKeys(t *testing.T) {
	m := drive(sized(140, 40), "space", "d")
	got := keyText(m)

	if !strings.Contains(got, "Your decks") {
		t.Errorf("headed:\n%s", got)
	}
	for _, want := range []string{"follow a deck", "new deck", "rename"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing:\n%s", want, got)
		}
	}
}

func TestTheBarsKeysAreOnTheHintLineNotInTheReference(t *testing.T) {
	// ? is a printable key, so while you're typing it types. The bar's own
	// keys belong where you can see them without stopping.
	m := drive(sized(140, 40), "space", "f")
	if !strings.Contains(stripANSI(m.View()), "another target") {
		t.Errorf("the hint line does not describe the bar:\n%s", stripANSI(m.View()))
	}

	m = drive(m, "?")
	if got := m.ws.current().search.Value(); got != "?" {
		t.Errorf("? did something other than type: bar holds %q", got)
	}
}

func TestTheReferenceFitsTheScreen(t *testing.T) {
	// It laid itself out in one column and ran off the bottom, which loses
	// exactly the part you were reaching for.
	for _, size := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {200, 50}, {70, 16}} {
		m := withCards(sized(size[0], size[1]), "d", sample(), sortArrival)
		m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
		m = drive(m, "?")

		lines := splitLines(m.View())
		if len(lines) > size[1] {
			t.Errorf("%dx%d: the reference is %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := visibleWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d is %d columns", size[0], size[1], i, w)
			}
		}
	}
}

func TestTheReferenceKeepsEverySectionWhole(t *testing.T) {
	// Splitting a block across two columns would be worse than a third
	// column.
	sections := []keySection{
		{"one", [][2]string{{"a", "x"}, {"b", "y"}}},
		{"two", [][2]string{{"c", "z"}}},
		{"three", [][2]string{{"d", "w"}, {"e", "v"}, {"f", "u"}}},
	}
	groups := packSections(sections, 5)
	if len(groups) < 2 {
		t.Fatalf("everything fitted in %d column(s) of 5 lines", len(groups))
	}

	total := 0
	for _, g := range groups {
		for _, s := range g {
			total++
			_ = s
		}
	}
	if total != len(sections) {
		t.Errorf("%d sections came out of %d", total, len(sections))
	}
}

// keyText is the reference as plain text.
func keyText(m Model) string {
	m = drive(m, "?")
	return stripANSI(m.View())
}

// footerOf is the bottom of the screen: the hint bar and the status beside
// it, which is what the reference has to agree with.
func footerOf(m Model) string {
	l := m.ws.layoutWithFooter(m.footerHeight())
	return stripANSI(m.viewFooter(l))
}

func TestTheHintBarDoesNotOfferStatisticsWhereThereAreNone(t *testing.T) {
	// s counts a list of cards. A decks panel has none, and the bar used to
	// offer it there anyway, because it picked one of three fixed strings.
	for _, key := range []string{"d", "r"} {
		m := drive(sized(120, 30), "space", key)
		if strings.Contains(footerOf(m), "statistics") {
			t.Errorf("<space>%s offers statistics:\n%s", key, footerOf(m))
		}
	}

	cards := withCards(sized(120, 30), "f", sample(), sortArrival)
	if !strings.Contains(footerOf(cards), "statistics") {
		t.Errorf("a list of cards doesn't offer statistics:\n%s", footerOf(cards))
	}
}

func TestTheHintBarSaysWhichDeckTheEditingKeysChange(t *testing.T) {
	// a, x, t, T, c and u act on the editing deck, not on the list under
	// the cursor. Reading "add" over somebody else's deck and watching the
	// card land elsewhere is what this names away.
	m := withCards(sized(140, 30), "d", sample(), sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	m.ws.editing = m.ws.focused

	// A second panel, so the editing deck is not the one in front of you.
	m = withCards(m, "f", sample(), sortArrival)
	got := footerOf(m)
	if !strings.Contains(got, "Ghen") {
		t.Errorf("the editing keys don't name the deck they change:\n%s", got)
	}
	if !strings.Contains(got, "add, remove a copy — Ghen") {
		t.Errorf("a and x don't say where the card goes:\n%s", got)
	}
}

func TestNoEditingDeckMeansNoEditingKeys(t *testing.T) {
	// With nothing being edited they do nothing but explain themselves, so
	// what is offered instead is the way to choose a deck.
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	got := footerOf(m)
	for _, gone := range []string{"add, remove a copy", "tag, tag again", "undo"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q offered with no deck being edited:\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "choose a deck to edit") {
		t.Errorf("no way offered to choose one:\n%s", got)
	}
}

func TestTheHintBarAndTheReferenceAgree(t *testing.T) {
	// Two lists describing one keymap is how one of them comes to lie. Every
	// key the reference names has to be on the bar, in every context.
	cases := map[string]Model{
		"decks":   drive(sized(140, 40), "space", "d"),
		"rules":   drive(sized(140, 40), "space", "r"),
		"search":  withCards(sized(140, 40), "f", sample(), sortArrival),
		"a deck":  editingDeckModel(),
		"stats":   drive(editingDeckModel(), "s"),
		"the bar": drive(sized(140, 40), "space", "f"),
	}
	for name, m := range cases {
		bar := footerOf(m)
		for _, row := range m.contextKeys() {
			if !strings.Contains(bar, row[1]) {
				t.Errorf("%s: the reference offers %q (%s) and the bar doesn't:\n%s",
					name, row[1], row[0], bar)
			}
		}
	}
}

func editingDeckModel() Model {
	m := withCards(sized(140, 40), "d", sample(), sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	m.ws.editing = m.ws.focused
	return m
}

func TestNoKeyIsOfferedTwice(t *testing.T) {
	// The decks panel listed i twice: the view claimed it and the panel
	// added it again with a different label.
	cases := map[string]Model{
		"decks":  drive(sized(140, 40), "space", "d"),
		"rules":  drive(sized(140, 40), "space", "r"),
		"search": withCards(sized(140, 40), "f", sample(), sortArrival),
		"a deck": editingDeckModel(),
	}
	for name, m := range cases {
		seen := map[string]string{}
		for _, row := range m.contextKeys() {
			if was, dup := seen[row[0]]; dup {
				t.Errorf("%s: %q offered as %q and again as %q", name, row[0], was, row[1])
			}
			seen[row[0]] = row[1]
		}
	}
}

func TestTheNoticeSitsBesideTheKeysAndGoesAway(t *testing.T) {
	// It used to replace the hint line rather than sit beside it, so the
	// result of the last thing you did stood on top of the keys for the
	// next — and nothing cleared it, so it stood there for good.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m.notice = "+1 Sol Ring"

	got := footerOf(m)
	if !strings.Contains(got, "+1 Sol Ring") {
		t.Fatalf("the notice isn't shown:\n%s", got)
	}
	if !strings.Contains(got, "up and down") {
		t.Errorf("the notice is standing on the keys:\n%s", got)
	}

	// Nothing on the first line runs into the notice.
	first := strings.Split(got, "\n")[0]
	if strings.Contains(first, "· +1 Sol Ring") {
		t.Errorf("the notice is packed in among the keys:\n%s", first)
	}

	m = drive(m, "j")
	if m.notice != "" {
		t.Errorf("the next key left the notice up: %q", m.notice)
	}
}

func TestAKeyThatSetsANoticeKeepsIt(t *testing.T) {
	// Clearing happens before the key is dispatched, so a key that says
	// something still gets to say it.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, "p") // nothing yanked, and nowhere to put it
	if m.notice == "" {
		t.Error("the key said nothing")
	}
}
