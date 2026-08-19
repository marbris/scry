package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/stats"
	"scry/internal/theme"
)

// Statistics, in the information panel.
//
// Not a panel of its own: it is a way of reading a list, and it needs the
// list beside it to be any use. K and J walk the categories and the list
// narrows to whichever one you are on, so the bars and the cards can never
// disagree — the bar you are standing on is exactly the cards in front of
// you.
//
// The bars all share one scale, so a glance compares them. Scaling each
// group to its own widest bar would make a deck with two of something look
// like a deck full of it.

// statsState is what the information panel is showing while in statistics
// mode, and what the list is narrowed to because of it.
type statsState struct {
	groups []stats.Group
	// row is the flattened index of the highlighted category, or -1 when
	// the panel is showing totals and the list is unnarrowed.
	row int
	// global is <space>s: every list on screen at once, rather than one.
	global bool
}

// statRows flattens the groups into the rows K and J step through.
func statRows(groups []stats.Group) []stats.Row {
	var out []stats.Row
	for _, g := range groups {
		out = append(out, g.Rows...)
	}
	return out
}

// build counts the cards. rowSource decides which categories exist and in
// what order — the whole list — while counted is what the numbers describe.
// The two differ once a category is chosen: the rows hold still while the
// numbers beside them describe what's left, so a category with nothing in it
// stays put and reads zero rather than vanishing under the cursor.
func (m *Model) buildStats() {
	cards, source := m.statCards()
	m.stats.groups = stats.Groups(source, cards)
	if m.stats.row >= len(statRows(m.stats.groups)) {
		m.stats.row = len(statRows(m.stats.groups)) - 1
	}
}

// statCards is what the statistics describe: the focused list, or every list
// on screen when <space>s asked for all of them.
func (m *Model) statCards() (counted, source []deck.Card) {
	lists := m.statLists()
	for _, l := range lists {
		source = append(source, l.all...)
		counted = append(counted, l.narrowed()...)
	}
	return counted, source
}

func (m *Model) statLists() []*cardList {
	if !m.stats.global {
		p := m.ws.current()
		if p == nil {
			return nil
		}
		if l := p.cardsView(); l != nil {
			return []*cardList{l}
		}
		return nil
	}

	var out []*cardList
	for _, p := range m.ws.panels {
		if l := p.cardsView(); l != nil {
			out = append(out, l)
		}
	}
	return out
}

// applyStatFilter narrows every list the statistics are counting to the
// highlighted category — which is what makes walking the bars a way of
// reading the deck rather than a report about it.
func (m *Model) applyStatFilter() {
	rows := statRows(m.stats.groups)
	var chosen *stats.Row
	if m.stats.row >= 0 && m.stats.row < len(rows) {
		chosen = &rows[m.stats.row]
	}
	for _, l := range m.statLists() {
		l.statFilter = chosen
		l.refresh()
	}
	m.buildStats()
}

// clearStatFilter puts every list back.
func (m *Model) clearStatFilter() {
	for _, p := range m.ws.panels {
		if l := p.cardsView(); l != nil && l.statFilter != nil {
			l.statFilter = nil
			l.refresh()
		}
	}
}

// moveStat walks the categories. Stepping off the top goes back to no
// category at all, which is how you get the whole list back without
// remembering which key clears it.
func (m *Model) moveStat(delta int) {
	rows := statRows(m.stats.groups)
	if len(rows) == 0 {
		return
	}
	m.stats.row += delta
	if m.stats.row < -1 {
		m.stats.row = -1
	}
	if m.stats.row >= len(rows) {
		m.stats.row = len(rows) - 1
	}
	m.applyStatFilter()
}

// ── Drawing ─────────────────────────────────────────────────────

// renderStats draws the groups as horizontal bars.
func (m Model) renderStats(width int) []string {
	if len(m.stats.groups) == 0 {
		return []string{mutedLine("nothing to count", width)}
	}

	// One scale across every group, so a glance compares them.
	widest := 0
	for _, g := range m.stats.groups {
		for _, r := range g.Rows {
			if r.Base > widest {
				widest = r.Base
			}
		}
	}
	if widest == 0 {
		widest = 1
	}

	labelWidth := 0
	for _, g := range m.stats.groups {
		for _, r := range g.Rows {
			if w := textWidth(r.Label); w > labelWidth {
				labelWidth = w
			}
		}
	}
	labelWidth = minInt(labelWidth, maxInt(width/2, 6))

	countWidth := textWidth(itoa(widest))
	barWidth := maxInt(width-labelWidth-countWidth-2, 1)

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var out []string
	at := 0
	for _, g := range m.stats.groups {
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, head.Render(fit(g.Title, width)))
		for _, r := range g.Rows {
			out = append(out, statBar(r, at == m.stats.row, labelWidth, barWidth, countWidth, widest))
			at++
		}
	}

	out = append(out, "", dim.Render(fit("K/J to narrow · esc to clear", width)))
	return out
}

func statBar(r stats.Row, under bool, labelWidth, barWidth, countWidth, scale int) string {
	filled := r.Count * barWidth / scale
	if r.Count > 0 && filled == 0 {
		filled = 1 // one card should never read as none
	}

	bar := lipgloss.NewStyle().Foreground(r.Color).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(theme.BarEmpty).Render(strings.Repeat("─", maxInt(barWidth-filled, 0)))

	labelStyle := lipgloss.NewStyle().Foreground(theme.Text)
	if under {
		labelStyle = labelStyle.Foreground(theme.SelectionFg).Bold(true)
	}

	line := labelStyle.Render(fit(r.Label, labelWidth)) + " " + bar + " " +
		lipgloss.NewStyle().Foreground(theme.TextDim).
			Render(pad(itoa(r.Count), countWidth))

	if under {
		return lipgloss.NewStyle().Background(theme.SelectionBg).Render(line)
	}
	return line
}
