package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/deck"
	"scry/internal/paths"
)

// What `scry` on its own comes back to.
//
// The workspace is the thing worth restoring: which panels were open, what
// each was showing, and which deck you were building. Not the cursor, and
// not what was highlighted — coming back to a card you don't remember
// selecting is disorienting in a way that coming back to your panels isn't.
//
// Only things that will still be there tomorrow are recorded. A deck browsed
// off Moxfield isn't yours and might be gone; a search can always be run
// again, which is why it is the query that's kept and not its results.

const sessionFile = "session.json"

type panelSession struct {
	Kind string `json:"kind"`
	// Query is what a find or rules panel was showing.
	Query string `json:"query,omitempty"`
	// Deck is the slug of a local deck the panel had open.
	Deck string `json:"deck,omitempty"`
}

type session struct {
	Panels  []panelSession `json:"panels,omitempty"`
	Focused int            `json:"focused,omitempty"`
	// Editing is the index of the panel that was the editing deck. Chosen
	// with e, so it is worth coming back to.
	Editing int `json:"editing,omitempty"`
}

func sessionPath() string { return filepath.Join(paths.State(), sessionFile) }

func loadSession() session {
	var s session
	body, err := os.ReadFile(sessionPath())
	if err != nil {
		return session{Editing: -1}
	}
	if json.Unmarshal(body, &s) != nil {
		return session{Editing: -1}
	}
	return s
}

// save records the workspace. Failures are silent: losing the session costs
// a few keystrokes tomorrow, and a dialogue about it on the way out would
// cost more.
func (m Model) saveSession() {
	s := session{Focused: m.ws.focused, Editing: -1}

	for i, p := range m.ws.panels {
		ps := panelSession{Kind: p.kind.String()}
		switch v := p.top().(type) {
		case *cardList:
			// A local deck comes back; somebody else's doesn't, and a
			// search comes back as the query that produced it.
			switch {
			case v.deck != nil && v.deck.Local():
				ps.Kind = "deck"
				ps.Deck = v.deck.Slug
			case v.deck != nil:
				continue // borrowed, and might not be there tomorrow
			default:
				ps.Kind = "find"
				ps.Query = v.name
			}
		case *rulesView:
			ps.Kind = "rules"
			ps.Query = v.name
		case *deckList:
			ps.Kind = "decks"
		case nil:
			// A search still in flight has no view yet but does have a
			// query, and that is the part worth keeping. A panel with
			// nothing in it at all is a question you hadn't answered.
			if p.kind == KindFind && p.title != "" {
				ps.Query = p.title
				break
			}
			continue
		default:
			// A sub-view — versions, somebody's decks — comes back as the
			// panel it was reached from.
			ps.Kind = p.kind.String()
		}

		if m.ws.editing == i {
			s.Editing = len(s.Panels)
		}
		s.Panels = append(s.Panels, ps)
	}

	if s.Focused >= len(s.Panels) {
		s.Focused = maxInt(len(s.Panels)-1, 0)
	}

	body, err := json.Marshal(s)
	if err != nil {
		return
	}
	os.WriteFile(sessionPath(), body, 0644)
}

// restore reopens what was there. Each panel that needs fetching hands back
// a command; the workspace is on screen before any of them answer, so it
// comes up at once and fills in.
func (m *Model) restore() tea.Cmd {
	s := loadSession()
	if len(s.Panels) == 0 {
		return nil
	}

	var cmds []tea.Cmd
	for _, ps := range s.Panels {
		switch ps.Kind {
		case "find":
			p := m.ws.open(KindFind)
			p.history = m.history
			if ps.Query != "" {
				p.search.SetValue(ps.Query)
				cmds = append(cmds, m.search(p))
			}

		case "decks":
			l := newDeckList()
			m.ws.open(KindDecks).show(l)
			cmds = append(cmds, checkLegality(l.localSlugs()))

		case "rules":
			p := m.ws.open(KindRules)
			if q := ruleQuery(ps.Query); q != "" {
				cmds = append(cmds, m.searchRules(p, q))
			}

		case "deck":
			// A deck that has since been deleted simply doesn't come back.
			if ps.Deck == "" || !deck.Exists(ps.Deck) {
				continue
			}
			p := m.ws.open(KindDecks)
			p.searchOpen = false
			p.search.Blur()
			p.loading = true
			p.title = ps.Deck
			cmds = append(cmds, openLocalDeck(p.id, true, ps.Deck))
		}
	}

	if m.ws.count() == 0 {
		return nil
	}
	// Focus first: it re-checks the target, and the target is the thing we
	// are about to restore.
	m.ws.focus(minInt(s.Focused, m.ws.count()-1))
	if s.Editing >= 0 && s.Editing < m.ws.count() {
		m.ws.editing = s.Editing
	}
	return tea.Batch(cmds...)
}

// ruleQuery undoes the "rules: " a rules panel puts on its own title, so a
// restored search is the search rather than the label.
func ruleQuery(title string) string {
	const prefix = "rules: "
	if len(title) > len(prefix) && title[:len(prefix)] == prefix {
		return title[len(prefix):]
	}
	return ""
}
