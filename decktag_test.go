package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// taggableDeck is a deck with enough in it to filter and tag: three fliers
// and two things that aren't.
func taggableDeck(t *testing.T) model {
	t.Helper()
	gitRepo(t)

	cards := []deckCard{
		{Card: ScryfallCard{Name: "Dragonlord Ojutai", TypeLine: "Creature — Dragon", OracleText: "Flying\nHexproof"}, Qty: 1},
		{Card: ScryfallCard{Name: "Goldspan Dragon", TypeLine: "Creature — Dragon", OracleText: "Flying, haste"}, Qty: 1},
		{Card: ScryfallCard{Name: "Serra Angel", TypeLine: "Creature — Angel", OracleText: "Flying, vigilance"}, Qty: 1},
		{Card: ScryfallCard{Name: "Sol Ring", TypeLine: "Artifact", OracleText: "{T}: Add {C}{C}."}, Qty: 1, Tags: []string{"ramp"}},
		{Card: ScryfallCard{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 7},
	}

	seedCache(t, map[string]ScryfallCard{})
	d := deckFileFrom(deckInfo{Name: "Fliers", Format: "commander"}, cards)
	if _, _, err := saveDeckVersioned("fliers", d); err != nil {
		t.Fatal(err)
	}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 190, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{Name: "Fliers", Slug: "fliers", Format: "commander", Total: 11, Unique: 5},
		cards: cards,
	})
	return m
}

// typeFilter types into the list's fuzzy filter. It has to run commands:
// bubbles applies a filter through one, so a filter typed with press alone
// never actually narrows anything.
func typeFilter(m model, text string) model {
	runes := []rune(text)
	m = press(m, "/")
	// Only the last keystroke's filter matters, and running commands for
	// each of them costs a 100ms hover tick apiece.
	for _, r := range runes[:len(runes)-1] {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: runes[len(runes)-1:]})
	return drive(m, tea.KeyMsg{Type: tea.KeyEnter})
}

func tagsOf(m model, name string) []string {
	if i := m.deckIndexOf(name); i >= 0 {
		return m.deckCards[i].Tags
	}
	return nil
}

func TestMarkingWithSpace(t *testing.T) {
	m := taggableDeck(t)
	first, _ := m.active().selected()

	m = press(m, " ")
	if !m.marked(first.Card) {
		t.Errorf("space did not mark %q", first.Card.Name)
	}
	// The cursor steps on, so marking a run is one key held down.
	second, _ := m.active().selected()
	if second.Card.Name == first.Card.Name {
		t.Error("space did not move to the next card")
	}

	// And it toggles.
	m = press(m, " ")
	m.deckPane.list.Select(0)
	m = press(m, " ")
	if m.marked(first.Card) {
		t.Error("space did not unmark on a second press")
	}
}

func TestMarkVisibleThenTag(t *testing.T) {
	// The workflow this exists for: filter to flying, mark that lot, tag
	// them all.
	m := taggableDeck(t)

	m = typeFilter(m, "flying")

	visible := m.active().list.VisibleItems()
	if len(visible) != 3 {
		t.Fatalf("the filter matched %d cards, want the 3 fliers", len(visible))
	}

	m = press(m, "v")
	if m.markCount() != 3 {
		t.Fatalf("v marked %d cards, want 3", m.markCount())
	}

	m = press(m, "T")
	if !m.tagging {
		t.Fatal("T did not open the tag prompt")
	}
	for _, r := range "flying" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.tagging {
		t.Error("the prompt stayed open")
	}
	for _, name := range []string{"Dragonlord Ojutai", "Goldspan Dragon", "Serra Angel"} {
		tags := tagsOf(m, name)
		if len(tags) != 1 || tags[0] != "flying" {
			t.Errorf("%s has tags %v, want [flying]", name, tags)
		}
	}
	// And nothing else was touched.
	if tags := tagsOf(m, "Sol Ring"); len(tags) != 1 || tags[0] != "ramp" {
		t.Errorf("Sol Ring's tags changed: %v", tags)
	}
	if tags := tagsOf(m, "Plains"); len(tags) != 0 {
		t.Errorf("Plains was tagged: %v", tags)
	}
	if !strings.Contains(m.notice, "3 cards") {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestTagPromptRemovesWithALeadingDash(t *testing.T) {
	m := taggableDeck(t)
	selectCard(&m.deckPane, "Sol Ring")

	m = press(m, "T")
	for _, r := range "-ramp" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if tags := tagsOf(m, "Sol Ring"); len(tags) != 0 {
		t.Errorf("-ramp left %v", tags)
	}
	if !strings.Contains(m.notice, "-ramp") {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestTagPromptTakesSeveralAtOnce(t *testing.T) {
	m := taggableDeck(t)
	selectCard(&m.deckPane, "Sol Ring")

	m = press(m, "T")
	for _, r := range "artifact, fast, -ramp" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	tags := tagsOf(m, "Sol Ring")
	if strings.Join(tags, ",") != "artifact,fast" {
		t.Errorf("tags = %v, want [artifact fast]", tags)
	}
}

func TestTagWithNothingMarkedTagsTheCursor(t *testing.T) {
	m := taggableDeck(t)
	selectCard(&m.deckPane, "Serra Angel")

	m = press(m, "T")
	for _, r := range "flying" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if tags := tagsOf(m, "Serra Angel"); len(tags) != 1 {
		t.Errorf("the card under the cursor was not tagged: %v", tags)
	}
	// Deliberately not everything the filter is showing — v is for that,
	// and tagging a hundred cards because a filter was still on would be a
	// nasty surprise.
	if tags := tagsOf(m, "Goldspan Dragon"); len(tags) != 0 {
		t.Errorf("tagging the cursor also tagged %v", tags)
	}
}

func TestMarksSurviveFilteringAndEditing(t *testing.T) {
	// Marks are held by name, not by position, so a filter that hides a
	// marked card — or an edit that rebuilds the list — can't lose it or
	// move it onto whatever ends up in that row instead.
	m := taggableDeck(t)
	selectCard(&m.deckPane, "Serra Angel")
	m = press(m, " ")
	if m.markCount() != 1 {
		t.Fatalf("%d marks, want 1", m.markCount())
	}

	// Filter to something the marked card isn't in. The filter is fuzzy and
	// matches oracle text too, so "sol" would also catch Goldspan Dragon
	// (s…o…l across "Goldspan Dragon Flying") — this one is unambiguous.
	m = typeFilter(m, "plains")
	for _, it := range m.active().list.VisibleItems() {
		if ci, ok := it.(cardItem); ok && ci.Card.Name == "Serra Angel" {
			t.Fatal("the filter did not hide the marked card")
		}
	}
	if m.markCount() != 1 {
		t.Errorf("filtering lost the mark")
	}

	// An edit rebuilds the whole list underneath it.
	m = press(m, "+")

	if !m.marked(ScryfallCard{Name: "Serra Angel"}) {
		t.Error("the mark did not survive a filter and an edit")
	}
	if m.marked(ScryfallCard{Name: "Plains"}) {
		t.Error("the mark moved onto the card that took its place")
	}
	if m.markCount() != 1 {
		t.Errorf("%d marks after an edit, want 1", m.markCount())
	}
}

func TestClearingMarks(t *testing.T) {
	m := taggableDeck(t)
	m = press(m, "v")
	if m.markCount() == 0 {
		t.Fatal("nothing was marked")
	}

	m = press(m, "V")
	if m.markCount() != 0 {
		t.Errorf("V left %d marks", m.markCount())
	}

	// esc clears them too, before it touches the filter.
	m = press(m, "v")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.markCount() != 0 {
		t.Errorf("esc left %d marks", m.markCount())
	}
}

func TestBulkTagIsOneCommit(t *testing.T) {
	m := taggableDeck(t)
	before, err := deckHistory("fliers", 20)
	if err != nil {
		t.Fatal(err)
	}

	m = press(m, "v") // all five
	m = press(m, "T")
	for _, r := range "reviewed" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(m, deckSaveTickMsg{seq: m.deckSeq})

	after, err := deckHistory("fliers", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("tagging five cards made %d commits, want 1", len(after)-len(before))
	}
	// The subject counts them rather than listing five names.
	if !strings.Contains(after[0].Subject, "retagged") {
		t.Errorf("commit subject = %q", after[0].Subject)
	}

	on, err := readDeck("fliers")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range on.Entries {
		var found bool
		for _, tag := range e.Tags {
			if tag == "reviewed" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s was not tagged in the file: %v", e.Name, e.Tags)
		}
	}
}

func TestTaggingACardNotInTheDeck(t *testing.T) {
	// Marks can be made in the search results, where a card may not be in
	// the deck at all. Tags live in the deck file, so there's nowhere to
	// put one — and that has to be said rather than silently dropped.
	m := taggableDeck(t)
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	m = press(m, "v") // marks the search results
	if m.markCount() == 0 {
		t.Fatal("nothing was marked")
	}

	m = press(m, "T")
	for _, r := range "x" {
		m = press(m, string(r))
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(m.notice, "not in the deck") {
		t.Errorf("notice = %q, should say the cards aren't in the deck", m.notice)
	}
}

func TestTaggingNeedsAnEditableDeck(t *testing.T) {
	gitRepo(t)
	t.Setenv("HOME", t.TempDir())

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 190, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{Name: "Someone else's", ID: "Y8dZ7", URL: "https://moxfield.com/decks/Y8dZ7"},
		cards: deckFixture(),
	})

	m = press(m, "T")
	if m.tagging {
		t.Error("the tag prompt opened on a deck that can't be saved")
	}
	if !strings.Contains(m.notice, ",i to make it yours") {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestApplyTagEdits(t *testing.T) {
	tests := []struct {
		name        string
		tags        []string
		add, remove []string
		want        string
	}{
		{"add to none", nil, []string{"flying"}, nil, "flying"},
		{"add keeps existing", []string{"ramp"}, []string{"artifact"}, nil, "artifact,ramp"},
		{"adding twice is once", []string{"ramp"}, []string{"ramp"}, nil, "ramp"},
		{"remove", []string{"ramp", "artifact"}, nil, []string{"ramp"}, "artifact"},
		{"remove the last leaves none", []string{"ramp"}, nil, []string{"ramp"}, ""},
		{"removing what isn't there", []string{"ramp"}, nil, []string{"flying"}, "ramp"},
		{"add and remove at once", []string{"ramp"}, []string{"fast"}, []string{"ramp"}, "fast"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(applyTagEdits(tt.tags, tt.add, tt.remove), ",")
			if got != tt.want {
				t.Errorf("applyTagEdits(%v, +%v, -%v) = %q, want %q",
					tt.tags, tt.add, tt.remove, got, tt.want)
			}
		})
	}
}

func TestMarksShowInTheList(t *testing.T) {
	m := taggableDeck(t)
	m = press(m, " ")

	view := stripANSI(m.View())
	if !strings.Contains(view, "●") {
		t.Error("a marked card is not shown as marked")
	}
	if !strings.Contains(view, "1 marked") {
		t.Error("the header does not say how many are marked")
	}
}

func TestMarkGutterKeepsRowsAligned(t *testing.T) {
	// The gutter is drawn whether or not the card is marked, so rows don't
	// shift sideways underneath you as you mark them.
	card := ScryfallCard{Name: "Serra Angel", TypeLine: "Creature — Angel", ManaCost: "{3}{W}{W}"}
	l := list.New([]list.Item{cardItem{Card: card}}, compactDelegate{}, 60, 4)

	var plain, marked strings.Builder
	compactDelegate{}.Render(&plain, l, 0, cardItem{Card: card})
	compactDelegate{marks: map[string]bool{"serra angel": true}}.
		Render(&marked, l, 0, cardItem{Card: card})

	if a, b := visibleLen(plain.String()), visibleLen(marked.String()); a != b {
		t.Errorf("marking changed the row width: %d vs %d\n%q\n%q",
			a, b, stripANSI(plain.String()), stripANSI(marked.String()))
	}
	if !strings.Contains(stripANSI(marked.String()), "●") {
		t.Error("the marked row has no mark on it")
	}
}
