package moxfield

import (
	"strings"
	"testing"
	"time"
)

func TestMoxfieldUserName(t *testing.T) {
	// A pasted profile URL should work as well as typing the name.
	for in, want := range map[string]string{
		"https://moxfield.com/users/MarBri":       "MarBri",
		"https://www.moxfield.com/users/MarBri/":  "MarBri",
		"moxfield.com/users/MarBri?tab=decks":     "MarBri",
		"https://moxfield.com/users/MarBri#decks": "MarBri",
		"HTTPS://MOXFIELD.COM/users/MarBri":       "MarBri",
	} {
		got, ok := UserName(in)
		if !ok || got != want {
			t.Errorf("UserName(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}

	// A bare name isn't a URL and is passed through as typed.
	for _, in := range []string{"MarBri", "", "https://moxfield.com/decks/abc"} {
		if got, ok := UserName(in); ok {
			t.Errorf("UserName(%q) = %q, want no match", in, got)
		}
	}
}

func TestDeckAge(t *testing.T) {
	// Nothing to go on reads as nothing, rather than as "today".
	for _, in := range []string{"", "not a date"} {
		if got := (UserDeck{Updated: in}).Age(); got != "" {
			t.Errorf("age(%q) = %q, want empty", in, got)
		}
	}
	if got := (UserDeck{Updated: "1970-01-01T00:00:00Z"}).Age(); !strings.HasSuffix(got, "years ago") {
		t.Errorf("a very old deck reads as %q", got)
	}

	// The plural is right at one, which is where a naive version says
	// "1 years ago".
	// The plural has to be right at one, which is where a naive version
	// says "1 years ago". Dated from now so the test doesn't rot.
	oneYear := time.Now().AddDate(-1, 0, -2).UTC().Format(time.RFC3339)
	if got := (UserDeck{Updated: oneYear}).Age(); got != "1 year ago" {
		t.Errorf("a year old reads as %q", got)
	}
	oneMonth := time.Now().AddDate(0, 0, -32).UTC().Format(time.RFC3339)
	if got := (UserDeck{Updated: oneMonth}).Age(); got != "1 month ago" {
		t.Errorf("a month old reads as %q", got)
	}
}

func TestSearchAsksForDecksThatArentLegal(t *testing.T) {
	// Moxfield's search returns only format-legal decks unless told
	// otherwise, and for anyone who builds in the open that's a small
	// fraction: an account with 42 public decks answered with 11, and the
	// 31 it left out were the ones mid-build — 157 cards, or 3, or none
	// yet. Those are the decks you'd open the list to work on.
	if !strings.Contains(moxSearchURL, "showIllegal=true") {
		t.Error("the deck search doesn't ask for decks that aren't legal yet")
	}
	// And it asks for them a hundred at a time rather than a screenful,
	// since it pages through the rest.
	if moxUserPageSize < 100 {
		t.Errorf("page size is %d", moxUserPageSize)
	}
	if moxUserMaxPages < 2 {
		t.Errorf("only %d page(s) are ever fetched", moxUserMaxPages)
	}
}

func TestUserDecksAreOrderedLegalThenNewest(t *testing.T) {
	// Most of a builder's decks are half-built. Burying the finished ones
	// under thirty works-in-progress makes the list harder to use than it
	// needs to be.
	decks := []UserDeck{
		{Name: "old wip", Updated: "2023-01-01T00:00:00Z"},
		{Name: "new done", Updated: "2025-12-01T00:00:00Z", Legal: true},
		{Name: "new wip", Updated: "2025-12-27T00:00:00Z"},
		{Name: "old done", Updated: "2024-01-01T00:00:00Z", Legal: true},
		{Name: "undated wip"},
	}
	sortUserDecks(decks)

	var got []string
	for _, d := range decks {
		got = append(got, d.Name)
	}
	want := "new done|old done|new wip|old wip|undated wip"
	if strings.Join(got, "|") != want {
		t.Errorf("\n got %s\nwant %s", strings.Join(got, "|"), want)
	}
}
