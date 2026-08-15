package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// openHistoryOn builds an app with a local deck open and a couple of commits
// behind it, then opens the history with ,g.
func openHistoryOn(t *testing.T) model {
	t.Helper()
	gitRepo(t)
	t.Setenv("HOME", t.TempDir())

	if _, _, err := saveDeckVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}
	changed := deckOf(t, strings.Replace(gitBaseDeck, "1 Sol Ring [ramp]", "1 Mana Crypt [ramp]", 1))
	if _, _, err := saveDeckVersioned("ghen", changed); err != nil {
		t.Fatal(err)
	}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Ghen", slug: "ghen", format: "commander", total: 10},
		cards: deckFixture(),
	})
	return leaderPress(m, "g")
}

func TestDeckHistoryOpens(t *testing.T) {
	m := openHistoryOn(t)

	if m.state != stateDeckHistory {
		t.Fatalf(",g left the app in state %v", m.state)
	}
	if m.historyErr != nil {
		t.Fatalf("history errored: %v", m.historyErr)
	}
	if got := len(m.historyList.Items()); got != 2 {
		t.Fatalf("got %d commits, want 2", got)
	}

	// Newest first, and it says what changed.
	first, ok := m.historyList.Items()[0].(commitItem)
	if !ok {
		t.Fatal("history list holds something other than commits")
	}
	if first.commit.subject != "+Mana Crypt, -Sol Ring" {
		t.Errorf("newest commit = %q", first.commit.subject)
	}

	// The diff for the selected commit is fetched without being asked for.
	if !strings.Contains(m.historyDiff, "Mana Crypt") {
		t.Errorf("diff does not show the change:\n%s", m.historyDiff)
	}

	// And it renders.
	view := stripANSI(m.viewDeckHistory())
	if !strings.Contains(view, "Mana Crypt") {
		t.Errorf("the view doesn't show the diff:\n%s", view)
	}
	if !strings.Contains(view, "History (2)") {
		t.Errorf("the view doesn't title itself:\n%s", view)
	}

	// esc goes back to the deck.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateResults {
		t.Errorf("esc left the app in state %v", m.state)
	}
}

func TestDeckHistoryFollowsTheCursor(t *testing.T) {
	m := openHistoryOn(t)

	// Moving down to the first commit swaps the diff for that one's.
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	sel, ok := m.historyList.SelectedItem().(commitItem)
	if !ok {
		t.Fatal("nothing selected")
	}
	if sel.commit.subject != "Add Ghen" {
		t.Fatalf("selected %q, want the first commit", sel.commit.subject)
	}
	// The deck being created shows up as the whole file being added.
	if !strings.Contains(m.historyDiff, "Ghen, Arcanum Weaver") {
		t.Errorf("diff didn't follow the cursor:\n%s", m.historyDiff)
	}
}

func TestDeckHistoryRestores(t *testing.T) {
	m := openHistoryOn(t)

	// Restore the older version, which still has Sol Ring in it.
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != stateResults {
		t.Errorf("restoring left the app in state %v", m.state)
	}

	back, err := readDeck("ghen")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(back.String(), "Sol Ring") {
		t.Errorf("the old version wasn't restored:\n%s", back.String())
	}

	// Nothing was rewritten — restoring is a new commit.
	commits, _ := deckHistory("ghen", 10)
	if len(commits) != 3 {
		t.Errorf("got %d commits after restoring, want 3", len(commits))
	}
}

func TestDeckHistoryNeedsADeckOfYourOwn(t *testing.T) {
	gitRepo(t)
	t.Setenv("HOME", t.TempDir())

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// On search results there's no deck at all.
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	m = leaderPress(m, "g")
	if m.state == stateDeckHistory {
		t.Error(",g opened a history for search results")
	}
	if !strings.Contains(m.notice, "no history") {
		t.Errorf("notice = %q", m.notice)
	}

	// A deck browsed off Moxfield isn't yours, so it has no history either.
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Someone else's", id: "Y8dZ7", url: "https://moxfield.com/decks/Y8dZ7"},
		cards: deckFixture(),
	})
	m = leaderPress(m, "g")
	if m.state == stateDeckHistory {
		t.Error(",g opened a history for a deck browsed off Moxfield")
	}
	if !strings.Contains(m.notice, "your own") {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestRenderDiffHighlights(t *testing.T) {
	diff := `diff --git a/ghen.deck b/ghen.deck
index 1234567..89abcde 100644
--- a/ghen.deck
+++ b/ghen.deck
@@ -7,7 +7,7 @@ format: commander
 1 Arcane Signet [ramp]
-1 Sol Ring [ramp]
+1 Mana Crypt [ramp]
 7 Plains`

	out := stripANSI(renderDiff(diff, 60))

	// The noise git puts at the top of every diff says nothing the commit
	// list hasn't already.
	for _, unwanted := range []string{"diff --git", "index 1234567", "--- a/", "+++ b/"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("diff header %q was not dropped:\n%s", unwanted, out)
		}
	}
	for _, want := range []string{"-1 Sol Ring", "+1 Mana Crypt", "@@", "7 Plains"} {
		if !strings.Contains(out, want) {
			t.Errorf("diff is missing %q:\n%s", want, out)
		}
	}
}
