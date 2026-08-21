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
// statsState is deliberately only the two facts a frame can't work out for
// itself. The bars themselves are derived when they are drawn, never stored:
// they are a function of the cards, and a stored copy goes stale the moment
// a search finishes — which is exactly what it did, leaving "nothing to
// count" standing over a panel full of cards.
type statsState struct {
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

// statGroups counts the cards. The source decides which categories exist and
// in what order — the whole list — while the counted set is what the numbers
// describe. The two differ once a category is chosen: the rows hold still
// while the numbers beside them describe what's left, so a category with
// nothing in it stays put and reads zero rather than vanishing under the
// cursor.
func (m Model) statGroups() []stats.Group {
	counted, source := m.statCards()
	return stats.Groups(source, counted)
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
	rows := statRows(m.statGroups())
	var chosen *stats.Row
	if m.stats.row >= 0 && m.stats.row < len(rows) {
		chosen = &rows[m.stats.row]
	}
	for _, l := range m.statLists() {
		l.statFilter = chosen
		l.refresh()
	}
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

// clearActiveFilters drops the narrowings on the focused list alone: its text
// filter and the statistics category. b. The statistics category is a single
// selection shown in the bars, so clearing it steps the highlight back to no
// category rather than leaving a bar lit over an unfiltered list.
func (m *Model) clearActiveFilters() {
	p := m.ws.current()
	if p == nil {
		return
	}
	if m.stats.row >= 0 {
		m.stats.row = -1
		m.applyStatFilter()
	}
	if l := p.cardsView(); l != nil && l.statFilter != nil {
		l.statFilter = nil
		l.refresh()
	}
	if v, ok := p.top().(filterable); ok {
		v.setFilter("")
	}
}

// clearAllFilters drops every narrowing on every list — text filters and the
// statistics category alike. space b, for when you've narrowed several panels
// and want them all back at once.
func (m *Model) clearAllFilters() {
	m.stats.row = -1
	m.clearStatFilter()
	for _, p := range m.ws.panels {
		if v, ok := p.top().(filterable); ok {
			v.setFilter("")
		}
	}
}

// moveStat walks the categories. Stepping off the top goes back to no
// category at all, which is how you get the whole list back without
// remembering which key clears it.
func (m *Model) moveStat(delta int) {
	rows := statRows(m.statGroups())
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

// jumpStat moves to the first category of the next group, or the previous
// one. Tags, then types, then colours, then the curve: with five groups of a
// dozen rows, walking row by row to reach the curve is a lot of J.
func (m *Model) jumpStat(delta int) {
	groups := m.statGroups()
	starts := groupStarts(groups)
	if len(starts) == 0 {
		return
	}

	switch {
	case delta > 0:
		for _, at := range starts {
			if at > m.stats.row {
				m.stats.row = at
				m.applyStatFilter()
				return
			}
		}
		// Past the last group: on to the last row, so J always moves.
		m.stats.row = len(statRows(groups)) - 1
	default:
		for i := len(starts) - 1; i >= 0; i-- {
			if starts[i] < m.stats.row {
				m.stats.row = starts[i]
				m.applyStatFilter()
				return
			}
		}
		// Past the first group: back to no category, which is the whole
		// list — the same place stepping off the top lands.
		m.stats.row = -1
	}
	m.applyStatFilter()
}

// groupStarts is the flattened index of each group's first row.
func groupStarts(groups []stats.Group) []int {
	var out []int
	at := 0
	for _, g := range groups {
		if len(g.Rows) == 0 {
			continue
		}
		out = append(out, at)
		at += len(g.Rows)
	}
	return out
}

// ── Drawing ─────────────────────────────────────────────────────

// statLine is which rendered line a category sits on, counting the group
// headings and the blank lines between them — which is what the panel has to
// scroll by.
func statLine(groups []stats.Group, row int) int {
	if row < 0 {
		return 0
	}
	line, at := 0, 0
	for _, g := range groups {
		if len(g.Rows) == 0 {
			continue
		}
		if line > 0 {
			line++ // the blank line between groups
		}
		line++ // the heading
		for range g.Rows {
			if at == row {
				return line
			}
			at++
			line++
		}
	}
	return line
}

// renderStats draws the groups as horizontal bars.
func (m Model) renderStats(width int) []string {
	groups := m.statGroups()
	if len(groups) == 0 {
		return []string{mutedLine("nothing to count", width)}
	}

	// One scale across every group, so a glance compares them.
	widest := 0
	for _, g := range groups {
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
	for _, g := range groups {
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
	for _, g := range groups {
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

// statOffset is how far the panel is scrolled for the highlighted category,
// which is the same arithmetic the render does. Exposed so a test can ask
// without drawing.
func (m Model) statOffset(height int) int {
	groups := m.statGroups()
	return scrollTo(statLine(groups, m.stats.row), 0,
		maxInt(height, 1), len(m.renderStats(30)))
}
