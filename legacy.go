package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Before decks were files, saving one recorded a name and a Moxfield address
// in decks.json and re-fetched the deck every time it was opened. Deck files
// replace that outright, so none of it is wired up any more — a name saved
// that way no longer resolves and nothing writes to the file.
//
// The file itself is left alone. It's the only record of which decks someone
// had saved, so `scry deck list` points at it until they've imported them
// and deleted it themselves. Deleting a person's data because we stopped
// reading it isn't ours to do.

type legacyBookmark struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func legacyBookmarksPath() string {
	return filepath.Join(dataDir(), "decks.json")
}

// legacyBookmarks reads the old file, if it's still there. Any problem with
// it means there's nothing useful to say, so it comes back empty.
func legacyBookmarks() map[string]legacyBookmark {
	body, err := os.ReadFile(legacyBookmarksPath())
	if err != nil {
		return nil
	}
	out := map[string]legacyBookmark{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}

// legacyNotice tells you what's still in the old file and how to bring it
// across. Empty once nothing is left to bring.
func legacyNotice() string {
	saved := legacyBookmarks()

	// A bookmark whose deck has since been imported is already across.
	var pending []string
	for alias := range saved {
		if !deckExists(alias) {
			pending = append(pending, alias)
		}
	}
	if len(pending) == 0 {
		return ""
	}
	sort.Strings(pending)

	dim := lipgloss.NewStyle().Foreground(gruvGray)
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n",
		dim.Render(fmt.Sprintf("%d deck(s) saved by an older version, in %s:",
			len(pending), legacyBookmarksPath())))
	for _, alias := range pending {
		fmt.Fprintf(&b, "  %s  %s\n",
			lipgloss.NewStyle().Foreground(gruvYellow).Render(alias),
			dim.Render(saved[alias].URL))
	}
	b.WriteString(dim.Render("Import them with `scry deck import <url>`, then delete that file.\n"))
	return b.String()
}
