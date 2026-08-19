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
	{"d", "decks", func(m *Model) { m.ws.open(KindDecks) }},
	{"r", "rules", func(m *Model) { m.ws.open(KindRules) }},
	{"n", "new", func(m *Model) { m.ws.open(KindNew) }},
	{"s", "stats", func(m *Model) { m.info.mode = infoStats }},
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

	for _, c := range leaderMenu {
		if c.key == key {
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

	if m.leader {
		// Sequenced rather than returned inline: the command mutates m, and
		// `return m, m.handleLeader(key)` leaves the compiler free to copy
		// m before the call runs.
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
			return m, tea.Quit
		case "?":
			m.showKeys = true
		}
		return m, nil
	}

	p := m.ws.current()

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

	switch key {
	case leaderKey, leaderAlt:
		m.leader = true

	case "h", "left":
		m.ws.step(-1)
	case "l", "right":
		m.ws.step(1)

	case "i":
		p.searchOpen = true
		p.search.Focus()

	// ── The list ────────────────────────────────────────────────

	case "j", "down":
		p.list().move(1)
	case "k", "up":
		p.list().move(-1)
	case "g":
		p.list().top()
	case "G":
		p.list().bottom()
	case "ctrl+d":
		p.list().move(m.pageStep())
	case "ctrl+u":
		p.list().move(-m.pageStep())

	case "o":
		p.list().cycleSort(1)
	case "O":
		p.list().cycleSort(-1)

	case "v":
		p.list().toggleMark()
	case "V":
		p.list().markAll()

	case "/":
		if p.cards != nil {
			p.filtering = true
			p.filterInput.SetValue(p.cards.filter)
			p.filterInput.CursorEnd()
			p.filterInput.Focus()
		}

	case "e":
		m.ws.pin()

	case "K", "shift+up":
		m.info.move(-1)
	case "J", "shift+down":
		m.info.move(1)
	case "ctrl+k":
		m.info.scroll(-1)
	case "ctrl+j":
		m.info.scroll(1)

	case "s":
		m.info.toggle(infoStats)

	case "?":
		m.showKeys = true

	case "esc":
		// The cascade: clear what's clearable, and only then close.
		switch {
		case p.cards != nil && p.cards.markCount() > 0:
			p.cards.clearMarks()
		case p.cards != nil && p.cards.filter != "":
			p.cards.setFilter("")
		case p.empty():
			// Closing the last panel lands on the splash rather than
			// quitting: esc *from* the splash is what leaves.
			m.ws.close()
		default:
			p.cards = nil
			p.title = ""
			p.searchOpen = true
			p.search.Focus()
		}

	case "q":
		return m, tea.Quit

	case "ctrl+c":
		return m, tea.Quit
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
		// Running a query is phase 5; for now the panel simply takes the
		// name of what was asked for, which is enough to see the header
		// replace the bar.
		if q := p.search.Value(); q != "" {
			p.title = p.kind.String() + ": " + q
			p.searchOpen = false
			p.search.Blur()
		}
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}

	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
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
		p.list().setFilter("")
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}

	var cmd tea.Cmd
	p.filterInput, cmd = p.filterInput.Update(msg)
	p.list().setFilter(p.filterInput.Value())
	return m, cmd
}
