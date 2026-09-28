package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/deck"
	"scry/internal/mtg"
)

// tagging opens the tag prompt on a deck whose cards carry some tags already,
// and types what's given.
func tagging(t *testing.T, typed string) Model {
	t.Helper()
	m, _, target := editing(t)
	target.all = append(target.all,
		deck.Card{Qty: 1, Card: mtg.Card{Name: "Cultivate"}, Tags: []string{"ramp"}},
		deck.Card{Qty: 1, Card: mtg.Card{Name: "Swords"}, Tags: []string{"removal", "instant"}},
		deck.Card{Qty: 1, Card: mtg.Card{Name: "Wrath"}, Tags: []string{"removal", "wipe"}},
		deck.Card{Qty: 1, Card: mtg.Card{Name: "Ravos"}, Tags: []string{"recursion"}},
		deck.Card{Qty: 1, Card: mtg.Card{Name: "Shatter"}, Tags: []string{"remove-artifact"}},
	)
	target.refresh()
	m = focusOn(m, 1)
	m = drive(m, "t")
	for _, r := range typed {
		m = drive(m, string(r))
	}
	return m
}

func tab(m Model, back bool) Model {
	msg := tea.KeyMsg{Type: tea.KeyTab}
	if back {
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func asked(m Model) string { return m.ws.current().askInput.Value() }

func TestTabFinishesTheOnlyTagThatFits(t *testing.T) {
	m := tab(tagging(t, "wi"), false)
	if got := asked(m); got != "wipe " {
		t.Errorf("got %q, want %q", got, "wipe ")
	}
}

func TestTabGrowsToWhatTheTagsShareThenWalksThem(t *testing.T) {
	m := tab(tagging(t, "rem"), false)
	if got := asked(m); got != "remov" {
		t.Fatalf("first tab gave %q, want the shared %q", got, "remov")
	}
	if !strings.Contains(m.notice, "removal") || !strings.Contains(m.notice, "remove-artifact") {
		t.Errorf("the tags that fit aren't listed: %q", m.notice)
	}
	if strings.Contains(m.notice, "recursion") {
		t.Errorf("recursion is offered for rem: %q", m.notice)
	}

	for _, step := range []struct {
		back bool
		want string
	}{
		{false, "removal"},
		{false, "remove-artifact"},
		{false, "removal"}, // wraps
		{true, "remove-artifact"},
	} {
		m = tab(m, step.back)
		if got := asked(m); got != step.want {
			t.Errorf("got %q, want %q", got, step.want)
		}
	}
}

func TestWithNothingSharedTabWalksAtOnce(t *testing.T) {
	m := tab(tagging(t, "r"), false)
	if got := asked(m); got != "ramp" {
		t.Errorf("got %q, want the first tag that fits", got)
	}
	if !strings.Contains(m.notice, "[ramp]") {
		t.Errorf("the one showing isn't marked: %q", m.notice)
	}
}

func TestTabCompletesTheLastWordAndKeepsTheMinus(t *testing.T) {
	m := tab(tagging(t, "ramp -ins"), false)
	if got := asked(m); got != "ramp -instant " {
		t.Errorf("got %q", got)
	}
}

func TestTypingEndsTheWalk(t *testing.T) {
	m := tab(tagging(t, "rem"), false) // "remov", listing two
	m = drive(m, "a")                  // "remova": no longer what tab left
	m = tab(m, false)
	if got := asked(m); got != "removal " {
		t.Errorf("got %q, want the one tag that fits remova", got)
	}
}

func TestTabWithNothingThatFitsSaysSo(t *testing.T) {
	m := tab(tagging(t, "zz"), false)
	if got := asked(m); got != "zz" {
		t.Errorf("the input changed to %q", got)
	}
	if !strings.Contains(m.notice, "no tag starts with") {
		t.Errorf("notice is %q", m.notice)
	}
}

func TestACompletedTagIsApplied(t *testing.T) {
	m := tab(tagging(t, "wi"), false)
	m = drive(m, "enter")
	target := m.ws.panels[1].cardsView()
	i := target.indexOfCard("Sol Ring")
	if i < 0 || len(target.all[i].Tags) != 1 || target.all[i].Tags[0] != "wipe" {
		t.Errorf("tags are %v", target.all[i].Tags)
	}
}

func TestCommonPrefixIsRuneSafe(t *testing.T) {
	if got := commonPrefix([]string{"éa", "èb"}); got != "" {
		t.Errorf("got %q", got)
	}
	if got := commonPrefix([]string{"removal", "remove", "rem"}); got != "rem" {
		t.Errorf("got %q", got)
	}
}
