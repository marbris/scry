package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scry/internal/deck"
)

// gitRepo is a copy of the helper in package deck: the decks directory
// pointed somewhere temporary, with the machine's git identity kept out of
// it. Test helpers don't cross package boundaries without being exported
// into the shipped API, which isn't a trade worth making for ten lines.
func gitRepo(t *testing.T) string {
	t.Helper()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	t.Setenv("SCRY_DECKS_DIR", dir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "nonexistent-gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(dir, "nonexistent-gitconfig"))
	return dir
}

// seedCache writes a card cache under a temporary HOME so a test can resolve
// without touching the network. Also a copy of package deck's, for the same
// reason.
func seedCache(t *testing.T, cards map[string]ScryfallCard) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	body, err := json.Marshal(cards)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deck.CachePath(), body, 0644); err != nil {
		t.Fatal(err)
	}
}

// deckOf parses a deck file from a literal, for tests that care what a deck
// contains rather than how it was written.
func deckOf(t *testing.T, text string) *deckFile {
	t.Helper()
	d, err := parseDeckFile(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// gitBaseDeck is the same fixture package deck's git tests use — a small
// commander deck with tags, quantities and a section split.
const gitBaseDeck = `name: Ghen
format: commander

[commander]
1 Ghen, Arcanum Weaver [wincon]

[mainboard]
1 Sol Ring [ramp]
1 Arcane Signet [ramp]
7 Plains
`
