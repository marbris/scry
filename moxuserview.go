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

// stateMoxUser: someone's decks on Moxfield, listed so one can be imported
// or opened without going to a browser to find its URL first.

type moxDeckItem struct {
	deck     moxUserDeck
	imported bool // already one of yours, under this name
}

func (d moxDeckItem) Title() string       { return d.deck.Name }
func (d moxDeckItem) Description() string { return d.deck.Format }
func (d moxDeckItem) FilterValue() string {
	return d.deck.Name + " " + d.deck.Format
}

type moxDeckDelegate struct{}

func (d moxDeckDelegate) Height() int                             { return 2 }
func (d moxDeckDelegate) Spacing() int                            { return 1 }
func (d moxDeckDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d moxDeckDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(moxDeckItem)
	if !ok {
		return
	}
	dk := it.deck
	width := m.Width() - 4

	nameStyle := lipgloss.NewStyle().Foreground(gruvFg)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	cursor := "  "
	if index == m.Index() {
		cursor = lipgloss.NewStyle().Foreground(gruvOrange).Render("▌ ")
		nameStyle = nameStyle.Bold(true)
	}

	first := cursor + nameStyle.Render(truncate(dk.Name, width-12))
	if it.imported {
		first += lipgloss.NewStyle().Foreground(gruvAqua).Render("  ✓ yours")
	}

	parts := []string{fmt.Sprintf("%d cards", dk.Cards)}
	if dk.Format != "" {
		parts = append([]string{dk.Format}, parts...)
	}
	if len(dk.Colors) > 0 {
		parts = append(parts, strings.Join(dk.Colors, ""))
	}
	if age := dk.age(); age != "" {
		parts = append(parts, age)
	}
	second := "  " + dimStyle.Render(truncate(strings.Join(parts, "  ·  "), width))

	fmt.Fprint(w, first+"\n"+second)
}

// ── Opening ─────────────────────────────────────────────────────

// openMoxUserPrompt asks whose decks to list.
func (m model) openMoxUserPrompt() (tea.Model, tea.Cmd) {
	m.prevState = m.state
	m.state = stateMoxUser
	m.moxUserErr = nil
	m.moxUserAsking = true
	m.moxUserInput = newMoxUserInput()
	m.moxUserList = newMoxUserList(m.width, m.height)
	return m, textinput.Blink
}

func newMoxUserInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "moxfield user name, or a link to their profile"
	ti.Prompt = "Whose decks: "
	ti.CharLimit = 120
	ti.Width = 48
	ti.Focus()
	ti.PromptStyle = lipgloss.NewStyle().Foreground(gruvAqua).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(gruvFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(gruvGray)
	return ti
}

func newMoxUserList(width, height int) list.Model {
	w := width
	if w < 20 {
		w = 20
	}
	l := list.New(nil, moxDeckDelegate{}, w, height)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Filter = literalFilter
	l.Styles.Title = lipgloss.NewStyle().Foreground(gruvBg).Background(gruvAqua).Padding(0, 1)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(gruvYellow)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(gruvOrange)
	return l
}

// ── Update ──────────────────────────────────────────────────────

func (m model) updateMoxUser(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case moxUserDecksMsg:
		m.moxUserLoading = false
		m.moxUserErr = msg.err
		if msg.err != nil {
			// Back to the prompt, with the name still in it to correct.
			m.moxUserAsking = true
			return m, textinput.Blink
		}

		// Whatever asked for these, the answer is what's on screen now —
		// otherwise a reply arriving while the prompt is back up would fill
		// a list nobody can see.
		m.moxUserAsking = false
		m.moxUser = msg.user
		items := make([]list.Item, len(msg.decks))
		for i, d := range msg.decks {
			items[i] = moxDeckItem{deck: d, imported: deckExists(slugify(d.Name))}
		}
		m.moxUserList.SetItems(items)
		m.moxUserList.Title = fmt.Sprintf("%s · %d decks", msg.user, len(items))
		return m, nil

	case deckImportedMsg:
		m.moxUserLoading = false
		if msg.err != nil {
			m.moxUserErr = msg.err
			return m, nil
		}
		// Straight into the deck that was just imported.
		m.state = m.prevState
		m.deckLoading = true
		return m, openLocalDeckCmd(msg.slug)

	case tea.KeyMsg:
		if m.moxUserAsking {
			return m.updateMoxUserPrompt(msg)
		}
		if m.moxUserList.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "?":
			return m.openKeyReference()
		case "esc", "q":
			m.state = m.prevState
			return m, nil
		case "u":
			// Ask again, for someone else.
			m.moxUserAsking = true
			m.moxUserInput = newMoxUserInput()
			return m, textinput.Blink
		case "enter", "i":
			sel, ok := m.moxUserList.SelectedItem().(moxDeckItem)
			if !ok {
				return m, nil
			}
			m.moxUserLoading = true
			m.moxUserErr = nil
			return m, importMoxDeckCmd(sel.deck.PublicID)
		case "b":
			// Browse it without importing, the way a pasted URL does.
			sel, ok := m.moxUserList.SelectedItem().(moxDeckItem)
			if !ok {
				return m, nil
			}
			m.state = m.prevState
			m.deckLoading = true
			m.searching = true
			return m, loadDeckCmd(sel.deck.PublicID)
		}
	}

	var cmd tea.Cmd
	m.moxUserList, cmd = m.moxUserList.Update(msg)
	return m, cmd
}

func (m model) updateMoxUserPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// With nothing listed there's nothing to go back to.
		if len(m.moxUserList.Items()) == 0 {
			m.state = m.prevState
			m.moxUserAsking = false
			return m, nil
		}
		m.moxUserAsking = false
		return m, nil

	case "enter":
		name := strings.TrimSpace(m.moxUserInput.Value())
		if name == "" {
			return m, nil
		}
		m.moxUserAsking = false
		m.moxUserLoading = true
		m.moxUserErr = nil
		return m, fetchMoxUserDecksCmd(name)
	}

	m.moxUserErr = nil
	var cmd tea.Cmd
	m.moxUserInput, cmd = m.moxUserInput.Update(msg)
	return m, cmd
}

// ── Importing ───────────────────────────────────────────────────

type deckImportedMsg struct {
	slug string
	err  error
}

func importMoxDeckCmd(id string) tea.Cmd {
	return func() tea.Msg {
		d, err := importMoxfield(id)
		if err != nil {
			return deckImportedMsg{err: err}
		}
		slug := slugify(d.Name)
		if slug == "" {
			slug = id
		}
		if _, _, err := saveDeckVersioned(slug, d); err != nil {
			return deckImportedMsg{err: err}
		}
		return deckImportedMsg{slug: slug}
	}
}

// ── View ────────────────────────────────────────────────────────

func (m model) viewMoxUser() string {
	dim := lipgloss.NewStyle().Foreground(gruvGray)

	errLine := ""
	if m.moxUserErr != nil {
		errLine = lipgloss.NewStyle().Foreground(gruvRed).Render("  " + m.moxUserErr.Error())
	}

	if m.moxUserAsking {
		return lipgloss.NewStyle().Padding(1, 2).Render(
			lipgloss.NewStyle().Foreground(gruvAqua).Bold(true).Render("Decks on Moxfield") + "\n\n" +
				m.moxUserInput.View() + "\n" + errLine + "\n\n" +
				dim.Render("Only public decks are listed — Moxfield doesn't show\n"+
					"anyone else's private or unlisted ones.\n\n"+
					"enter: list them   esc: back"))
	}

	if m.moxUserLoading {
		return lipgloss.NewStyle().Padding(1, 2).Render(dim.Render("Asking Moxfield…"))
	}

	m.moxUserList.SetSize(m.width, m.height-3)
	hint := dim.Render("  enter: import   b: browse without importing   " +
		"u: someone else   /: filter   esc: back")
	return lipgloss.JoinVertical(lipgloss.Left, m.moxUserList.View(), errLine, hint)
}
