package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/theme"
)

// Drawing the workspace: the row of panels, the information panel beside
// them, and the line along the bottom that says what the keys do here.

// viewWorkspace lays the whole frame out.
func (m Model) viewWorkspace() string {
	ws := m.ws // a copy: layout records the scroll position, and View is a
	l := ws.layout()

	var columns []string
	for i, width := range l.panels {
		at := l.first + i
		columns = append(columns, m.viewPanel(ws.panels[at], at, width, l.height))
	}
	if l.info > 0 {
		columns = append(columns, m.viewInfo(l.info, l.height))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	return lipgloss.JoinVertical(lipgloss.Left, row, m.viewFooter(l))
}

// viewPanel draws one panel: a border, its header, and its contents.
func (m Model) viewPanel(p *panel, index, width, height int) string {
	focused := index == m.ws.focused
	editing := index == m.ws.editing

	// The border says which panel has the keys, and which one a/x/t are
	// going to write to — the two things you need to know without looking.
	colour := theme.Border
	switch {
	case focused:
		colour = theme.BorderFocus
	case editing:
		colour = theme.BorderEditing
	}

	inner := maxInt(width-2, 1)
	p.restyle()

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	if !focused {
		head = lipgloss.NewStyle().Foreground(theme.TextDim)
	}

	lines := []string{head.Render(fit(p.header(inner), inner))}
	lines = append(lines, lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", inner)))
	lines = append(lines, m.viewPanelBody(p, inner, height-4)...)

	body := lipgloss.NewStyle().
		Width(inner).
		Height(maxInt(height-2, 1)).
		MaxWidth(inner).
		Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colour).
		Render(body)
}

// viewPanelBody is what a panel holds. Every kind is empty until the phases
// that fill them, so for now it says what it's waiting for.
func (m Model) viewPanelBody(p *panel, width, height int) []string {
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var lines []string
	if p.empty() {
		lines = append(lines,
			dim.Render(fit("nothing here yet", width)),
			"",
			dim.Render(fit("type a "+p.kind.prompt(), width)),
		)
	} else {
		lines = append(lines, dim.Render(fit("— results land here —", width)))
	}

	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// viewInfo draws the information panel.
func (m Model) viewInfo(width, height int) string {
	inner := maxInt(width-2, 1)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fit(m.info.mode.String(), inner))

	lines := []string{
		title,
		lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", inner)),
	}
	if p := m.ws.panels[m.ws.focused]; p != nil {
		lines = append(lines, dim.Render(fit("focused: "+p.kind.String(), inner)))
	}
	lines = append(lines, "", dim.Render(fit("K/J move · ctrl+j/k scroll", inner)))

	for len(lines) < height-2 {
		lines = append(lines, strings.Repeat(" ", inner))
	}

	body := lipgloss.NewStyle().Width(inner).Height(maxInt(height-2, 1)).
		MaxWidth(inner).Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Render(body)
}

// ── The bottom line ─────────────────────────────────────────────

// viewFooter is the leader menu while the leader is waiting, and otherwise
// the keys that apply where you are.
func (m Model) viewFooter(l layout) string {
	if m.leader {
		return m.viewLeaderBar()
	}
	return m.viewHint(l)
}

// viewLeaderBar is the menu the leader raises, so it never has to be
// memorised. Being able to see the menu is what makes a two-key binding
// cheaper in practice than a one-key chord you can't remember.
func (m Model) viewLeaderBar() string {
	key := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	what := lipgloss.NewStyle().Foreground(theme.Text)
	sep := lipgloss.NewStyle().Foreground(theme.TextMuted).Render(" · ")

	var parts []string
	for _, c := range leaderMenu {
		parts = append(parts, key.Render(c.key)+" "+what.Render(c.what))
	}
	if m.ws.count() > 1 {
		parts = append(parts, key.Render("1-9")+" "+what.Render("go to"))
	}

	return lipgloss.NewStyle().
		Background(theme.SurfaceAlt).
		Width(m.width).
		MaxWidth(m.width).
		Render(" " + truncate(strings.Join(parts, sep), maxInt(m.width-2, 1)))
}

// viewHint is one line describing where you are and what works here.
func (m Model) viewHint(l layout) string {
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)
	accent := lipgloss.NewStyle().Foreground(theme.Accent)

	var left string
	if p := m.ws.current(); p != nil {
		if p.searchOpen && p.search.Focused() {
			left = "tab target · enter run · esc back"
		} else {
			left = "h/l panel · i search · s stats · space menu · ? keys"
		}
	}

	// Which panel of how many, and whether any are off screen.
	right := ""
	if n := m.ws.count(); n > 0 {
		right = itoa(m.ws.focused+1) + "/" + itoa(n)
		if l.visible() < n {
			right += " ↔"
		}
	}

	gap := maxInt(m.width-runeLen(left)-runeLen(right)-2, 1)
	return " " + dim.Render(left) + strings.Repeat(" ", gap) + accent.Render(right) + " "
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
