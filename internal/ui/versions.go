package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/theme"
)

// A deck's versions.
//
// gv means "versions of the thing under the cursor" throughout: here, the
// commits that touched a deck; on a card, the wordings it has been printed
// with. Same question, two kinds of thing — which is why it is one key
// rather than two.

type versionList struct {
	cursor
	slug    string
	name    string
	commits []deck.Commit
}

type versionsMsg struct {
	panel   int
	slug    string
	name    string
	commits []deck.Commit
	err     error
}

// versionLimit is deep enough to find the change you half-remember making,
// and shallow enough that opening it is instant.
const versionLimit = 200

func loadVersions(panelID int, slug, name string) tea.Cmd {
	return func() tea.Msg {
		commits, err := deck.History(slug, versionLimit)
		return versionsMsg{panel: panelID, slug: slug, name: name, commits: commits, err: err}
	}
}

func (m Model) handleVersions(msg versionsMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	p.loading = false
	if msg.err != nil {
		p.err = msg.err
		return m, nil
	}

	v := &versionList{slug: msg.slug, name: msg.name, commits: msg.commits}
	p.push(v)
	p.title = v.title()
	return m, nil
}

func (l *versionList) title() string { return l.name + " · versions" }

func (l *versionList) subtitle() string {
	if len(l.commits) == 0 {
		return ""
	}
	return itoa(len(l.commits)) + " " + plural("version", len(l.commits))
}

func (l *versionList) lines(width, height int, focused bool, m *Model) []string {
	if len(l.commits) == 0 {
		return fillTo([]string{mutedLine("no versions recorded", width)}, width, height)
	}

	l.cursor.scrollInto(height, len(l.commits))
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.commits) && len(lines) < height; i++ {
		c := l.commits[i]
		under := focused && i == l.cursor.at

		when := c.When
		subject := fit(c.Subject, maxInt(width-textWidth(when)-1, 0))

		style := lipgloss.NewStyle().Foreground(theme.Text)
		if under {
			style = style.Foreground(theme.SelectionFg).Bold(true)
		}
		line := style.Render(subject) + " " +
			lipgloss.NewStyle().Foreground(theme.TextDim).Render(when)
		if under {
			line = lipgloss.NewStyle().Background(theme.SelectionBg).Width(width).Render(line)
		}
		lines = append(lines, line)
	}
	return fillTo(lines, width, height)
}

func (l *versionList) clear() bool { return false }

func (l *versionList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if l.cursor.navKey(k, len(l.commits), m.pageStep()) {
		return true, nil
	}
	return false, nil
}
