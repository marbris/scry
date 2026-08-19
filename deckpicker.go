package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateDecks: the decks you have, so opening one doesn't mean quitting to
// the shell to remember what it was called. Reading a deck file is cheap —
// no cards are resolved — so the whole list can be built up front and each
// deck described properly rather than by name alone.

type deckSummary struct {
	Slug    string
	Name    string
	Format  string
	Total   int
	Unique  int
	source  string
	changed string // when it last changed, as git tells it
	err     error  // a deck file that won't parse still gets a row
}

type deckListItem struct{ deck deckSummary }

func (d deckListItem) Title() string       { return d.deck.Name }
func (d deckListItem) Description() string { return d.deck.Format }
func (d deckListItem) FilterValue() string {
	return d.deck.Slug + " " + d.deck.Name + " " + d.deck.Format
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

	name := dk.Name
	if dk.err != nil {
		name = dk.Slug
	}
	first := cursor + nameStyle.Render(truncate(name, width))

	var detail string
	switch {
	case dk.err != nil:
		detail = lipgloss.NewStyle().Foreground(gruvRed).Render(truncate(dk.err.Error(), width))
	default:
		parts := []string{dk.Slug}
		if dk.Format != "" {
			parts = append(parts, dk.Format)
		}
		parts = append(parts, fmt.Sprintf("%d cards · %d distinct", dk.Total, dk.Unique))
		if dk.changed != "" {
			parts = append(parts, dk.changed)
		}
		detail = slugStyle.Render(dk.Slug) +
			dimStyle.Render(truncate("  "+strings.Join(parts[1:], "  ·  "), width-runeLen(dk.Slug)))
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
			s := deckSummary{Slug: slug, Name: slug}
			d, err := readDeck(slug)
			if err != nil {
				s.err = err
				out = append(out, s)
				continue
			}
			s.Name = d.Name
			s.Format = d.Format
			s.source = d.Source
			s.Total, s.Unique = d.Counts()

			// The last commit that touched it, which is more use than a
			// modification time when the point is the history.
			if commits, err := deckHistory(slug, 1); err == nil && len(commits) > 0 {
				s.changed = commits[0].When
			}
			out = append(out, s)
		}
		return decksListedMsg{decks: out}
	}
}

// ── Opening ─────────────────────────────────────────────────────

func (m model) openDeckPicker() (tea.Model, tea.Cmd) {
	m = m.enterState(stateDecks)
	m.deckPickerErr = nil
	m.naming = false

	if len(m.deckPicker.Items()) == 0 {
		m.deckPicker = newDeckPickerList(m.width, m.height)
	}
	return m, listDecksCmd()
}

// openNewDeckPrompt goes straight to naming a new deck, without stopping at
// the list of the ones you already have.
func (m model) openNewDeckPrompt() (tea.Model, tea.Cmd) {
	next, cmd := m.openDeckPicker()
	opened := next.(model)
	opened.naming = true
	opened.deckNameInput = newDeckNameInput()
	return opened, tea.Batch(cmd, textinput.Blink)
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
	l.Filter = literalFilter
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
		if m.naming {
			return m.updateDeckNaming(msg)
		}
		if m.deckPicker.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "?":
			return m.openKeyReference()
		case "m":
			return m.openMoxUserPrompt()
		case "n":
			m.naming = true
			m.deckNameInput = newDeckNameInput()
			return m, textinput.Blink
		case "esc", "q":
			return m.leaveState(), nil
		case "enter":
			sel, ok := m.deckPicker.SelectedItem().(deckListItem)
			if !ok {
				return m, nil
			}
			if sel.deck.err != nil {
				m.deckPickerErr = sel.deck.err
				return m, nil
			}
			m = m.showResults()
			m.deckLoading = true
			m.searching = true
			return m, openLocalDeckCmd(sel.deck.Slug)
		}
	}

	var cmd tea.Cmd
	m.deckPicker, cmd = m.deckPicker.Update(msg)
	return m, cmd
}

// ── Naming a new deck ───────────────────────────────────────────

func newDeckNameInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "deck name"
	ti.Prompt = "New deck: "
	ti.CharLimit = 80
	ti.Width = 40
	ti.Focus()
	ti.PromptStyle = lipgloss.NewStyle().Foreground(gruvAqua).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(gruvFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(gruvGray)
	return ti
}

func (m model) updateDeckNaming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.naming = false
		return m, nil

	case "enter":
		slug, d, err := newDeck(m.deckNameInput.Value(), "")
		if err != nil {
			m.deckPickerErr = err
			return m, nil
		}
		if _, _, err := saveDeckVersioned(slug, d); err != nil {
			m.deckPickerErr = err
			return m, nil
		}

		// Straight into the deck you just made, rather than back to a list
		// to find it in.
		m.naming = false
		m.deckPickerErr = nil
		m = m.showResults()
		m.deckLoading = true
		return m, openLocalDeckCmd(slug)
	}

	m.deckPickerErr = nil
	var cmd tea.Cmd
	m.deckNameInput, cmd = m.deckNameInput.Update(msg)
	return m, cmd
}

// ── View ────────────────────────────────────────────────────────

func (m model) viewDeckPicker() string {
	m.deckPicker.SetSize(m.width, m.height-3)

	// An error goes above the list rather than in place of it: it's usually
	// something you can act on ("you already have a deck called that"), and
	// replacing the screen would hide the prompt you'd act in.
	errLine := ""
	if m.deckPickerErr != nil {
		errLine = lipgloss.NewStyle().Foreground(gruvRed).
			Render("  " + m.deckPickerErr.Error())
	}

	if len(m.deckPicker.Items()) == 0 && !m.naming {
		return lipgloss.NewStyle().Padding(1, 2).Render(
			lipgloss.NewStyle().Foreground(gruvAqua).Bold(true).Render("No decks yet") + "\n\n" +
				lipgloss.NewStyle().Foreground(gruvGray).Render(
					"n            start an empty one\n"+
						"\n"+
						"or copy one in from Moxfield:\n"+
						"  scry deck import <moxfield url>\n"+
						"or paste a Moxfield URL into the search bar and press w.\n\n"+
						"esc: back") + "\n" + legacyNotice())
	}

	if m.naming {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.deckPicker.View(), errLine,
			"  "+m.deckNameInput.View(),
			lipgloss.NewStyle().Foreground(gruvGray).Render("  enter: create  esc: cancel"))
	}

	hint := m.hintLine(m.width)
	return lipgloss.JoinVertical(lipgloss.Left, m.deckPicker.View(), errLine, hint)
}
