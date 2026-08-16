package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The key map, in one place. The leader menu, the `?` reference and the
// hint lines are all rendered from these tables, so they can't drift apart
// — which they had, over four phases of adding keys wherever one was free.
//
// The scheme:
//
//   - A plain letter does something to what's in front of you: add this
//     card, remove it, swap the panel.
//   - The leader, comma, is followed by one letter and goes somewhere else:
//     another screen, or something you do once a session rather than once a
//     card. A hint bar lists them, so nothing has to be remembered.
//   - q leaves whatever you're on, and leaves the app from the main screen.
//     esc peels one layer at a time and quits when there's nothing left.
//
// The leader is a comma, which is a character you can type in a query, so
// in the search bar it only opens the menu when the query is empty. That's
// the case that matters: a fresh launch, where the search bar has focus and
// there is no list to press a letter in.

const leaderKey = ","

// ── The leader menu ─────────────────────────────────────────────

type leaderCmd struct {
	key   string
	what  string // in the reference, where there's room to be clear
	short string // in the hint bar on a narrow terminal
	run   func(model) (tea.Model, tea.Cmd)
}

// leaderCmds is the menu, in the order it's shown.
var leaderCmds = []leaderCmd{
	{"d", "Decks", "Decks", func(m model) (tea.Model, tea.Cmd) { return m.openDeckPicker() }},
	{"n", "New deck", "New", func(m model) (tea.Model, tea.Cmd) { return m.openNewDeckPrompt() }},
	{"g", "Deck history", "History", func(m model) (tea.Model, tea.Cmd) { return m.openDeckHistory() }},
	{"i", "Import this deck as mine", "Import", func(m model) (tea.Model, tea.Cmd) { return m.saveCurrentDeck(), nil }},
	{"m", "Someone's decks on Moxfield", "Moxfield", func(m model) (tea.Model, tea.Cmd) { return m.openMoxUserPrompt() }},
	{"r", "Rules browser", "Rules", func(m model) (tea.Model, tea.Cmd) { return m.openRulesBrowser(nil, "") }},
	{"s", "Query syntax", "Syntax", func(m model) (tea.Model, tea.Cmd) { return m.openSyntaxHelp() }},
	{"k", "Keys", "Keys", func(m model) (tea.Model, tea.Cmd) { return m.openKeyReference() }},
}

// handleLeader runs the command a key names, or cancels. Anything unknown
// cancels rather than doing something surprising.
func (m model) handleLeader(key string) (tea.Model, tea.Cmd) {
	m.leader = false
	for _, c := range leaderCmds {
		if c.key == key {
			return c.run(m)
		}
	}
	return m, nil
}

// leaderBar is the hint shown while the leader is waiting for its second
// key, so the menu never has to be memorised. It wraps onto as many lines as
// it needs rather than being cut off — a menu clipped halfway hides exactly
// the entries you opened it to read.
func (m model) leaderBar(width int) string {
	return strings.Join(m.leaderBarLines(width), "\n")
}

func (m model) leaderBarLines(width int) []string {
	keyStyle := lipgloss.NewStyle().Foreground(gruvBg).Background(gruvYellow).Bold(true)
	whatStyle := lipgloss.NewStyle().Foreground(gruvFg)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	const gap = "  "

	// Full labels if the whole menu fits on one line, short ones otherwise.
	long := true
	if leaderBarWidth(true, gap) > width {
		long = false
	}

	var lines []string
	var cur string
	curW := 0
	for _, c := range leaderCmds {
		label := c.short
		if long {
			label = c.what
		}
		w := 3 + 1 + runeLen(label) + runeLen(gap)
		if curW > 0 && curW+w > width {
			lines = append(lines, cur)
			cur, curW = "", 0
		}
		cur += keyStyle.Render(" "+c.key+" ") + whatStyle.Render(" "+label) + dimStyle.Render(gap)
		curW += w
	}
	if cur != "" {
		lines = append(lines, cur)
	}

	for i := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(width).Render(lines[i])
	}
	return lines
}

func leaderBarWidth(long bool, gap string) int {
	total := 0
	for _, c := range leaderCmds {
		label := c.short
		if long {
			label = c.what
		}
		total += 3 + 1 + runeLen(label) + runeLen(gap)
	}
	return total
}

// ── The key reference ───────────────────────────────────────────

// binding is one key and what it does. `what` is the sentence the reference
// prints; `hint` is the two-or-three words the hint line has room for, and
// an empty one keeps the binding out of that line. needsDeck holds back the
// ones that would only mislead with no deck open.
type binding struct {
	keys      string
	what      string
	hint      string
	needsDeck bool
}

type keyGroup struct {
	title    string
	bindings []binding
}

// keysFor is what `?` shows: the keys for the screen you're on, and nothing
// else. A reference you have to scroll past three other screens to use is
// one you stop opening.
//
// It's also what the hint line at the bottom is built from, so the two can't
// come to disagree — which they had, with `?` explained in three places at
// once and in one of them by the wrong name.
func keysFor(s state) []keyGroup {
	switch s {
	case stateRules:
		return []keyGroup{{"Rules browser", []binding{
			{keys: "↑/↓, j/k", what: "Move through the rules"},
			{keys: "/", what: "Search the rules and glossary", hint: "search"},
			{keys: "g", what: "Switch between rules and glossary", hint: "glossary"},
			{keys: "J/K", what: "Scroll the rule text", hint: "scroll"},
			{keys: "?", what: "These keys", hint: "keys"},
			{keys: "esc, q", what: "Back", hint: "back"},
		}}}

	case stateDeckHistory:
		return []keyGroup{{"Deck history", []binding{
			{keys: "↑/↓, j/k", what: "Move through the versions"},
			{keys: "J/K", what: "Scroll the diff", hint: "scroll"},
			{keys: "/", what: "Search the history", hint: "search"},
			{keys: "enter", what: "Restore this version (as a new commit)", hint: "restore"},
			{keys: "?", what: "These keys", hint: "keys"},
			{keys: "esc, q", what: "Back", hint: "back"},
		}}}

	case stateDecks:
		return []keyGroup{{"Decks", []binding{
			{keys: "↑/↓, j/k", what: "Move through your decks"},
			{keys: "enter", what: "Open it", hint: "open"},
			{keys: "n", what: "Start a new deck", hint: "new deck"},
			{keys: "m", what: "Someone's decks on Moxfield", hint: "moxfield"},
			{keys: "/", what: "Filter by name or format", hint: "filter"},
			{keys: "?", what: "These keys", hint: "keys"},
			{keys: "esc, q", what: "Back", hint: "back"},
		}}}

	case stateMoxUser:
		return []keyGroup{{"Decks on Moxfield", []binding{
			{keys: "↑/↓, j/k", what: "Move through their decks"},
			{keys: "enter, i", what: "Import it as one of yours", hint: "import"},
			{keys: "b", what: "Browse it without importing", hint: "browse"},
			{keys: "u", what: "Someone else's decks", hint: "someone else"},
			{keys: "/", what: "Filter by name or format", hint: "filter"},
			{keys: "?", what: "These keys", hint: "keys"},
			{keys: "esc, q", what: "Back", hint: "back"},
		}}}

	case stateHelp:
		return []keyGroup{{"Query syntax", []binding{
			{keys: "↑/↓, j/k", what: "Scroll"},
			{keys: "d/u", what: "Page down / up"},
			{keys: "?", what: "These keys", hint: "keys"},
			{keys: "esc, q", what: "Back", hint: "back"},
		}}}
	}

	// The main screen, which has most of them.
	return []keyGroup{
		{"The search bar", []binding{
			{keys: "enter", what: "Run the search", hint: "search"},
			{keys: "↑/↓", what: "Walk back through the queries you've run", hint: "history"},
			{keys: "tab", what: "Cycle the order the query asks Scryfall for", hint: "query order"},
			{keys: "pgup/pgdn", what: "Move through the results without leaving the bar"},
			{keys: "esc", what: "Back to the list"},
		}},
		{"Move", []binding{
			{keys: "↑/↓, j/k", what: "Through the list"},
			{keys: "tab", what: "Between the search results and the deck", hint: "switch list", needsDeck: true},
			{keys: "i", what: "Edit the search query", hint: "search"},
			{keys: "/", what: "Filter by name or oracle text", hint: "filter"},
			{keys: "o / O", what: "Reorder the list — name, mana value, type, colour, rank", hint: "sort"},
			{keys: "ctrl+d, ctrl+u", what: "Scroll the panel half a screen"},
			{keys: "esc", what: "Clear marks, then the filter, then the category, then quit"},
			{keys: "q", what: "Quit"},
		}},
		{"The panel", []binding{
			{keys: "r", what: "Rules this card invokes — from the card view", hint: "rules"},
			{keys: "t", what: "How its printed text changed — from the card view", hint: "text"},
			{keys: "s", what: "Statistics for the list", hint: "stats"},
			{keys: "enter", what: "Back to the card from any of them", hint: "card"},
			{keys: "R", what: "In the rules panel: open them in the browser"},
			{keys: "J/K", what: "Scroll the panel; in statistics, walk the categories"},
		}},
		{"The deck", []binding{
			{keys: "a", what: "Add the selected card", hint: "add", needsDeck: true},
			{keys: "x", what: "Remove it", hint: "remove", needsDeck: true},
			{keys: "c", what: "Mark it a commander, or unmark it", needsDeck: true},
			{keys: "+ / -", what: "Another copy, or one fewer", needsDeck: true},
			{keys: "u", what: "Undo the last change", hint: "undo", needsDeck: true},
			{keys: "w", what: "Write the deck now", needsDeck: true},
		}},
		{"Tagging", []binding{
			{keys: "space", what: "Mark this card, and step to the next", needsDeck: true},
			{keys: "v", what: "Mark everything the list is showing", needsDeck: true},
			{keys: "V", what: "Clear the marks", needsDeck: true},
			{keys: "T", what: "Tag the marked cards — a leading - removes", hint: "tag", needsDeck: true},
		}},
		{"Elsewhere", leaderBindings()},
	}
}

// ── The hint line ───────────────────────────────────────────────

// hintLine is the one line of key hints at the bottom of the screen. One
// line, in one place: the list's own help line is off and the panel doesn't
// carry its own, so nothing is said twice.
func (m model) hintLine(width int) string {
	keyStyle := lipgloss.NewStyle().Foreground(gruvYellow)
	whatStyle := lipgloss.NewStyle().Foreground(gruvGray)

	render := func(b binding) string {
		return keyStyle.Render(b.keys) + " " + whatStyle.Render(b.hint)
	}
	cost := func(b binding) int { return runeLen(b.keys) + 1 + runeLen(b.hint) + 2 }

	// The keys that lead to every other key go on last but are budgeted for
	// first: a narrow terminal should lose "rules" and "stats" before it
	// loses the two hints that would have told you about them.
	var lead, tail []binding
	for _, g := range keysFor(m.state) {
		// On the main screen the bar and the lists want different halves of
		// the table; everywhere else there's only one thing to be doing.
		if m.state == stateResults && (g.title == "The search bar") != m.searchFocused() {
			continue
		}
		for _, b := range g.bindings {
			if b.hint == "" || (b.needsDeck && !m.deckOpen()) {
				continue
			}
			if b.keys == leaderKey || b.keys == "?" {
				tail = append(tail, b)
				continue
			}
			lead = append(lead, b)
		}
	}

	budget := width - 1
	for _, b := range tail {
		budget -= cost(b)
	}

	var parts []string
	for _, b := range lead {
		if budget-cost(b) < 0 {
			break
		}
		budget -= cost(b)
		parts = append(parts, render(b))
	}
	for _, b := range tail {
		parts = append(parts, render(b))
	}

	return lipgloss.NewStyle().MaxWidth(width).
		Render(" " + strings.Join(parts, whatStyle.Render("  ")))
}

// leaderBindings turns the leader menu into reference rows, so the two can't
// disagree about what the leader does.
func leaderBindings() []binding {
	out := make([]binding, 0, len(leaderCmds)+1)
	for _, c := range leaderCmds {
		out = append(out, binding{keys: leaderKey + c.key, what: c.what})
	}
	return append(out,
		binding{keys: leaderKey, what: "The menu above", hint: "more"},
		binding{keys: "?", what: "These keys", hint: "keys"})
}

// ── The reference screen ────────────────────────────────────────

func (m model) openKeyReference() (tea.Model, tea.Cmd) {
	m = m.enterState(stateKeys)
	m.keysScroll = 0
	return m, nil
}

func (m model) openSyntaxHelp() (tea.Model, tea.Cmd) {
	m = m.enterState(stateHelp)
	m.helpScroll = 0
	return m, nil
}

func (m model) updateKeyReference(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q", "?":
		return m.leaveState(), nil
	case "down", "j":
		m.keysScroll++
	case "up", "k":
		if m.keysScroll > 0 {
			m.keysScroll--
		}
	}
	return m, nil
}

func (m model) viewKeyReference() string {
	// The reference is for the screen it was opened from, not for itself.
	groups := keysFor(m.cameFrom())

	titleStyle := lipgloss.NewStyle().Foreground(gruvBg).Background(gruvAqua).Bold(true).Padding(0, 1)
	groupStyle := lipgloss.NewStyle().Foreground(gruvOrange).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(gruvYellow)
	whatStyle := lipgloss.NewStyle().Foreground(gruvFg)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	// Line the descriptions up across every group.
	width := 0
	for _, g := range groups {
		for _, b := range g.bindings {
			if n := runeLen(b.keys); n > width {
				width = n
			}
		}
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Keys") + "\n\n")
	for _, g := range groups {
		b.WriteString(groupStyle.Render(g.title) + "\n")
		for _, bind := range g.bindings {
			b.WriteString("  " + keyStyle.Render(padTo(bind.keys, width)) +
				"  " + whatStyle.Render(bind.what) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render("j/k: scroll   esc, q: back"))

	return lipgloss.NewStyle().Padding(1, 2).Render(
		scrollView(b.String(), m.keysScroll, m.height-2))
}

// ── The CLI's own help ──────────────────────────────────────────

const cliUsage = `scry — Magic: The Gathering cards, rules and decks in the terminal

Usage:
  scry                      Open the app
  scry <query>              Run a Scryfall query; one result prints to stdout
  scry <moxfield url>       Browse a public Moxfield deck
  scry deck …               Your decks — see ` + "`scry deck`" + `
  scry rules …              The comprehensive rules — see ` + "`scry rules`" + `
  scry -h, --help           This

Queries use Scryfall's own syntax, and the app's ,s shows a reference:
  scry 't:creature c:rw cmc<=3'
  scry 'o:"draw a card" f:commander'

Data lives in ~/.local/share/scry/. Decks are files in a git repository —
SCRY_DECKS_DIR moves them somewhere else.`

func printUsage() { fmt.Println(cliUsage) }
