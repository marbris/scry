package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Saved decks live in one small JSON file beside the rules cache, keyed by
// the short name you'd type: `scry deck ghen`. Only the reference is kept,
// never the card list — the deck is re-fetched each time, so a deck that's
// been edited since you saved it comes back current.

type savedDeck struct {
	Name  string `json:"name"`  // the deck's own title on Moxfield
	ID    string `json:"id"`    // the id in its public URL
	URL   string `json:"url"`   // the public URL, for reference
	Saved string `json:"saved"` // when it was saved, RFC 3339
}

func decksFilePath() string {
	return filepath.Join(dataDir(), "decks.json")
}

// loadSavedDecks returns the saved decks by alias. A missing file is not an
// error — it just means nothing's been saved yet.
func loadSavedDecks() (map[string]savedDeck, error) {
	body, err := os.ReadFile(decksFilePath())
	if os.IsNotExist(err) {
		return map[string]savedDeck{}, nil
	}
	if err != nil {
		return nil, err
	}

	decks := map[string]savedDeck{}
	if err := json.Unmarshal(body, &decks); err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", decksFilePath(), err)
	}
	return decks, nil
}

func writeSavedDecks(decks map[string]savedDeck) error {
	body, err := json.MarshalIndent(decks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(decksFilePath(), append(body, '\n'), 0644)
}

// saveDeck records a deck under an alias, replacing any deck already saved
// under it.
func saveDeck(alias string, d savedDeck) error {
	decks, err := loadSavedDecks()
	if err != nil {
		return err
	}
	d.Saved = time.Now().Format(time.RFC3339)
	decks[alias] = d
	return writeSavedDecks(decks)
}

func forgetDeck(alias string) (bool, error) {
	decks, err := loadSavedDecks()
	if err != nil {
		return false, err
	}
	if _, ok := decks[alias]; !ok {
		return false, nil
	}
	delete(decks, alias)
	return true, writeSavedDecks(decks)
}

// lookupSavedDeck resolves an alias to its deck id.
func lookupSavedDeck(alias string) (savedDeck, bool) {
	decks, err := loadSavedDecks()
	if err != nil {
		return savedDeck{}, false
	}
	d, ok := decks[alias]
	return d, ok
}

// slugify turns a deck's title into an alias worth typing: "Winota:
// Snowball Stax" becomes "winota-snowball-stax".
func slugify(name string) string {
	var b strings.Builder
	lastDash := true // no leading dash
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// aliasesSorted lists the saved aliases in a stable order for display.
func aliasesSorted(decks map[string]savedDeck) []string {
	out := make([]string, 0, len(decks))
	for alias := range decks {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}
