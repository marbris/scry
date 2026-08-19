// Package ui is the panel workspace: a row of panels you can open, close and
// move between, with one information panel pinned to the right.
//
// It deliberately stays one package. Panels hold views, views open panels,
// and the workspace holds panels — mutually referential by nature, so
// splitting it further would mean inventing interfaces to satisfy the
// compiler rather than to explain anything. Filenames do the organising.
package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/theme"
)

// Model is the Bubbletea model for the whole program.
type Model struct {
	ws   workspace
	info infoPanel

	// leader is set between pressing the leader key and the key that says
	// what to do with it; showKeys is the full reference, which is a lid
	// rather than a screen.
	leader   bool
	showKeys bool

	// history is every query run, shared by every find panel: searches you
	// ran in one panel are worth recalling in the next.
	history []string

	width, height int
}

// defaultQuerySort is EDHREC rank — for a Commander player the cards other
// people actually play are the ones worth seeing first.
const defaultQuerySort = 9

func New() Model {
	return Model{ws: newWorkspace(), history: LoadQueryHistory()}
}

// NewWithQuery opens straight onto a search, for `scry --panels <query>`.
func NewWithQuery(query string) (Model, tea.Cmd) {
	m := New()
	p := m.ws.open(KindFind)
	p.history = m.history
	p.search.SetValue(query)
	p.search.CursorEnd()
	return m, m.search(p)
}

func (m Model) Init() tea.Cmd { return tea.EnterAltScreen }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ws.width, m.ws.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case searchDoneMsg:
		return m.handleSearchDone(msg)
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "" // no size yet; anything drawn now is drawn at the wrong one
	}

	base := lipgloss.NewStyle().
		Background(theme.Surface).
		Foreground(theme.Text).
		Width(m.width).
		Height(m.height).
		MaxWidth(m.width).
		MaxHeight(m.height)

	if m.showKeys {
		return base.Render(m.viewKeys())
	}
	if m.ws.empty() {
		return base.Render(m.viewSplash())
	}
	return base.Render(m.viewWorkspace())
}
