package main

import (
	"scry/internal/deck"

	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// statPanel is the category list exactly as the panel draws it.
// deckCards converts a list's rows for the statistics package, which counts
// deck cards rather than list items.
func deckCards(items []cardItem) []deck.Card {
	out := make([]deck.Card, len(items))
	for i, it := range items {
		out[i] = it.deckCard()
	}
	return out
}

func (m model) statPanel() []statGroup {
	counted := m.getVisibleEntries()

	// With a category selected the list on screen is already narrowed to
	// it, so the rows have to come from the full set to stay still.
	rowSource := counted
	if m.active().statFilter != nil {
		rowSource = toEntries(m.active().baseItems)
	}
	return statGroups(deckCards(rowSource), deckCards(counted))
}

// rowIndex finds where a category sits in the list, or -1.
func rowIndex(rows []statRow, want *statRow) int {
	for i, r := range rows {
		if r.Same(want) {
			return i
		}
	}
	return -1
}

func flatRows(groups []statGroup) []statRow {
	var out []statRow
	for _, g := range groups {
		out = append(out, g.Rows...)
	}
	return out
}

// statLine is the line the nth row is rendered on, counting from the top of
// the panel. renderStats lays the panel out the same way; the two are kept
// honest by TestStatLineMatchesRender.
func statLine(groups []statGroup, index int) int {
	line := 2 // the "Statistics (n cards)" heading and the blank under it
	for _, g := range groups {
		line++ // the group's own heading
		if index < len(g.Rows) {
			return line + index
		}
		index -= len(g.Rows)
		line += len(g.Rows) + 1 // its rows, then the gap to the next group
	}
	return line
}

// ── The panel ───────────────────────────────────────────────────

func (m model) renderStats(maxW int) string {
	groups := m.statPanel()

	total := 0
	for _, e := range m.getVisibleEntries() {
		total += e.Qty
	}

	if maxW < 30 {
		maxW = 30
	}

	// One label width across every group, so the bars line up down the panel.
	labelW := 0
	for _, r := range flatRows(groups) {
		if len(r.Label) > labelW {
			labelW = len(r.Label)
		}
	}

	title := fmt.Sprintf(" Statistics (%d cards)", total)
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(gruvOrange).Render(title))
	if label := m.statFilterLabel(); label != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(gruvGray).Render(" · " + label))
	}
	b.WriteString("\n\n")

	if len(groups) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(gruvGray).Render("  (nothing to count)") + "\n")
		return b.String()
	}

	index := 0
	for gi, g := range groups {
		if gi > 0 {
			b.WriteString("\n")
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(gruvAqua).Render(g.Title) + "\n")

		// Each group scales to its largest bar in the unfiltered set, so a
		// filtered category's bars shrink to show how much of the whole
		// they are. Scaling to the filtered maximum instead would refill
		// the panel at every step and make every selection look alike.
		maxBase := 0
		for _, r := range g.Rows {
			if r.Base > maxBase {
				maxBase = r.Base
			}
		}
		countW := len(strconv.Itoa(maxBase))
		barMaxW := maxW - labelW - countW - 5
		if barMaxW < 5 {
			barMaxW = 5
		}

		for _, r := range g.Rows {
			// A category the filter has emptied keeps its place, with no
			// bar and its label dimmed, so the list holds still.
			barLen := 0
			if r.Count > 0 && maxBase > 0 {
				barLen = (r.Count * barMaxW) / maxBase
				if barLen < 1 {
					barLen = 1
				}
			}

			labelColor, countColor := r.Color, gruvFgDim
			if r.Count == 0 {
				labelColor, countColor = gruvGray, gruvGray
			}

			label := lipgloss.NewStyle().Width(labelW).Foreground(labelColor).Render(r.Label)
			bar := lipgloss.NewStyle().Foreground(r.Color).Render(strings.Repeat("█", barLen))
			count := lipgloss.NewStyle().Foreground(countColor).Render(fmt.Sprintf(" %d", r.Count))

			row := fmt.Sprintf("%s %s%s", label, bar, count)
			if index == m.active().statIndex {
				row = lipgloss.NewStyle().Foreground(gruvOrange).Render("▸ ") +
					lipgloss.NewStyle().Background(gruvBgLight).Render(row)
			} else {
				row = "  " + row
			}
			b.WriteString(row + "\n")
			index++
		}
	}

	return b.String()
}

// ── Filtering by category ───────────────────────────────────────

// getVisibleEntries is the cards on screen with their deck quantities, so a
// deck running 30 Mountains counts as 30.
func (m model) getVisibleEntries() []cardItem {
	return toEntries(m.active().list.VisibleItems())
}

func toEntries(items []list.Item) []cardItem {
	entries := make([]cardItem, 0, len(items))
	for _, item := range items {
		ci, ok := item.(cardItem)
		if !ok {
			continue
		}
		if ci.Qty < 1 {
			ci.Qty = 1
		}
		entries = append(entries, ci)
	}
	return entries
}

// statMove walks the category list, narrowing the cards to whichever
// category the cursor lands on.
func (m model) statMove(delta int) (tea.Model, tea.Cmd) {
	rows := flatRows(m.statPanel())
	if len(rows) == 0 {
		return m, nil
	}

	index := m.active().statIndex + delta
	if m.active().statIndex < 0 {
		// The first press enters the list rather than stepping through it.
		index = 0
		if delta < 0 {
			index = len(rows) - 1
		}
	}
	if index < 0 {
		index = 0
	}
	if index >= len(rows) {
		index = len(rows) - 1
	}

	m.active().statIndex = index
	row := rows[index]
	m.active().statFilter = &row

	// Scroll last: changing the list moves the cursor onto a different
	// card, and syncHover resets the panel's scroll when it does.
	next, cmd := m.applyStatFilter()
	scrolled := next.(model)

	// Selecting the first category swaps the row list over to the full
	// result set, which can shift positions if a typed filter was in play.
	// Follow the category rather than the index.
	groups := scrolled.statPanel()
	if i := rowIndex(flatRows(groups), scrolled.active().statFilter); i >= 0 {
		scrolled.active().statIndex = i
	}
	scrolled.scrollStatIntoView(statLine(groups, scrolled.active().statIndex))
	return scrolled, cmd
}

// scrollStatIntoView keeps the selected category on screen as the cursor
// runs past the bottom of the panel.
func (m *model) scrollStatIntoView(line int) {
	height := m.resultsLayout().panelH - 1
	if height < 1 {
		return
	}
	if line < m.previewScroll {
		m.previewScroll = line
	}
	if line >= m.previewScroll+height {
		m.previewScroll = line - height + 1
	}
}

// clearStatFilter puts every card back in the list.
func (m model) clearStatFilter() (tea.Model, tea.Cmd) {
	m.active().statFilter = nil
	m.active().statIndex = -1
	return m.applyStatFilter()
}

// applyStatFilter rebuilds the list from the full result set, keeping only
// the selected category's cards if one is selected.
func (m model) applyStatFilter() (tea.Model, tea.Cmd) {
	p := m.active()
	p.refresh()
	p.list.ResetSelected()
	next, cmd := m.syncHover()
	return next, cmd
}

// statFilterLabel names the active category for the header.
func (m model) statFilterLabel() string {
	if m.active().statFilter == nil {
		return ""
	}
	return m.active().statFilter.Group + ": " + m.active().statFilter.Label
}
