package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The statistics panel is a list you can walk with J/K. Every row carries
// the test it counted itself with, so filtering the cards to a category and
// the number printed beside that category can never disagree.

type statRow struct {
	group string // "Color", "Rarity", …
	label string // "Blue", "rare", "3", "Creature", "Ramp"
	count int    // over the cards on screen — what the bar shows
	base  int    // over the whole result set — whether the row exists at all
	color lipgloss.Color
	match func(cardItem) bool
}

type statGroup struct {
	title string
	rows  []statRow
}

// same reports whether two rows name the same category. The rows carry
// closures, so they can't be compared directly.
func (r statRow) same(other *statRow) bool {
	return other != nil && r.group == other.group && r.label == other.label
}

// ── Building the rows ───────────────────────────────────────────

// colorRows is the colour spread of the spells. Lands are left out for the
// same reason they're left out of the curve: nearly all of them are
// colourless by the card's own reckoning, so counting them says how many
// lands the deck runs — under a "Colorless" heading, where it reads as if
// the deck were full of colourless spells.
func colorRows() []statRow {
	spec := []struct {
		label string
		code  string
		color lipgloss.Color
	}{
		{"White", "W", lipgloss.Color("#fbf1c7")},
		{"Blue", "U", gruvBlue},
		{"Black", "B", gruvGray},
		{"Red", "R", gruvRed},
		{"Green", "G", gruvGreen},
	}

	rows := make([]statRow, 0, len(spec)+2)
	for _, s := range spec {
		code := s.code
		rows = append(rows, statRow{
			group: "Color", label: s.label, color: s.color,
			match: func(ci cardItem) bool {
				if isLand(ci.card) {
					return false
				}
				for _, c := range ci.card.displayColors() {
					if c == code {
						return true
					}
				}
				return false
			},
		})
	}
	rows = append(rows,
		statRow{
			group: "Color", label: "Colorless", color: gruvFgDim,
			match: func(ci cardItem) bool {
				return !isLand(ci.card) && len(ci.card.displayColors()) == 0
			},
		},
		statRow{
			group: "Color", label: "Multi", color: gruvYellow,
			match: func(ci cardItem) bool {
				return !isLand(ci.card) && len(ci.card.displayColors()) > 1
			},
		},
	)
	return rows
}

func rarityRows(entries []cardItem) []statRow {
	color := map[string]lipgloss.Color{
		"common": gruvFg, "uncommon": gruvFgDim, "rare": gruvYellow,
		"mythic": gruvOrange, "special": gruvPurple,
	}

	// The usual rarities in their usual order, then anything unexpected.
	order := []string{"common", "uncommon", "rare", "mythic", "special", "bonus"}
	known := map[string]bool{}
	for _, r := range order {
		known[r] = true
	}
	var extra []string
	for _, e := range entries {
		r := e.card.Rarity
		if r == "" {
			r = "unknown"
		}
		if !known[r] {
			known[r] = true
			extra = append(extra, r)
		}
	}
	sort.Strings(extra)

	rows := make([]statRow, 0, len(order)+len(extra))
	for _, r := range append(order, extra...) {
		rarity := r
		col, ok := color[rarity]
		if !ok {
			col = gruvGray
		}
		rows = append(rows, statRow{
			group: "Rarity", label: rarity, color: col,
			match: func(ci cardItem) bool {
				got := ci.card.Rarity
				if got == "" {
					got = "unknown"
				}
				return got == rarity
			},
		})
	}
	return rows
}

// cmcRows is the mana curve. Lands are left out of it: they nearly all cost
// nothing, so counting them buries the curve under a column at zero that
// says only how many lands the deck runs — which the type breakdown below
// already says, and better.
func cmcRows() []statRow {
	rows := make([]statRow, 0, 8)
	for i := 0; i <= 7; i++ {
		n := i
		label := strconv.Itoa(n)
		if n == 7 {
			label = "7+"
		}
		rows = append(rows, statRow{
			group: "Mana Value", label: label, color: gruvAqua,
			match: func(ci cardItem) bool {
				if isLand(ci.card) {
					return false
				}
				cmc := int(ci.card.CMC)
				if n == 7 {
					return cmc >= 7
				}
				return cmc == n
			},
		})
	}
	return rows
}

// isLand reports whether a card's front face is a land, which is what
// decides where it's counted.
func isLand(c ScryfallCard) bool { return primaryType(c.TypeLine) == "Land" }

func typeRows() []statRow {
	types := []string{"Creature", "Instant", "Sorcery", "Artifact", "Enchantment", "Planeswalker", "Land"}
	rows := make([]statRow, 0, len(types))
	for _, t := range types {
		cardType := t
		rows = append(rows, statRow{
			group: "Type", label: cardType, color: gruvPurple,
			match: func(ci cardItem) bool {
				return strings.Contains(ci.card.TypeLine, cardType)
			},
		})
	}
	return rows
}

// tagRows come from the deck itself — only a Moxfield deck whose author
// tagged their cards has any.
func tagRows(entries []cardItem) []statRow {
	seen := map[string]bool{}
	var labels []string
	for _, e := range entries {
		for _, t := range e.tags {
			if !seen[t] {
				seen[t] = true
				labels = append(labels, t)
			}
		}
	}
	sort.Strings(labels)

	rows := make([]statRow, 0, len(labels))
	for _, l := range labels {
		tag := l
		rows = append(rows, statRow{
			group: "Tags", label: tag, color: gruvYellow,
			match: func(ci cardItem) bool {
				for _, t := range ci.tags {
					if t == tag {
						return true
					}
				}
				return false
			},
		})
	}
	return rows
}

// statGroups builds the category list from rowSource and counts it over
// counted. The two differ once you're filtering by a category: which rows
// exist, their order and their positions all come from the whole result set
// and so hold still, while the numbers beside them describe just the cards
// on screen — a category with nothing left in it stays put and reads zero.
func statGroups(rowSource, counted []cardItem) []statGroup {
	// Most telling first: what the deck's author called their cards, then
	// what those cards are, and the printing details last.
	groups := []statGroup{
		{title: "Tags", rows: tagRows(rowSource)},
		{title: "Type", rows: typeRows()},
		{title: "Color (excl. lands)", rows: colorRows()},
		{title: "Mana Value (excl. lands)", rows: cmcRows()},
		{title: "Rarity", rows: rarityRows(rowSource)},
	}

	out := make([]statGroup, 0, len(groups))
	for _, g := range groups {
		kept := make([]statRow, 0, len(g.rows))
		for _, r := range g.rows {
			for _, e := range rowSource {
				if r.match(e) {
					r.base += e.qty
				}
			}
			// A category nothing in the deck has ever matched is left out
			// entirely; one the current filter has emptied is not.
			if r.base == 0 {
				continue
			}
			for _, e := range counted {
				if r.match(e) {
					r.count += e.qty
				}
			}
			kept = append(kept, r)
		}
		if len(kept) == 0 {
			continue
		}
		// Tags have no natural order, so the commonest lead — by their
		// standing in the whole set, so walking the list can't reorder it.
		if g.title == "Tags" {
			sort.SliceStable(kept, func(i, j int) bool { return kept[i].base > kept[j].base })
		}
		out = append(out, statGroup{title: g.title, rows: kept})
	}
	return out
}

// statPanel is the category list exactly as the panel draws it.
func (m model) statPanel() []statGroup {
	counted := m.getVisibleEntries()

	// With a category selected the list on screen is already narrowed to
	// it, so the rows have to come from the full set to stay still.
	rowSource := counted
	if m.active().statFilter != nil {
		rowSource = toEntries(m.active().baseItems)
	}
	return statGroups(rowSource, counted)
}

// rowIndex finds where a category sits in the list, or -1.
func rowIndex(rows []statRow, want *statRow) int {
	for i, r := range rows {
		if r.same(want) {
			return i
		}
	}
	return -1
}

func flatRows(groups []statGroup) []statRow {
	var out []statRow
	for _, g := range groups {
		out = append(out, g.rows...)
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
		if index < len(g.rows) {
			return line + index
		}
		index -= len(g.rows)
		line += len(g.rows) + 1 // its rows, then the gap to the next group
	}
	return line
}

// ── The panel ───────────────────────────────────────────────────

func (m model) renderStats(maxW int) string {
	groups := m.statPanel()

	total := 0
	for _, e := range m.getVisibleEntries() {
		total += e.qty
	}

	if maxW < 30 {
		maxW = 30
	}

	// One label width across every group, so the bars line up down the panel.
	labelW := 0
	for _, r := range flatRows(groups) {
		if len(r.label) > labelW {
			labelW = len(r.label)
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
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(gruvAqua).Render(g.title) + "\n")

		// Each group scales to its largest bar in the unfiltered set, so a
		// filtered category's bars shrink to show how much of the whole
		// they are. Scaling to the filtered maximum instead would refill
		// the panel at every step and make every selection look alike.
		maxBase := 0
		for _, r := range g.rows {
			if r.base > maxBase {
				maxBase = r.base
			}
		}
		countW := len(strconv.Itoa(maxBase))
		barMaxW := maxW - labelW - countW - 5
		if barMaxW < 5 {
			barMaxW = 5
		}

		for _, r := range g.rows {
			// A category the filter has emptied keeps its place, with no
			// bar and its label dimmed, so the list holds still.
			barLen := 0
			if r.count > 0 && maxBase > 0 {
				barLen = (r.count * barMaxW) / maxBase
				if barLen < 1 {
					barLen = 1
				}
			}

			labelColor, countColor := r.color, gruvFgDim
			if r.count == 0 {
				labelColor, countColor = gruvGray, gruvGray
			}

			label := lipgloss.NewStyle().Width(labelW).Foreground(labelColor).Render(r.label)
			bar := lipgloss.NewStyle().Foreground(r.color).Render(strings.Repeat("█", barLen))
			count := lipgloss.NewStyle().Foreground(countColor).Render(fmt.Sprintf(" %d", r.count))

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
		if ci.qty < 1 {
			ci.qty = 1
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
	return m.active().statFilter.group + ": " + m.active().statFilter.label
}
