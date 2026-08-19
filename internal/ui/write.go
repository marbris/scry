package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/deck"
)

// Writing a list to disk.
//
// w means write *this list*, and what that produces depends on what the list
// is — the same way vim's :w saves the file and :w <name> makes a new one:
//
//	a local deck             saved where it came from, and committed
//	a search or a remote     a name is asked for, and it becomes yours
//
// W does the same but leaves the list where it was and opens the new deck in
// a panel of its own. That is the general rule: a capital puts the result in
// a new panel.

type deckSavedMsg struct {
	panel   int
	slug    string
	subject string
	warning string
	err     error
}

// write is w and W.
func (m *Model) write(l *cardList, p *panel, newPane bool) tea.Cmd {
	if l.deck != nil && l.deck.Local() {
		if !l.dirty {
			m.notice = l.deck.Name + " is already up to date"
			return nil
		}
		return saveDeck(p.id, *l.deck, l.all)
	}

	// Not yours yet, so it needs a name before it can be.
	name := l.name
	if l.deck != nil && l.deck.Name != "" {
		name = l.deck.Name
	}
	p.ask(askWrite, "save as", name)
	p.writeToNewPane = newPane
	return nil
}

// saveDeck writes the deck and records the change, off the main thread.
func saveDeck(panelID int, info deck.Info, cards []deck.Card) tea.Cmd {
	return func() tea.Msg {
		file := deck.FileFrom(info, cards)
		subject, warning, err := deck.SaveVersioned(info.Slug, file)
		return deckSavedMsg{panel: panelID, slug: info.Slug, subject: subject, warning: warning, err: err}
	}
}

// saveAsNew turns a search result or somebody else's deck into one of yours.
func saveAsNew(panelID int, newPane bool, name string, cards []deck.Card) tea.Cmd {
	return func() tea.Msg {
		slug, file, err := deck.New(uniqueName(name), deck.DefaultFormat)
		if err != nil {
			return deckSavedMsg{panel: panelID, err: err}
		}
		for _, c := range cards {
			section := "mainboard"
			if c.Commander {
				section = "commander"
			}
			file.Entries = append(file.Entries, deck.Entry{
				Qty: maxInt(c.Qty, 1), Name: c.Card.Name, Tags: c.Tags, Section: section,
			})
		}
		subject, warning, err := deck.SaveVersioned(slug, file)
		if err != nil {
			return deckSavedMsg{panel: panelID, err: err}
		}
		// Opened straight away, because the point of saving a search is to
		// start working on it.
		return deckWrittenMsg{
			panel: panelID, newPane: newPane, slug: slug,
			subject: subject, warning: warning,
		}
	}
}

// deckWrittenMsg is a new deck that now needs opening.
type deckWrittenMsg struct {
	panel   int
	newPane bool
	slug    string
	subject string
	warning string
}

func (m Model) handleDeckSaved(msg deckSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = "error: " + msg.err.Error()
		return m, nil
	}

	if p := m.ws.byID(msg.panel); p != nil {
		if l := p.cardsView(); l != nil {
			l.dirty = false
		}
	}

	switch {
	case msg.warning != "":
		m.notice = "saved — " + msg.warning
	case msg.subject == "":
		m.notice = "nothing to save"
	default:
		m.notice = "saved: " + msg.subject
	}
	return m, reloadDecks
}

func (m Model) handleDeckWritten(msg deckWrittenMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}

	m.notice = "saved as " + msg.slug
	if msg.warning != "" {
		m.notice += " — " + msg.warning
	}

	target := p
	if msg.newPane {
		target = m.ws.open(p.kind)
		m.ws.focus(m.ws.indexOf(p))
	}
	target.loading = true
	target.title = msg.slug
	return m, tea.Batch(openLocalDeck(target.id, msg.newPane, msg.slug), reloadDecks)
}

// dirtyDecks is every deck with edits not yet written, which is what quitting
// has to ask about.
func (m Model) dirtyDecks() []string {
	var out []string
	for _, p := range m.ws.panels {
		l := p.cardsView()
		if l != nil && l.dirty && l.deck != nil {
			out = append(out, l.deck.Name)
		}
	}
	return out
}
