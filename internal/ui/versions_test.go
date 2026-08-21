package ui

import (
	"testing"

	"scry/internal/deck"
)

func TestVersionListWiresRevertAndCopy(t *testing.T) {
	// r reverts the deck to the version under the cursor; c makes a new deck
	// out of it. Both hand back a command to do the git work off the main
	// thread — this checks the wiring without running it.
	commits := []deck.Commit{{Hash: "abc1234def", Short: "abc1234", Subject: "x", When: "now"}}
	l := &versionList{
		slug: "deck", name: "Deck",
		all: commits, commits: commits,
		diffs: map[string]string{},
	}

	for _, k := range []string{"r", "c"} {
		handled, cmd := l.key(k, nil, &panel{})
		if !handled {
			t.Errorf("%q was not handled by the version list", k)
		}
		if cmd == nil {
			t.Errorf("%q did not produce a command", k)
		}
	}
}

func TestShortHash(t *testing.T) {
	if got := shortHash("abc1234def5678"); got != "abc1234" {
		t.Errorf("shortHash abbreviated to %q", got)
	}
	if got := shortHash("abc"); got != "abc" {
		t.Errorf("shortHash mangled a short hash: %q", got)
	}
}
