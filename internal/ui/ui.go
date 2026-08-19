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

	width, height int
}

func New() Model {
	return Model{ws: newWorkspace()}
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
