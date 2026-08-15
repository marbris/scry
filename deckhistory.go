package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateDeckHistory: what you've done to the open deck, and the way back.
// The commit list is on the left and the change it made on the right, so
// finding the version you want is a matter of scrolling until the diff looks
// familiar rather than remembering a hash.

// historyLimit is how far back the browser reads. Longer histories are still
// all there in git; this is just what one screen is willing to list.
const historyLimit = 200

type commitItem struct{ commit deckCommit }

func (c commitItem) Title() string       { return c.commit.subject }
func (c commitItem) Description() string { return c.commit.when }
func (c commitItem) FilterValue() string { return c.commit.subject + " " + c.commit.short }

type commitDelegate struct{}

func (d commitDelegate) Height() int                             { return 1 }
func (d commitDelegate) Spacing() int                            { return 0 }
func (d commitDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d commitDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(commitItem)
	if !ok {
		return
	}

	selected := index == m.Index()
	subjectStyle := lipgloss.NewStyle().Foreground(gruvFg)
	hashStyle := lipgloss.NewStyle().Foreground(gruvYellow)
	whenStyle := lipgloss.NewStyle().Foreground(gruvGray)
	cursor := "  "
	if selected {
		cursor = lipgloss.NewStyle().Foreground(gruvOrange).Render("▌ ")
		subjectStyle = subjectStyle.Bold(true)
	}

	// hash, then as much of the subject as fits, then how long ago.
	width := m.Width() - 4
	when := it.commit.when
	subjectW := width - len(it.commit.short) - runeLen(when) - 3
	if subjectW < 8 {
		subjectW = 8
		when = ""
	}

	line := cursor + hashStyle.Render(it.commit.short) + " " +
		subjectStyle.Render(truncate(it.commit.subject, subjectW))
	if when != "" {
		pad := width - len(it.commit.short) - runeLen(truncate(it.commit.subject, subjectW)) - runeLen(when)
		if pad < 1 {
			pad = 1
		}
		line += strings.Repeat(" ", pad) + whenStyle.Render(when)
	}
	fmt.Fprint(w, line)
}

// ── Messages ────────────────────────────────────────────────────

type deckHistoryMsg struct {
	slug    string
	commits []deckCommit
	err     error
}

type deckDiffMsg struct {
	hash string
	diff string
	err  error
}

func loadDeckHistoryCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		commits, err := deckHistory(slug, historyLimit)
		return deckHistoryMsg{slug: slug, commits: commits, err: err}
	}
}

func loadDeckDiffCmd(slug, hash string) tea.Cmd {
	return func() tea.Msg {
		diff, err := deckDiff(slug, hash)
		return deckDiffMsg{hash: hash, diff: diff, err: err}
	}
}

func restoreDeckCmd(slug, hash string) tea.Cmd {
	return func() tea.Msg {
		if err := restoreDeck(slug, hash); err != nil {
			return deckLoadedMsg{err: err}
		}
		// Reopening is what puts the restored deck on screen.
		info, cards, err := openLocalDeck(slug)
		return deckLoadedMsg{info: info, cards: cards, err: err}
	}
}

// ── Opening ─────────────────────────────────────────────────────

// openDeckHistory switches to the history browser for the open deck. Only a
// deck of your own has one: a deck being browsed off Moxfield isn't yours to
// have a history of.
func (m model) openDeckHistory() (tea.Model, tea.Cmd) {
	if m.deck == nil || !m.deck.local() {
		m.notice = "no history — open one of your own decks first"
		return m, nil
	}
	if !gitAvailable() {
		m.notice = gitNotInstalled
		return m, nil
	}

	m.prevState = m.state
	m.state = stateDeckHistory
	m.historyList = newCommitList(m.width, m.height)
	m.historyDiff = ""
	m.historyErr = nil
	m.diffScroll = 0
	return m, loadDeckHistoryCmd(m.deck.slug)
}

func newCommitList(width, height int) list.Model {
	w := width / 2
	if w < 20 {
		w = 20
	}
	l := list.New(nil, commitDelegate{}, w, height)
	l.Title = "History"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Foreground(gruvBg).Background(gruvAqua).Padding(0, 1)
	return l
}

// ── Update ──────────────────────────────────────────────────────

func (m model) updateDeckHistory(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case deckHistoryMsg:
		m.historyErr = msg.err
		items := make([]list.Item, len(msg.commits))
		for i, c := range msg.commits {
			items[i] = commitItem{commit: c}
		}
		m.historyList.SetItems(items)
		m.historyList.Title = fmt.Sprintf("History (%d)", len(items))
		return m, m.syncDiff()

	case deckDiffMsg:
		// A diff that arrives after the cursor has moved on is stale.
		if sel, ok := m.historyList.SelectedItem().(commitItem); ok && sel.commit.hash != msg.hash {
			return m, nil
		}
		m.historyDiff = msg.diff
		m.historyErr = msg.err
		m.diffScroll = 0
		return m, nil

	case tea.KeyMsg:
		if m.historyList.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "?":
			return m.openKeyReference()
		case "esc", "q":
			m.state = m.prevState
			return m, nil
		case "J", "shift+down":
			m.diffScroll++
			return m, nil
		case "K", "shift+up":
			if m.diffScroll > 0 {
				m.diffScroll--
			}
			return m, nil
		case "enter", "R":
			// Restoring writes a new commit rather than rewinding, so this
			// needs no confirmation — nothing is lost by trying it.
			sel, ok := m.historyList.SelectedItem().(commitItem)
			if !ok || m.deck == nil {
				return m, nil
			}
			m.state = m.prevState
			m.notice = "restored to " + sel.commit.short
			return m, restoreDeckCmd(m.deck.slug, sel.commit.hash)
		}
	}

	before := m.historyList.Index()
	var cmd tea.Cmd
	m.historyList, cmd = m.historyList.Update(msg)
	if m.historyList.Index() != before {
		return m, tea.Batch(cmd, m.syncDiff())
	}
	return m, cmd
}

// syncDiff fetches the diff for whichever commit is under the cursor.
func (m model) syncDiff() tea.Cmd {
	sel, ok := m.historyList.SelectedItem().(commitItem)
	if !ok || m.deck == nil {
		return nil
	}
	return loadDeckDiffCmd(m.deck.slug, sel.commit.hash)
}

// ── View ────────────────────────────────────────────────────────

func (m model) viewDeckHistory() string {
	listW := m.width / 2
	if listW < 20 {
		listW = 20
	}
	m.historyList.SetSize(listW, m.height)

	diffW := m.width - listW - 4
	if diffW < 30 {
		diffW = 30
	}

	var body string
	switch {
	case m.historyErr != nil:
		body = lipgloss.NewStyle().Foreground(gruvRed).Render(m.historyErr.Error())
	case len(m.historyList.Items()) == 0:
		body = lipgloss.NewStyle().Foreground(gruvGray).
			Render("No history yet — this deck hasn't changed since you saved it.")
	default:
		body = renderDiff(m.historyDiff, diffW-4)
	}

	hint := lipgloss.NewStyle().Foreground(gruvGray).
		Render("enter: restore this version  J/K: scroll  /: search  esc: back")
	body = scrollView(body, m.diffScroll, m.height-3) + "\n" + hint

	diffStyle := lipgloss.NewStyle().
		Width(diffW).
		Height(m.height).
		Padding(1, 2).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(gruvGray)

	listView := lipgloss.NewStyle().MaxWidth(listW).Render(m.historyList.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, diffStyle.Render(body))
}

// renderDiff colours a unified diff the way git does, and drops the header
// lines that only repeat what the list already says.
func renderDiff(diff string, width int) string {
	if strings.TrimSpace(diff) == "" {
		return lipgloss.NewStyle().Foreground(gruvGray).Render("No changes recorded.")
	}

	addStyle := lipgloss.NewStyle().Foreground(gruvGreen)
	delStyle := lipgloss.NewStyle().Foreground(gruvRed)
	hunkStyle := lipgloss.NewStyle().Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	var b strings.Builder
	started := false
	for _, line := range strings.Split(diff, "\n") {
		// A commit with no body still prints one, so the patch would
		// otherwise start a couple of blank lines down.
		if !started {
			if strings.TrimSpace(line) == "" {
				continue
			}
			started = true
		}
		switch {
		case strings.HasPrefix(line, "diff --git"),
			strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "+++ "),
			strings.HasPrefix(line, "new file"),
			strings.HasPrefix(line, "deleted file"):
			continue
		case strings.HasPrefix(line, "@@"):
			b.WriteString(hunkStyle.Render(truncate(line, width)) + "\n")
		case strings.HasPrefix(line, "+"):
			b.WriteString(addStyle.Render(truncate(line, width)) + "\n")
		case strings.HasPrefix(line, "-"):
			b.WriteString(delStyle.Render(truncate(line, width)) + "\n")
		default:
			b.WriteString(dimStyle.Render(truncate(line, width)) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
