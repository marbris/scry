package ui

import (
	"strings"

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
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	sections := m.keySections()

	// The widest key across the whole reference, so descriptions line up
	// down all of it rather than per block.
	keyWidth := 0
	for _, s := range sections {
		for _, row := range s.rows {
			if w := textWidth(row[0]); w > keyWidth {
				keyWidth = w
			}
		}
	}

	// How many columns it takes to fit the height, then how wide each may
	// be — in that order, because the width follows from the count.
	groups := packSections(sections, maxInt(m.height-4, 4))
	gap := 3
	colWidth := (m.width - gap*(len(groups)-1)) / maxInt(len(groups), 1)

	rendered := make([]string, len(groups))
	for i, group := range groups {
		rendered[i] = lipgloss.JoinVertical(lipgloss.Left,
			renderSections(group, keyWidth, colWidth)...)
	}

	body := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top,
		interleave(rendered, strings.Repeat(" ", gap))...), "\n")

	block := lipgloss.JoinVertical(lipgloss.Left,
		append(body, "", dim.Render("any key to close"))...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

// keySection is one titled block of the reference.
type keySection struct {
	title string
	rows  [][2]string
}

// keySections is the reference: the leader's own table, which is the one
// thing the hint bar along the bottom doesn't already carry, and then the
// contextual keymap — the same list the bar shows, laid out to be read
// rather than skimmed.
func (m Model) keySections() []keySection {
	return []keySection{
		{"Panels — space", [][2]string{
			{"space f", "new find panel"},
			{"space d", "new decks panel"},
			{"space r", "new rules panel"},
			{"space n", "new panel, tab to choose"},
			{"space c", "close this panel"},
			{"space o", "close the others"},
			{"space h/l", "move this panel"},
			{"space s", "statistics, everything"},
			{"space 1-9", "go to panel N"},
		}},
		{m.hereTitle(), m.contextKeys()},
	}
}

// height is how many lines a section takes, title included.
func (s keySection) height() int { return len(s.rows) + 1 }

// packSections groups the blocks into as few columns as will fit the height,
// keeping each block whole. A reference that runs off the bottom is missing
// exactly the part you were reaching for.
func packSections(sections []keySection, height int) [][]keySection {
	var out [][]keySection
	var cur []keySection
	used := 0

	for _, s := range sections {
		need := s.height()
		if len(cur) > 0 {
			need++ // the blank line between blocks
		}
		if used+need > height && len(cur) > 0 {
			out = append(out, cur)
			cur, used = []keySection{s}, s.height()
			continue
		}
		cur = append(cur, s)
		used += need
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// renderSections draws one column, every line exactly colWidth wide so the
// column beside it starts where it should.
func renderSections(sections []keySection, keyWidth, colWidth int) []string {
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	key := lipgloss.NewStyle().Foreground(theme.Highlight)
	what := lipgloss.NewStyle().Foreground(theme.Text)

	descWidth := maxInt(colWidth-keyWidth-4, 6)

	var out []string
	for i, s := range sections {
		if i > 0 {
			out = append(out, strings.Repeat(" ", colWidth))
		}
		out = append(out, head.Render(fit(s.title, colWidth)))
		for _, row := range s.rows {
			out = append(out, "  "+
				key.Render(pad(row[0], keyWidth))+"  "+
				what.Render(fit(row[1], descWidth)))
		}
	}
	return out
}

// interleave puts a gap between each pair of columns.
func interleave(parts []string, gap string) []string {
	out := make([]string, 0, maxInt(len(parts)*2-1, 0))
	for i, p := range parts {
		if i > 0 {
			out = append(out, gap)
		}
		out = append(out, p)
	}
	return out
}

// hereTitle names the section describing where you are.
func (m Model) hereTitle() string {
	p := m.ws.current()
	if p == nil {
		return "Here"
	}
	switch v := p.top().(type) {
	case *deckList:
		return "Your decks"
	case *userDeckList:
		return "Somebody's decks"
	case *versionList:
		return "Versions"
	case *rulesView:
		return "The rules"
	case *cardList:
		if v.deck != nil {
			return "This deck"
		}
		return "These cards"
	}
	return "This panel"
}
