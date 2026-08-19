package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/deck"
	"scry/internal/moxfield"
)

// What the decks panel actually does: opening things, and changing them.
//
// Everything that touches the network or the disk runs off the main thread
// and comes back as a message. The panel it came from is found by id, since
// the row can be reordered or closed while a deck is being resolved.

// ── Messages ────────────────────────────────────────────────────

type deckOpenedMsg struct {
	panel   int
	newPane bool
	info    deck.Info
	cards   []deck.Card
	err     error
}

type userDecksMsg struct {
	panel   int
	newPane bool
	user    string
	decks   []moxfield.UserDeck
	err     error
}

// reloadDecksMsg tells every decks panel to read the directory again, after
// something changed it.
type reloadDecksMsg struct{}

// noticeMsg is a one-line result — "copied", "deleted" — shown until the
// next keypress.
type noticeMsg struct {
	text string
	err  error
}

// ── Opening ─────────────────────────────────────────────────────

// openEntry acts on the highlighted row: a deck becomes a list of cards, a
// person becomes a list of their decks.
func (m *Model) openEntry(l *deckList, p *panel, newPane bool) tea.Cmd {
	e, ok := l.current()
	if !ok {
		return nil
	}

	target := p
	if newPane {
		// L opens to the right and leaves you where you were, so the list
		// you were browsing is still under the cursor.
		target = m.ws.open(p.kind)
		m.ws.focus(m.ws.indexOf(p))
	}
	target.loading = true
	target.err = nil

	switch e.kind {
	case entryLocal:
		target.title = e.name
		return openLocalDeck(target.id, newPane, e.slug)
	case entryRemote:
		target.title = e.name
		return openRemoteDeck(target.id, newPane, e.id)
	case entryUser:
		target.title = e.user
		return openUserDecks(target.id, newPane, e.user)
	}
	return nil
}

func openLocalDeck(panelID int, newPane bool, slug string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := deck.Open(slug)
		return deckOpenedMsg{panel: panelID, newPane: newPane, info: info, cards: cards, err: err}
	}
}

func openRemoteDeck(panelID int, newPane bool, id string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := moxfield.Load(id)
		return deckOpenedMsg{panel: panelID, newPane: newPane, info: info, cards: cards, err: err}
	}
}

func openUserDecks(panelID int, newPane bool, user string) tea.Cmd {
	return func() tea.Msg {
		name, decks, err := moxfield.UserDecks(user)
		if name == "" {
			name = user
		}
		return userDecksMsg{panel: panelID, newPane: newPane, user: name, decks: decks, err: err}
	}
}

// handleDeckOpened files a resolved deck.
func (m Model) handleDeckOpened(msg deckOpenedMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil // the panel closed while the cards were resolving
	}
	p.loading = false

	// A deck whose cards mostly resolved is still worth showing; the error
	// rides alongside as a notice rather than instead of the deck.
	if msg.err != nil && len(msg.cards) == 0 {
		p.err = msg.err
		return m, nil
	}
	if msg.err != nil {
		m.notice = msg.err.Error()
	}

	l := newCardList(msg.cards, sortArrival, "decklist")
	l.name = msg.info.Name
	l.deck = &msg.info
	l.recheck()

	// A deck opened in its own panel replaces what was there; one opened
	// from the decks list steps into it, so esc goes back to the list.
	if msg.newPane || p.top() == nil {
		p.show(l)
	} else {
		p.push(l)
		p.title = l.name
	}
	m.ws.deriveEditingIfUnpinned()
	return m, nil
}

func (m Model) handleUserDecks(msg userDecksMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	p.loading = false
	if msg.err != nil {
		p.err = msg.err
		return m, nil
	}

	v := newUserDeckList(msg.user, msg.decks)
	if msg.newPane || p.top() == nil {
		p.show(v)
	} else {
		p.push(v)
		p.title = v.title()
	}
	return m, nil
}

// ── Changing things ─────────────────────────────────────────────

// deleteEntry removes whatever the row stands for: a deck file, a remote
// link, a person. Only the first of those loses anything that can't be got
// back, which is why only that one asks first.
func (m *Model) deleteEntry(l *deckList, e deckEntry) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			if err := deck.DeleteCommitted(e.slug); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "deleted " + e.name}
		case entryRemote:
			b := deck.LoadBookmarks()
			b.RemoveRemote(e.id)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "stopped following " + e.name}
		case entryUser:
			b := deck.LoadBookmarks()
			b.RemoveUser(e.user)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "removed " + e.user}
		}
		return nil
	}
}

// copyEntry duplicates a local deck, or takes a local copy of a remote one.
// A remote whose copy already exists is synced instead — overwritten, and
// the change recorded in git, which is what makes overwriting safe.
func copyEntry(e deckEntry) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			d, err := deck.Read(e.slug)
			if err != nil {
				return noticeMsg{err: err}
			}
			slug, copied, err := deck.New(uniqueName(d.Name+" copy"), d.Format)
			if err != nil {
				return noticeMsg{err: err}
			}
			copied.Entries = d.Entries
			if _, _, err := deck.SaveVersioned(slug, copied); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "copied to " + slug}

		case entryRemote:
			d, err := moxfield.Import(e.id)
			if err != nil {
				return noticeMsg{err: err}
			}
			slug := deck.Slugify(d.Name)
			existed := deck.Exists(slug)
			subject, _, err := deck.SaveVersioned(slug, d)
			if err != nil {
				return noticeMsg{err: err}
			}
			if existed {
				if subject == "" {
					return noticeMsg{text: slug + " is already up to date"}
				}
				return noticeMsg{text: "synced " + slug + ": " + subject}
			}
			return noticeMsg{text: "copied to " + slug}
		}
		return noticeMsg{text: "nothing to copy"}
	}
}

// uniqueName finds a name whose slug isn't taken, so copying twice gives you
// two decks rather than an error.
func uniqueName(base string) string {
	if !deck.Exists(deck.Slugify(base)) {
		return base
	}
	for i := 2; i < 100; i++ {
		name := fmt.Sprintf("%s %d", base, i)
		if !deck.Exists(deck.Slugify(name)) {
			return name
		}
	}
	return base
}

// newDeck makes an empty one. A deck you've just created is empty by
// definition, and you fill it by adding cards to it.
func newDeckCmd(name string) tea.Cmd {
	return func() tea.Msg {
		slug, d, err := deck.New(name, deck.DefaultFormat)
		if err != nil {
			return noticeMsg{err: err}
		}
		if _, _, err := deck.SaveVersioned(slug, d); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "made " + slug}
	}
}

// renameDeck changes a local deck's title, or what a remote is called in
// your list. The slug is left alone: it names a file with a git history, and
// renaming that would lose the history rather than move it.
func renameCmd(e deckEntry, name string) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			d, err := deck.Read(e.slug)
			if err != nil {
				return noticeMsg{err: err}
			}
			d.Name = name
			if _, _, err := deck.SaveVersioned(e.slug, d); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "renamed to " + name}
		case entryRemote:
			b := deck.LoadBookmarks()
			b.RenameRemote(e.id, name)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "renamed to " + name}
		}
		return noticeMsg{text: "a person can't be renamed"}
	}
}

// follow records whatever was pasted into the decks panel's search bar: a
// Moxfield deck URL or id becomes a remote, anything else is taken for a
// username.
func follow(input string) tea.Cmd {
	return func() tea.Msg {
		b := deck.LoadBookmarks()

		if id, ok := moxfield.Ref(input); ok {
			name := id
			if d, err := moxfield.Fetch(id); err == nil && d.Name != "" {
				name = d.Name
			}
			b.AddRemote(deck.Remote{Name: name, ID: id, URL: input})
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "following " + name}
		}

		user, ok := moxfield.User(input)
		if !ok {
			return noticeMsg{err: fmt.Errorf("not a Moxfield deck or username: %q", input)}
		}
		b.AddUser(user)
		if err := deck.SaveBookmarks(b); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "following " + user}
	}
}

// reloadDecks tells every decks panel to read the directory again.
func reloadDecks() tea.Msg { return reloadDecksMsg{} }

// ── Legality ────────────────────────────────────────────────────

// legalityMsg carries the verdict on one deck back to the lists showing it.
type legalityMsg struct {
	slug     string
	legality deck.Legality
}

// checkLegality works out whether each local deck is legal, from the card
// cache alone. One command per deck rather than one for all of them, so the
// first answers appear while the rest are still being worked out.
func checkLegality(slugs []string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(slugs))
	for _, slug := range slugs {
		s := slug
		cmds = append(cmds, func() tea.Msg {
			return legalityMsg{slug: s, legality: deck.CheckCached(s)}
		})
	}
	return tea.Batch(cmds...)
}

func (m Model) handleLegality(msg legalityMsg) (tea.Model, tea.Cmd) {
	for _, p := range m.ws.panels {
		if l, ok := p.top().(*deckList); ok {
			l.setLegality(msg.slug, msg.legality)
		}
		if l := p.cardsView(); l != nil && l.deck != nil && l.deck.Slug == msg.slug {
			legality := msg.legality
			l.legality = &legality
		}
	}
	return m, nil
}
