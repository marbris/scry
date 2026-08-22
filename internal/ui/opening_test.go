package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"scry/internal/deck"
	"scry/internal/moxfield"
	"scry/internal/mtg"
)

// seedCache writes a card cache so a deck can be opened without the network.
func seedCache(t *testing.T, cards map[string]mtg.Card) {
	t.Helper()
	body, err := json.Marshal(cards)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deck.CachePath(), body, 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(deck.CachePath()) })
}

// decksPanel opens the decks panel over one seeded deck.
func decksPanel(t *testing.T) (Model, *deckList) {
	t.Helper()
	seedCache(t, map[string]mtg.Card{
		"sol ring":             {Name: "Sol Ring", TypeLine: "Artifact"},
		"ghen, arcanum weaver": {Name: "Ghen, Arcanum Weaver", TypeLine: "Legendary Creature — Human"},
	})
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[commander]\n1 Ghen, Arcanum Weaver\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m := drive(sized(160, 30), "space", "d")
	return m, m.ws.current().top().(*deckList)
}

func TestEnterOpensADeckInThePanelYouAreIn(t *testing.T) {
	// Stepping into it, so esc goes back to the list rather than closing.
	m, l := decksPanel(t)
	l.cursor.at = 0

	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter did nothing")
	}
	m = settle(m, cmd)

	p := m.ws.current()
	if m.ws.count() != 1 {
		t.Errorf("enter opened %d panels", m.ws.count())
	}
	if len(p.stack) != 2 {
		t.Fatalf("the deck did not go on top of the list: %d views", len(p.stack))
	}
	if p.cardsView() == nil {
		t.Fatal("no cards")
	}

	// And esc comes back out to the decks list.
	m = drive(m, "esc")
	if _, ok := m.ws.current().top().(*deckList); !ok {
		t.Errorf("esc left the panel showing %T", m.ws.current().top())
	}
}

func TestLOpensADeckBesideAndLeavesYouWhereYouWere(t *testing.T) {
	// So the list you were browsing is still under the cursor.
	m, l := decksPanel(t)
	l.cursor.at = 0

	m, cmd := press(m, "L")
	if cmd == nil {
		t.Fatal("L did nothing")
	}
	m = settle(m, cmd)

	if m.ws.count() != 2 {
		t.Fatalf("L opened %d panels", m.ws.count())
	}
	if m.ws.focused != 0 {
		t.Errorf("focus moved to panel %d", m.ws.focused)
	}
	if _, ok := m.ws.panels[0].top().(*deckList); !ok {
		t.Error("the decks list is no longer under the cursor")
	}
	if m.ws.panels[1].cardsView() == nil {
		t.Error("the new panel has no cards")
	}
}

func TestAnOpenedDeckBecomesTheOneBeingEdited(t *testing.T) {
	m, l := decksPanel(t)
	l.cursor.at = 0

	m, cmd := press(m, "enter")
	m = settle(m, cmd)

	if m.ws.editing != 0 {
		t.Errorf("the editing deck is panel %d", m.ws.editing)
	}
	if _, why := m.editTarget(); why != "" {
		t.Errorf("it can't be edited: %s", why)
	}
}

func TestOpeningADeckWhoseCardsAreUnknownStillOpensIt(t *testing.T) {
	// A deck that opens missing its two newest cards beats one that won't
	// open at all.
	seedCache(t, map[string]mtg.Card{"sol ring": {Name: "Sol Ring"}})
	seedDeck(t, "part", "name: Part\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("part") })

	m := sized(160, 30)
	p := m.ws.open(KindDecks)
	next, _ := m.Update(deckOpenedMsg{
		panel: p.id,
		info:  deck.Info{Name: "Part", Slug: "part"},
		cards: []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}},
		err:   deck.UnresolvedError{Names: []string{"Nonesuch"}},
	})
	m = next.(Model)

	if p.cardsView() == nil {
		t.Fatal("the deck did not open")
	}
	if !strings.Contains(stripANSI(m.View()), "Nonesuch") {
		t.Errorf("nothing said which card was missing:\n%s", stripANSI(m.View()))
	}
}

func TestAPersonsDecksOpenAsASubView(t *testing.T) {
	m := sized(160, 30)
	p := m.ws.open(KindDecks)
	p.show(newDeckList())

	next, _ := m.Update(userDecksMsg{
		panel: p.id, user: "MarBri",
		decks: []moxfield.UserDeck{
			{Name: "Hinata", PublicID: "abc", Cards: 100},
			{Name: "Elf Ball", PublicID: "def", Cards: 100},
		},
	})
	m = next.(Model)

	if len(p.stack) != 2 {
		t.Fatalf("the decks went somewhere other than on top: %d views", len(p.stack))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Hinata") || !strings.Contains(view, "MarBri") {
		t.Errorf("got:\n%s", view)
	}

	// esc comes back to your own decks.
	m = drive(m, "esc")
	if _, ok := m.ws.current().top().(*deckList); !ok {
		t.Errorf("esc left the panel showing %T", m.ws.current().top())
	}
}

func TestDeletingADeckActuallyDeletesIt(t *testing.T) {
	m, l := decksPanel(t)
	l.cursor.at = 0
	if !deck.Exists("ghen") {
		t.Fatal("the deck wasn't there to begin with")
	}

	m = drive(m, "d")
	m, cmd := press(m, "y")
	if cmd == nil {
		t.Fatal("y did nothing")
	}
	m = settle(m, cmd)

	if deck.Exists("ghen") {
		t.Error("the deck is still there")
	}
	if !strings.Contains(stripANSI(m.View()), "deleted") {
		t.Errorf("nothing said so:\n%s", stripANSI(m.View()))
	}
}

func TestRenamingADeckChangesItsTitleAndNotItsFile(t *testing.T) {
	// The slug names a file with a git history; renaming that would lose
	// the history rather than move it.
	m, l := decksPanel(t)
	l.cursor.at = 0

	msg := renameCmd(deckEntry{kind: entryLocal, slug: "ghen", name: "Ghen"}, "Ghen Reborn")().(noticeMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if !deck.Exists("ghen") {
		t.Error("the file moved")
	}
	d, err := deck.Read("ghen")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Ghen Reborn" {
		t.Errorf("the title is %q", d.Name)
	}
	_ = m
}

func TestCWithNoDeckOpenDoesNothing(t *testing.T) {
	// c is a role in a deck you're editing. With no such deck open it says
	// so rather than conjuring a deck up around the card.
	m := withCards(sized(140, 30), "f", []deck.Card{
		{Card: mtg.Card{Name: "Ghen, Arcanum Weaver", TypeLine: "Legendary Creature — Human"}},
	}, sortArrival)

	m, cmd := press(m, "c")
	if cmd != nil {
		t.Fatal("c ran a command with no deck open")
	}
	if deck.Exists("ghen-arcanum-weaver") {
		deck.Delete("ghen-arcanum-weaver")
		t.Fatal("c made a deck when it should have done nothing")
	}
	if !strings.Contains(stripANSI(m.View()), "no deck open to edit") {
		t.Errorf("c did not explain itself:\n%s", stripANSI(m.View()))
	}
}

func TestSaveEverythingWritesEveryDeckWithOutstandingEdits(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "one", "name: One\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	seedDeck(t, "two", "name: Two\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("one"); deck.Delete("two") })

	m := sized(200, 30)
	m, a := openDeckPanel(t, m, "one", "One", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}})
	m, b := openDeckPanel(t, m, "two", "Two", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}})

	a.all = append(a.all, deck.Card{Qty: 1, Card: mtg.Card{Name: "Llanowar Elves"}})
	a.dirty = true
	b.all = append(b.all, deck.Card{Qty: 1, Card: mtg.Card{Name: "Birds of Paradise"}})
	b.dirty = true

	if got := len(m.dirtyDecks()); got != 2 {
		t.Fatalf("%d decks are outstanding", got)
	}
	m = settle(m, m.saveEverything())

	for slug, card := range map[string]string{"one": "Llanowar Elves", "two": "Birds of Paradise"} {
		d, err := deck.Read(slug)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(d.String(), card) {
			t.Errorf("%s was not written:\n%s", slug, d.String())
		}
	}
}

func TestAPersonsDecksCanBeNarrowed(t *testing.T) {
	// Somebody with two hundred decks is exactly who / is for.
	m := sized(160, 30)
	p := m.ws.open(KindDecks)
	p.show(newDeckList())

	next, _ := m.Update(userDecksMsg{
		panel: p.id, user: "MarBri",
		decks: []moxfield.UserDeck{
			{Name: "Elf Ball", PublicID: "a", Cards: 100},
			{Name: "Goblin Rush", PublicID: "b", Cards: 100},
			{Name: "Elfball Redux", PublicID: "c", Cards: 100},
		},
	})
	m = next.(Model)

	m = drive(m, "/", "e", "l", "f", "enter")
	v := p.top().(*userDeckList)
	if len(v.decks) != 2 {
		t.Errorf("filtering to 'elf' left %d decks", len(v.decks))
	}

	// b clears the filter; esc would step back to your own decks instead.
	m = drive(m, "b")
	if len(v.decks) != 3 {
		t.Errorf("b left %d decks", len(v.decks))
	}
}

func TestFollowingFromAUsersDecksReachesTheListBeneath(t *testing.T) {
	// A user's decks sit on top of your own decks in the same panel, so a
	// reload has to walk the whole stack — otherwise a deck followed or copied
	// from up there isn't in the list esc steps back down to.
	t.Cleanup(func() { deck.SaveBookmarks(deck.Bookmarks{}) })

	m := sized(160, 30)
	p := m.ws.open(KindDecks)
	dl := newDeckList()
	p.show(dl)
	p.push(newUserDeckList("MarBri", []moxfield.UserDeck{
		{Name: "Elf Ball", PublicID: "a", Cards: 100},
	}))

	// Following writes the bookmark, then asks for a reload.
	var b deck.Bookmarks
	b.AddRemote(deck.Remote{Name: "Elf Ball", ID: "a"})
	if err := deck.SaveBookmarks(b); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(reloadDecksMsg{})
	m = next.(Model)

	found := false
	for _, e := range dl.all {
		if e.kind == entryRemote && e.id == "a" {
			found = true
		}
	}
	if !found {
		t.Errorf("the reload did not reach the decks list under the sub-view: %v", dl.all)
	}
}

func TestOpeningARemoteDeckFollowsIt(t *testing.T) {
	// Looking at somebody's deck is how you decide to follow it, and the
	// alternative is finding your way back to a deck you saw once and can't
	// name.
	t.Cleanup(func() { deck.SaveBookmarks(deck.Bookmarks{}) })

	var b deck.Bookmarks
	b.AddRemote(deck.Remote{Name: "Hinata", ID: "abc"})
	if err := deck.SaveBookmarks(b); err != nil {
		t.Fatal(err)
	}

	// A remote in the bookmarks appears in the decks list as R.
	l := newDeckList()
	found := false
	for _, e := range l.all {
		if e.kind == entryRemote && e.id == "abc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the followed deck is not listed: %v", l.all)
	}
	if got := stripANSI(renderEntry(deckEntry{kind: entryRemote, name: "Hinata", id: "abc"}, 30, false)); !strings.Contains(got, "R") {
		t.Errorf("it is not marked as remote: %q", got)
	}
}
