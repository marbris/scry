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

type binding struct{ keys, what string }

type keyGroup struct {
	title    string
	bindings []binding
}

// keysFor is what `?` shows: the keys for the screen you're on, and nothing
// else. A reference you have to scroll past three other screens to use is
// one you stop opening.
func keysFor(s state) []keyGroup {
	switch s {
	case stateRules:
		return []keyGroup{{"Rules browser", []binding{
			{"↑/↓, j/k", "Move through the rules"},
			{"/", "Search the rules and glossary"},
			{"g", "Switch between rules and glossary"},
			{"J/K", "Scroll the rule text"},
			{"esc, q", "Back"},
		}}}

	case stateDeckHistory:
		return []keyGroup{{"Deck history", []binding{
			{"↑/↓, j/k", "Move through the versions"},
			{"J/K", "Scroll the diff"},
			{"/", "Search the history"},
			{"enter", "Restore this version (as a new commit)"},
			{"esc, q", "Back"},
		}}}

	case stateMoxUser:
		return []keyGroup{{"Decks on Moxfield", []binding{
			{"↑/↓, j/k", "Move through their decks"},
			{"enter, i", "Import it as one of yours"},
			{"b", "Browse it without importing"},
			{"u", "Someone else's decks"},
			{"/", "Filter by name or format"},
			{"esc, q", "Back"},
		}}}

	case stateDecks:
		return []keyGroup{{"Decks", []binding{
			{"↑/↓, j/k", "Move through your decks"},
			{"/", "Filter by name or format"},
			{"enter", "Open it"},
			{"n", "Start a new deck"},
			{"m", "Someone's decks on Moxfield"},
			{"esc, q", "Back"},
		}}}

	case stateHelp:
		return []keyGroup{{"Query syntax", []binding{
			{"↑/↓, j/k", "Scroll"},
			{"d/u", "Page down / up"},
			{"esc, q", "Back"},
		}}}
	}

	// The main screen, which has most of them.
	return []keyGroup{
		{"Move", []binding{
			{"↑/↓, j/k", "Through the list"},
			{"tab", "Between the search results and the deck"},
			{"i", "Edit the search query"},
			{"/", "Filter by name or oracle text"},
			{"J/K", "Scroll the panel"},
			{"ctrl+d, ctrl+u", "Scroll the panel half a screen"},
			{"esc", "Clear marks, then the filter, then the category, then quit"},
			{"q", "Quit"},
		}},
		{"The panel", []binding{
			{"r", "Rules this card invokes"},
			{"s", "Statistics for the list"},
			{"t", "How the card's printed text changed"},
			{"J/K", "In statistics: walk the categories, narrowing the list"},
			{"enter", "Browse the rules this card matched"},
		}},
		{"The deck", []binding{
			{"a", "Add the selected card"},
			{"x", "Remove it"},
			{"c", "Mark it a commander, or unmark it"},
			{"+ / -", "Another copy, or one fewer"},
			{"u", "Undo the last change"},
			{"w", "Write the deck now"},
		}},
		{"Tagging", []binding{
			{"space", "Mark this card, and step to the next"},
			{"v", "Mark everything the list is showing"},
			{"V", "Clear the marks"},
			{"T", "Tag the marked cards — a leading - removes"},
		}},
		{"Elsewhere", leaderBindings()},
	}
}

// leaderBindings turns the leader menu into reference rows, so the two can't
// disagree about what the leader does.
func leaderBindings() []binding {
	out := make([]binding, 0, len(leaderCmds)+1)
	for _, c := range leaderCmds {
		out = append(out, binding{leaderKey + c.key, c.what})
	}
	return append(out, binding{"?", "These keys"})
}

// ── The reference screen ────────────────────────────────────────

func (m model) openKeyReference() (tea.Model, tea.Cmd) {
	m.prevState = m.state
	m.state = stateKeys
	m.keysScroll = 0
	return m, nil
}

func (m model) openSyntaxHelp() (tea.Model, tea.Cmd) {
	m.prevState = m.state
	m.state = stateHelp
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
		m.state = m.prevState
		return m, nil
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
	groups := keysFor(m.prevState)

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
