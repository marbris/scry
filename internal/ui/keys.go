package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Keys, in two spaces.
//
// Bare keys act on the contents of the focused panel. The leader acts on the
// panels themselves — making them, closing them, moving them about. That
// split is what lets r mean rename in a list and <space>r open a rules
// panel without either of them being ambiguous, the same way vim has f,
// ctrl+f, gf and zf all meaning different things.
//
// The leader is space, with comma as an alias for the fingers that learned
// the old one.
//
// It follows from that choice that the leader does nothing inside a search
// bar, where space is a space — the same way vim's leader does nothing in
// insert mode. The bar is insert mode. Since a new panel opens with its bar
// focused, opening a second empty panel means finishing or abandoning the
// first, which is fair: an empty panel is a question you haven't answered.

const (
	leaderKey = " "
	leaderAlt = ","
)

// leaderCmd is one entry in the menu the leader raises.
type leaderCmd struct {
	key  string
	what string
	run  func(*Model)
}

// leaderMenu is the menu, in the order it's shown: making panels, then
// getting rid of them, then moving among them.
var leaderMenu = []leaderCmd{
	{"f", "find", func(m *Model) { m.ws.open(KindFind) }},
	{"d", "decks", nil}, // opens the list and checks it, so it needs a command
	{"r", "rules", nil}, // needs a command, so it is run below
	{"n", "new", func(m *Model) { m.ws.open(KindNew) }},
	{"s", "stats (everything)", func(m *Model) { m.toggleStats(true) }},
	{"c", "close", func(m *Model) { m.ws.close() }},
	{"o", "only", func(m *Model) { m.ws.only() }},
	{"h", "move left", func(m *Model) { m.ws.movePanel(-1) }},
	{"l", "move right", func(m *Model) { m.ws.movePanel(1) }},
	{"?", "keys", func(m *Model) { m.showKeys = !m.showKeys }},
}

// handleLeader runs the command a key names. A key that names nothing
// cancels, rather than doing something surprising with a near miss.
func (m *Model) handleLeader(key string) tea.Cmd {
	m.leader = false

	// <space>1…9 jumps straight to a panel, which beats counting presses of
	// l once four or five are open.
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		m.ws.focus(int(key[0] - '1'))
		return nil
	}

	// Two entries hand back a command rather than only changing the
	// workspace, so they can't sit in the table above.
	if key == "r" {
		return m.openRules()
	}
	if key == "d" {
		l := newDeckList()
		m.ws.open(KindDecks).show(l)
		return checkLegality(l.localSlugs())
	}

	for _, c := range leaderMenu {
		if c.key == key && c.run != nil {
			c.run(m)
			return nil
		}
	}
	return nil
}

// handleKey routes a keypress: the leader first, then whatever the focused
// panel's search bar wants, then the workspace's own keys.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.goPrefix {
		m.goPrefix = false
		return m.handleGoto(key)
	}

	// The unsaved-changes question takes every key until it's answered.
	if m.quitting {
		m.quitting = false
		switch key {
		case "y":
			return m, m.quit()
		case "w":
			cmd := m.saveEverything()
			return m, cmd
		}
		return m, nil
	}

	if m.leader {
		// Sequenced rather than returned inline. This is the shape to watch
		// for throughout: the call mutates m, and `return m, m.f()` leaves
		// the compiler free to copy m for the return value before f runs.
		// Every call of that shape in this package is split in two.
		cmd := m.handleLeader(key)
		return m, cmd
	}

	// The key reference is a lid: anything closes it.
	if m.showKeys {
		m.showKeys = false
		return m, nil
	}

	// With nothing open, the leader is the only way forward, so the splash
	// takes a couple of shortcuts to it.
	if m.ws.empty() {
		switch key {
		case leaderKey, leaderAlt:
			m.leader = true
		case "q", "esc", "ctrl+c":
			return m.tryQuit()
		case "?":
			m.showKeys = true
		}
		return m, nil
	}

	p := m.ws.current()

	// A prompt is a text field, and takes the keys while it's open.
	if p.asking != askNone {
		return m.handleAskKey(msg)
	}

	// The filter prompt is a text field too, and takes precedence over the
	// list's keys while it's open.
	if p.filtering {
		return m.handleFilterKey(msg)
	}

	// A focused search bar is a text field first: it gets the printable
	// keys, and the leader with them, or you could never type a space.
	if p.searchOpen && p.search.Focused() {
		return m.handleSearchKey(msg)
	}

	// y in the printed-text panel is the go-ahead to download the sets that
	// aren't cached. It has to be claimed before the view sees it, because
	// a card list takes y for yank.
	if key == "y" && m.info.mode == infoVersions {
		if c := m.focusedCard(); c != nil {
			if h, ok := m.histories[c.OracleID]; ok && h.state == histWaiting {
				cmd := m.fetchAllSets(h)
				return m, cmd
			}
		}
	}

	// g is a prefix everywhere and never a key on its own, so it has to be
	// claimed before any view sees it — a list would otherwise take it for
	// "go to the top" and gd and gv could never be typed. gg still means
	// the top: handleGoto passes the second g back down.
	if key == "g" {
		m.goPrefix = true
		return m, nil
	}

	// The view has first refusal on anything that isn't the workspace's.
	if v := p.top(); v != nil {
		if handled, cmd := v.key(key, &m, p); handled {
			// The cursor may have moved, so start the clock on whatever is
			// under it now.
			hover := m.hover()
			return m, tea.Batch(cmd, hover)
		}
	}

	switch key {
	case leaderKey, leaderAlt:
		m.leader = true

	case "h", "left":
		m.ws.step(-1)
		cmd := m.hover()
		return m, cmd
	case "l", "right":
		m.ws.step(1)
		cmd := m.hover()
		return m, cmd

	case "i":
		// The bar keeps the query that produced what's on screen, so i is
		// "edit this search" rather than "start again" — with the cursor
		// where you'd carry on typing.
		p.searchOpen = true
		p.search.Focus()
		p.search.CursorEnd()

	case "e":
		m.ws.pin()

	case "K", "shift+up":
		if m.info.mode == infoStats {
			m.moveStat(-1)
		} else {
			m.info.move(-1)
		}
	case "J", "shift+down":
		if m.info.mode == infoStats {
			m.moveStat(1)
		} else {
			m.info.move(1)
		}
	case "ctrl+k":
		m.info.scroll(-1)
	case "ctrl+j":
		m.info.scroll(1)

	case "s":
		m.toggleStats(false)

	case "?":
		m.showKeys = true

	case "esc":
		// The cascade, outward one step at a time, as the design has it:
		// clear a narrowing, step back out of a sub-view, close the panel.
		// Closing the last one lands on the splash rather than quitting —
		// esc *from* the splash is what leaves.
		switch {
		// A statistics narrowing is the outermost thing esc undoes: it was
		// imposed from the panel beside the list, so it comes off first.
		case m.info.mode == infoStats && m.stats.row >= 0:
			m.stats.row = -1
			m.applyStatFilter()
		case m.info.mode == infoStats:
			m.toggleStats(m.stats.global)
		case p.top() != nil && p.top().clear():
		case p.pop():
		default:
			m.ws.close()
		}

	case "q", "ctrl+c":
		return m.tryQuit()
	}
	return m, nil
}

// handleSearchKey is the search bar's own keymap. Everything it doesn't
// claim goes into the input as text.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.ws.current()

	switch msg.String() {
	case "tab":
		p.setKind(p.kind.next(1))
		return m, nil
	case "shift+tab":
		p.setKind(p.kind.next(-1))
		return m, nil

	case "esc":
		// The same cascade as everywhere else: clear what's clearable, then
		// leave, then close. Typing half a query and pressing esc should
		// lose the half-query, not the panel.
		if p.search.Value() != "" {
			p.search.SetValue("")
			return m, nil
		}
		if p.empty() {
			// Closing the last panel lands on the splash rather than
			// quitting. Leaving the program is what esc does *from* the
			// splash, so there is always one press between you and the exit.
			m.ws.close()
			return m, nil
		}
		p.searchOpen = false
		p.search.Blur()
		return m, nil

	case "enter":
		if p.kind == KindFind {
			cmd := m.search(p)
			return m, cmd
		}
		if p.kind == KindRules {
			q := p.search.Value()
			if q == "" {
				return m, nil
			}
			cmd := m.searchRules(p, q)
			return m, cmd
		}
		if p.kind == KindDecks {
			// The decks panel's bar follows a Moxfield deck or a person
			// rather than searching: what you can already see is filtered
			// with /, and what you can't is somewhere else entirely.
			q := p.search.Value()
			p.search.SetValue("")
			p.searchOpen = false
			p.search.Blur()
			if p.top() == nil {
				p.show(newDeckList())
			}
			return m, follow(q)
		}
		// The other kinds get their own bar in the phases that build them.
		if q := p.search.Value(); q != "" {
			p.title = p.kind.String() + ": " + q
			p.searchOpen = false
			p.search.Blur()
		}
		return m, nil

	case "up":
		p.recall(-1)
		return m, nil
	case "down":
		p.recall(1)
		return m, nil

	case "ctrl+o":
		cmd := m.cycleQuerySort(p, 1)
		return m, cmd

	case "ctrl+c":
		return m, m.quit()
	}

	// Anything else is typing, which ends a walk through the history: what
	// is in the bar is yours again rather than something recalled.
	before := p.search.Value()
	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
	if p.search.Value() != before {
		p.leaveHistory()
	}
	return m, cmd
}

// pageStep is half a screen, which is what ctrl+d and ctrl+u move by in vim.
func (m Model) pageStep() int {
	return maxInt((m.height-hintHeight-4)/2, 1)
}

// handleFilterKey is the / prompt. It narrows as you type, so you can see
// what you're doing rather than committing blind.
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.ws.current()

	switch msg.String() {
	case "enter":
		p.filtering = false
		p.filterInput.Blur()
		return m, nil

	case "esc":
		// Abandoning the prompt puts back whatever was filtered before it
		// opened, rather than leaving a half-typed narrowing in place.
		p.filtering = false
		p.filterInput.Blur()
		p.setFilter("")
		return m, nil

	case "ctrl+c":
		return m, m.quit()
	}

	var cmd tea.Cmd
	p.filterInput, cmd = p.filterInput.Update(msg)
	p.setFilter(p.filterInput.Value())
	return m, cmd
}

// handleGoto runs the g-prefixed keys.
func (m Model) handleGoto(key string) (tea.Model, tea.Cmd) {
	p := m.ws.current()
	if p == nil {
		return m, nil
	}

	switch key {
	case "g":
		if v := p.top(); v != nil {
			v.key("g", &m, p) // the views' own "to the top"
		}

	case "d":
		// Straight to the deck being edited, from wherever you are.
		if m.ws.editing >= 0 {
			m.ws.focus(m.ws.editing)
		}

	case "v":
		cmd := m.versions(p)
		return m, cmd
	}
	return m, nil
}

// versions opens the history of whatever is under the cursor: a deck's
// commits here, a card's printed wordings when the information panel learns
// to show them.
func (m *Model) versions(p *panel) tea.Cmd {
	switch v := p.top().(type) {
	case *deckList:
		e, ok := v.current()
		if !ok || e.kind != entryLocal {
			return nil
		}
		p.loading = true
		return loadVersions(p.id, e.slug, e.name)

	case *cardList:
		// A deck's versions are its commits; a card's are the wordings it
		// has been printed with. Same question, two kinds of thing.
		if v.deck != nil && v.deck.Local() {
			p.loading = true
			return loadVersions(p.id, v.deck.Slug, v.deck.Name)
		}
		if c, ok := v.current(); ok {
			return m.openHistory(c.Card)
		}
	}
	return nil
}

// tryQuit leaves, unless something would be lost by leaving.
//
// Saving is explicit, so this is the price of that: a deck edited and not
// written is only safe if the way out asks about it.
func (m Model) tryQuit() (tea.Model, tea.Cmd) {
	if len(m.dirtyDecks()) == 0 {
		return m, m.quit()
	}
	m.quitting = true
	return m, nil
}

// saveEverything writes every deck with unsaved edits, for the w in the
// quit question.
func (m Model) saveEverything() tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range m.ws.panels {
		l := p.cardsView()
		if l != nil && l.dirty && l.deck != nil && l.deck.Local() {
			cmds = append(cmds, saveDeck(p.id, *l.deck, l.all))
		}
	}
	return tea.Batch(cmds...)
}

// toggleStats turns the statistics on or off. Turning them off puts back
// whatever they were narrowing, because a narrowing you can no longer see
// the reason for is a bug you'd spend a while finding.
func (m *Model) toggleStats(global bool) {
	if m.info.mode == infoStats && m.stats.global == global {
		m.info.mode = infoCard
		m.stats.row = -1
		m.clearStatFilter()
		return
	}
	m.info.mode = infoStats
	m.stats.global = global
	m.stats.row = -1
	m.info.offset = 0
	m.clearStatFilter()
	m.buildStats()
}
