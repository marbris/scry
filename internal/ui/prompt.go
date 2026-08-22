package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/theme"
)

// A one-line question, asked in the panel's header where the search bar
// normally sits.
//
// Naming a new deck and renaming one are the same interaction with different
// consequences, so they are the same prompt with different labels. It lives
// in the header rather than a dialogue because a panel twenty columns wide
// has no room for a dialogue, and because the answer belongs to this panel
// rather than to the screen.

type askKind int

const (
	askNone askKind = iota
	askNewDeck
	askRename
	askFollow
	askTag
	askWrite
)

// ask raises the prompt.
func (p *panel) ask(kind askKind, label, initial string) {
	p.asking = kind
	p.askInput = textinput.New()
	p.askInput.Prompt = label + ": "
	p.askInput.PromptStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	p.askInput.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	p.askInput.SetValue(initial)
	p.askInput.CursorEnd()
	p.askInput.Focus()
}

func (p *panel) stopAsking() {
	p.asking = askNone
	p.askInput.Blur()
}

// handleAskKey runs the prompt. Enter acts, esc abandons, and everything
// else is typing — there is nothing else a one-line question needs.
func (m Model) handleAskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.ws.current()

	switch msg.String() {
	case "esc":
		p.stopAsking()
		return m, nil

	case "enter":
		kind, answer := p.asking, p.askInput.Value()
		p.stopAsking()
		if answer == "" {
			return m, nil
		}

		switch kind {
		case askNewDeck:
			dir := ""
			if l, ok := p.top().(*deckList); ok {
				dir = l.currentFolder()
			}
			return m, newDeckCmd(dir, answer)
		case askFollow:
			return m, follow(answer)
		case askRename:
			if l, ok := p.top().(*deckList); ok {
				if e, ok := l.current(); ok {
					return m, renameCmd(e, answer)
				}
			}

		case askTag:
			if l := p.cardsView(); l != nil {
				m.tag(l.selection(), answer)
			}

		case askWrite:
			if l := p.cardsView(); l != nil {
				return m, saveAsNew(p.id, p.writeToNewPane, answer, l.all)
			}
		}
		return m, nil

	case "ctrl+c":
		return m, m.quit()
	}

	var cmd tea.Cmd
	p.askInput, cmd = p.askInput.Update(msg)
	return m, cmd
}
