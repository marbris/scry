package ui

import (
	"github.com/charmbracelet/lipgloss"

	"scry/internal/theme"
)

// The splash, and the key reference.
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

// viewKeys is the full reference. It's scoped: the keys of the panel you're
// in, not a wall of everything the program can do — which is the difference
// between a reference you consult and one you close again.
func (m Model) viewKeys() string {
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	key := lipgloss.NewStyle().Foreground(theme.Highlight)
	what := lipgloss.NewStyle().Foreground(theme.Text)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	section := func(title string, rows [][2]string) []string {
		out := []string{head.Render(title)}
		for _, r := range rows {
			out = append(out, "  "+key.Render(pad(r[0], 12))+" "+what.Render(r[1]))
		}
		return append(out, "")
	}

	var lines []string
	lines = append(lines, section("Panels — space", [][2]string{
		{"space f", "new find panel"},
		{"space d", "new decks panel"},
		{"space r", "new rules panel"},
		{"space n", "new panel, tab to choose"},
		{"space c", "close this panel"},
		{"space o", "close every other panel"},
		{"space h/l", "move this panel along the row"},
		{"space 1-9", "go to panel N"},
	})...)
	lines = append(lines, section("Moving", [][2]string{
		{"h l", "previous / next panel"},
		{"j k", "previous / next row"},
		{"K J", "move in the information panel"},
		{"ctrl+k/j", "scroll the information panel"},
	})...)
	lines = append(lines, section("Here", [][2]string{
		{"i", "search bar"},
		{"tab", "change what the bar searches"},
		{"s", "statistics"},
		{"e", "pin as the editing deck"},
		{"esc", "clear, then close"},
		{"q", "quit"},
	})...)
	lines = append(lines, dim.Render("any key to close"))

	block := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}
