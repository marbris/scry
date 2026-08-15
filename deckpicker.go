package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateDecks: the decks you have, so opening one doesn't mean quitting to
// the shell to remember what it was called. Reading a deck file is cheap —
// no cards are resolved — so the whole list can be built up front and each
// deck described properly rather than by name alone.

type deckSummary struct {
	slug    string
	name    string
	format  string
	total   int
	unique  int
	source  string
	changed string // when it last changed, as git tells it
	err     error  // a deck file that won't parse still gets a row
}

type deckListItem struct{ deck deckSummary }

func (d deckListItem) Title() string       { return d.deck.name }
func (d deckListItem) Description() string { return d.deck.format }
func (d deckListItem) FilterValue() string {
	return d.deck.slug + " " + d.deck.name + " " + d.deck.format
}

type deckPickerDelegate struct{}

func (d deckPickerDelegate) Height() int                             { return 2 }
func (d deckPickerDelegate) Spacing() int                            { return 1 }
func (d deckPickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d deckPickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(deckListItem)
	if !ok {
		return
	}
	dk := it.deck
	width := m.Width() - 4

	nameStyle := lipgloss.NewStyle().Foreground(gruvFg)
	slugStyle := lipgloss.NewStyle().Foreground(gruvYellow)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	cursor := "  "
	if index == m.Index() {
		cursor = lipgloss.NewStyle().Foreground(gruvOrange).Render("▌ ")
		nameStyle = nameStyle.Bold(true)
	}

	name := dk.name
	if dk.err != nil {
		name = dk.slug
	}
	first := cursor + nameStyle.Render(truncate(name, width))

	var detail string
	switch {
	case dk.err != nil:
		detail = lipgloss.NewStyle().Foreground(gruvRed).Render(truncate(dk.err.Error(), width))
	default:
		parts := []string{dk.slug}
		if dk.format != "" {
			parts = append(parts, dk.format)
		}
		parts = append(parts, fmt.Sprintf("%d cards · %d distinct", dk.total, dk.unique))
		if dk.changed != "" {
			parts = append(parts, dk.changed)
		}
		detail = slugStyle.Render(dk.slug) +
			dimStyle.Render(truncate("  "+strings.Join(parts[1:], "  ·  "), width-runeLen(dk.slug)))
	}

	fmt.Fprint(w, first+"\n  "+detail)
}

// ── Loading ─────────────────────────────────────────────────────

type decksListedMsg struct {
	decks []deckSummary
	err   error
}

func listDecksCmd() tea.Cmd {
	return func() tea.Msg {
		slugs, err := listDecks()
		if err != nil {
			return decksListedMsg{err: err}
		}

		out := make([]deckSummary, 0, len(slugs))
		for _, slug := range slugs {
			s := deckSummary{slug: slug, name: slug}
			d, err := readDeck(slug)
			if err != nil {
				s.err = err
				out = append(out, s)
				continue
			}
			s.name = d.Name
			s.format = d.Format
			s.source = d.Source
			s.total, s.unique = d.counts()

			// The last commit that touched it, which is more use than a
			// modification time when the point is the history.
			if commits, err := deckHistory(slug, 1); err == nil && len(commits) > 0 {
				s.changed = commits[0].when
			}
			out = append(out, s)
		}
		return decksListedMsg{decks: out}
	}
}

// ── Opening ─────────────────────────────────────────────────────

func (m model) openDeckPicker() (tea.Model, tea.Cmd) {
	m.prevState = m.state
	m.state = stateDecks
	m.deckPickerErr = nil

	if len(m.deckPicker.Items()) == 0 {
		m.deckPicker = newDeckPickerList(m.width, m.height)
	}
	return m, listDecksCmd()
}

func newDeckPickerList(width, height int) list.Model {
	w := width
	if w < 20 {
		w = 20
	}
	l := list.New(nil, deckPickerDelegate{}, w, height)
	l.Title = "Decks"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Foreground(gruvBg).Background(gruvAqua).Padding(0, 1)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(gruvYellow)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(gruvOrange)
	return l
}

// ── Update ──────────────────────────────────────────────────────

func (m model) updateDeckPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case decksListedMsg:
		m.deckPickerErr = msg.err
		items := make([]list.Item, len(msg.decks))
		for i, d := range msg.decks {
			items[i] = deckListItem{deck: d}
		}
		m.deckPicker.SetItems(items)
		m.deckPicker.Title = fmt.Sprintf("Decks (%d)", len(items))
		return m, nil

	case tea.KeyMsg:
		if m.deckPicker.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "esc", "q":
			m.state = m.prevState
			return m, nil
		case "enter":
			sel, ok := m.deckPicker.SelectedItem().(deckListItem)
			if !ok {
				return m, nil
			}
			if sel.deck.err != nil {
				m.deckPickerErr = sel.deck.err
				return m, nil
			}
			m.state = m.prevState
			m.deckLoading = true
			m.searching = true
			return m, openLocalDeckCmd(sel.deck.slug)
		}
	}

	var cmd tea.Cmd
	m.deckPicker, cmd = m.deckPicker.Update(msg)
	return m, cmd
}

// ── View ────────────────────────────────────────────────────────

func (m model) viewDeckPicker() string {
	m.deckPicker.SetSize(m.width, m.height-2)

	if m.deckPickerErr != nil {
		return lipgloss.NewStyle().Padding(1, 2).Foreground(gruvRed).
			Render(m.deckPickerErr.Error())
	}
	if len(m.deckPicker.Items()) == 0 {
		return lipgloss.NewStyle().Padding(1, 2).Render(
			lipgloss.NewStyle().Foreground(gruvAqua).Bold(true).Render("No decks yet") + "\n\n" +
				lipgloss.NewStyle().Foreground(gruvGray).Render(
					"Copy one in from Moxfield:\n"+
						"  scry deck import <moxfield url>\n\n"+
						"or paste a Moxfield URL into the search bar and press w.\n\n"+
						"esc: back") + "\n" + legacyNotice())
	}

	hint := lipgloss.NewStyle().Foreground(gruvGray).
		Render("  enter: open  /: filter  esc: back")
	return lipgloss.JoinVertical(lipgloss.Left, m.deckPicker.View(), hint)
}
