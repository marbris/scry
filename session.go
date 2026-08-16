package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// What `scry` on its own comes back to. Running it with no arguments used to
// mean an empty screen, which is rarely what you want: you were in the
// middle of something last time, and the two things that made it yours were
// the deck you had open and the search you had run.
//
// Only those two. Not which panel was up, or where the cursor was — coming
// back to the statistics panel over a card you don't remember selecting is
// disorienting in a way that coming back to your deck isn't.

const sessionFile = "session.json"

type session struct {
	Query string `json:"query,omitempty"`
	Deck  string `json:"deck,omitempty"`
}

func sessionPath() string {
	return filepath.Join(dataDir(), sessionFile)
}

// loadSession reads the last one. Anything wrong with the file means a fresh
// start, which is what would have happened anyway.
func loadSession() session {
	body, err := os.ReadFile(sessionPath())
	if err != nil {
		return session{}
	}
	var s session
	if err := json.Unmarshal(body, &s); err != nil {
		return session{}
	}
	return s
}

func saveSession(s session) error {
	body, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), body, 0644)
}

// currentSession is what the app is showing, as much of it as is worth
// coming back to.
func (m model) currentSession() session {
	s := session{Query: m.lastQuery}
	if m.deck != nil && m.deck.local() {
		// A deck browsed off Moxfield isn't yours and might not be there
		// tomorrow; only one of your own is worth reopening.
		s.Deck = m.deck.slug
	}
	return s
}

// restore fills in what to open on startup from the last session. A query or
// a deck named on the command line is what you just asked for, and wins.
func (m model) restore() model {
	if m.initialQuery != "" || m.initialDeck != "" || m.initialDeckSlug != "" {
		return m
	}

	s := loadSession()
	if s.Deck != "" && deckExists(s.Deck) {
		m.initialDeckSlug = s.Deck
	}
	if s.Query != "" {
		m.initialQuery = s.Query
		m.searchInput.SetValue(s.Query)
		m.searchInput.CursorEnd()
		m.searching = true
	}
	return m
}
