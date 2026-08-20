package ui

import (
	"github.com/charmbracelet/lipgloss"

	"scry/internal/theme"
)

// The splash.
//
// The splash is what `scry` opens to with nothing restored: no panels, and
// therefore nothing to look at but the way in. It says the three keys that
// open something and gets out of the way.

func (m Model) viewSplash() string {
	name := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("scry")
	tag := lipgloss.NewStyle().Foreground(theme.TextDim).
		Render("Magic: The Gathering — cards, decks and rules")

	key := lipgloss.NewStyle().Foreground(theme.Highlight).Bold(true)
	what := lipgloss.NewStyle().Foreground(theme.Text)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	rows := []struct{ k, v string }{
		{"space f", "find cards on Scryfall"},
		{"space d", "your decks, and Moxfield"},
		{"space r", "the comprehensive rules"},
	}

	var lines []string
	lines = append(lines, name, tag, "")
	for _, r := range rows {
		lines = append(lines, key.Render(pad(r.k, 9))+" "+what.Render(r.v))
	}
	lines = append(lines, "", dim.Render("space for the menu · ? for the keys · q to quit"))

	block := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}
