package main

// Rules UI — the card-relevant rules panel shown next to search results,
// and the full comprehensive-rules browser (merged in from mtg-rules).

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── List items ──────────────────────────────────────────────────

type ruleItem struct {
	rule  Rule
	index int
}

func (r ruleItem) Title() string       { return r.rule.Number }
func (r ruleItem) Description() string { return r.rule.Text }
func (r ruleItem) FilterValue() string { return r.rule.Number + " " + r.rule.Text }

type glossaryItem struct {
	entry GlossaryEntry
}

func (g glossaryItem) Title() string       { return g.entry.Term }
func (g glossaryItem) Description() string { return truncate(g.entry.Definition, 80) }
func (g glossaryItem) FilterValue() string { return g.entry.Term + " " + g.entry.Definition }

type ruleDelegate struct{}

func (d ruleDelegate) Height() int                             { return 1 }
func (d ruleDelegate) Spacing() int                            { return 0 }
func (d ruleDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d ruleDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	selected := index == m.Index()

	var numStr, textStr string
	maxText := m.Width() - 18
	if maxText < 10 {
		maxText = 10
	}

	switch i := item.(type) {
	case ruleItem:
		numStr = i.rule.Number
		textStr = truncate(i.rule.Text, maxText)
	case glossaryItem:
		numStr = "§"
		textStr = truncate(i.entry.Term, maxText)
	default:
		return
	}

	numStyle := lipgloss.NewStyle().Width(12).Foreground(gruvAqua).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(gruvFg)

	if selected {
		cursor := lipgloss.NewStyle().Foreground(gruvOrange).Render("▸ ")
		line := numStyle.Foreground(gruvOrange).Render(numStr) +
			textStyle.Foreground(gruvYellow).Render(textStr)
		fmt.Fprint(w, cursor+lipgloss.NewStyle().Background(gruvBgLight).Render(line))
		return
	}

	fmt.Fprint(w, "  "+numStyle.Render(numStr)+textStyle.Render(textStr))
}

func buildRuleItems(data RulesData) []list.Item {
	items := make([]list.Item, len(data.Rules))
	for i, r := range data.Rules {
		items[i] = ruleItem{rule: r, index: i}
	}
	return items
}

func buildGlossaryItems(data RulesData) []list.Item {
	items := make([]list.Item, len(data.Glossary))
	for i, g := range data.Glossary {
		items[i] = glossaryItem{entry: g}
	}
	return items
}

// searchRules does an AND-over-terms substring search across rules and
// glossary — the same behaviour the standalone mtg-rules search had.
func searchRules(data RulesData, query string) []list.Item {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return buildRuleItems(data)
	}

	matches := func(haystack string) bool {
		haystack = strings.ToLower(haystack)
		for _, t := range terms {
			if !strings.Contains(haystack, t) {
				return false
			}
		}
		return true
	}

	var results []list.Item
	for i, r := range data.Rules {
		if matches(r.Number + " " + r.Text) {
			results = append(results, ruleItem{rule: r, index: i})
		}
	}
	for _, g := range data.Glossary {
		if matches(g.Term + " " + g.Definition) {
			results = append(results, glossaryItem{entry: g})
		}
	}
	return results
}

// ── Rules browser ───────────────────────────────────────────────

func (m model) updateRulesBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.rulesList.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "?":
			return m.openKeyReference()
		case "esc", "q":
			m.state = m.prevState
			return m, nil
		case "g":
			m.showGlossary = !m.showGlossary
			if m.showGlossary {
				items := buildGlossaryItems(m.rules)
				m.rulesList.SetItems(items)
				m.rulesList.Title = fmt.Sprintf("Glossary (%d)", len(items))
			} else {
				items := buildRuleItems(m.rules)
				m.rulesList.SetItems(items)
				m.rulesList.Title = fmt.Sprintf("Rules (%d)", len(items))
			}
			m.rulesList.ResetSelected()
			m.browseScroll = 0
			return m, nil
		case "J", "shift+down":
			m.browseScroll++
			return m, nil
		case "K", "shift+up":
			if m.browseScroll > 0 {
				m.browseScroll--
			}
			return m, nil
		}
	}

	before := m.rulesList.Index()
	var cmd tea.Cmd
	m.rulesList, cmd = m.rulesList.Update(msg)
	if m.rulesList.Index() != before {
		m.browseScroll = 0
	}
	return m, cmd
}

func (m model) viewRulesBrowse() string {
	if !m.rules.loaded() {
		return lipgloss.NewStyle().Padding(1, 2).Render(m.rulesStatus())
	}

	listW := m.width / 2
	if listW < 20 {
		listW = 20
	}
	m.rulesList.SetSize(listW, m.height)

	previewW := m.width - listW - 4
	if previewW < 30 {
		previewW = 30
	}

	var preview string
	switch item := m.rulesList.SelectedItem().(type) {
	case ruleItem:
		preview = m.renderRuleFull(item.rule, previewW-4)
	case glossaryItem:
		preview = m.renderGlossaryEntry(item.entry, previewW-4)
	default:
		preview = lipgloss.NewStyle().Foreground(gruvGray).Render("No rule selected")
	}

	hint := lipgloss.NewStyle().Foreground(gruvGray).
		Render("/: search  g: glossary  J/K: scroll  esc: back")
	preview = scrollView(preview, m.browseScroll, m.height-3) + "\n" + hint

	previewStyle := lipgloss.NewStyle().
		Width(previewW).
		Height(m.height).
		Padding(1, 2).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(gruvGray)

	listView := lipgloss.NewStyle().MaxWidth(listW).Render(m.rulesList.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, previewStyle.Render(preview))
}

// renderRuleFull shows a rule with its parent context and its sub-rules.
func (m model) renderRuleFull(r Rule, width int) string {
	numStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)
	parentStyle := lipgloss.NewStyle().Foreground(gruvFgDim).Width(width)

	var b strings.Builder

	if r.Parent >= 0 && r.Parent < len(m.rules.Rules) {
		parent := m.rules.Rules[r.Parent]
		b.WriteString(dimStyle.Render("§ "+parent.Number) + "\n")
		b.WriteString(parentStyle.Render(parent.Text) + "\n\n")
	}

	b.WriteString(numStyle.Render(r.Number) + "\n")
	b.WriteString(highlightRuleText(r.Text, width, m.rules) + "\n")

	for _, childIdx := range r.Children {
		if childIdx >= len(m.rules.Rules) {
			continue
		}
		child := m.rules.Rules[childIdx]
		b.WriteString("\n" + numStyle.Render(child.Number) + "\n")
		b.WriteString(highlightRuleText(child.Text, width, m.rules) + "\n")

		for _, gcIdx := range child.Children {
			if gcIdx >= len(m.rules.Rules) {
				continue
			}
			gc := m.rules.Rules[gcIdx]
			b.WriteString("\n" + numStyle.Render("  "+gc.Number) + "\n")
			b.WriteString(indent(highlightRuleText(gc.Text, width-2, m.rules), 2) + "\n")
		}
	}

	return b.String()
}

func (m model) renderGlossaryEntry(g GlossaryEntry, width int) string {
	termStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvPurple)

	var b strings.Builder
	b.WriteString(termStyle.Render(g.Term) + "\n\n")
	b.WriteString(highlightRuleText(g.Definition, width, m.rules) + "\n")

	// Follow the "See rule 702.9" pointer if there is one.
	if ref := seeRuleRe.FindStringSubmatch(g.Definition); ref != nil {
		if idx, ok := m.rules.Index[ref[1]]; ok {
			b.WriteString("\n")
			b.WriteString(m.renderRuleFull(m.rules.Rules[idx], width))
		}
	}
	return b.String()
}

// ── Card-relevant rules panel ───────────────────────────────────

func (m model) rulesStatus() string {
	dim := lipgloss.NewStyle().Foreground(gruvGray)
	if m.rulesErr != nil {
		return lipgloss.NewStyle().Foreground(gruvRed).
			Render(fmt.Sprintf("Rules unavailable: %v", m.rulesErr))
	}
	return dim.Render("Loading comprehensive rules…")
}

// renderCardRules lists the rules that apply to a card's text.
func (m model) renderCardRules(c ScryfallCard, width int) string {
	if width < 20 {
		width = 20
	}
	if !m.rules.loaded() {
		return m.rulesStatus()
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvOrange)
	numStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	termStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvPurple)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)
	labelStyle := lipgloss.NewStyle().Foreground(gruvFgDim)

	matches := m.rules.MatchCard(c)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Rules") + " " +
		dimStyle.Render(fmt.Sprintf("· %s", truncate(c.Name, width-10))) + "\n\n")

	if len(matches) == 0 {
		b.WriteString(dimStyle.Render("Nothing in this card's text maps to a\nkeyword or glossary term."))
		return b.String()
	}

	var types []string

	for _, match := range matches {
		switch match.Kind {
		case matchKeyword:
			if match.Kw.Kind == kwWord {
				b.WriteString(numStyle.Render(match.Rule) + "  " +
					termStyle.Render(match.Term) + "\n")
				b.WriteString(dimStyle.Render("ability word — flavour only, no rules meaning") + "\n\n")
				continue
			}

			kindLabel := "keyword ability"
			if match.Kw.Kind == kwAction {
				kindLabel = "keyword action"
			}
			b.WriteString(numStyle.Render(match.Rule) + "  " +
				lipgloss.NewStyle().Bold(true).Foreground(gruvYellow).Render(match.Term) + "  " +
				labelStyle.Render(kindLabel) + "\n")

			idx, ok := m.rules.Index[match.Rule]
			if !ok {
				b.WriteString("\n")
				continue
			}
			rule := m.rules.Rules[idx]
			for _, childIdx := range rule.Children {
				if childIdx >= len(m.rules.Rules) {
					continue
				}
				child := m.rules.Rules[childIdx]
				body := highlightRuleText(child.Text, width-ruleGutter, m.rules)
				b.WriteString(hangingBlock(child.Number, dimStyle, body, ruleGutter) + "\n")
			}
			b.WriteString("\n")

		case matchGlossary:
			header := termStyle.Render("§ " + match.Term)
			if match.Rule != "" {
				header += "  " + dimStyle.Render("see "+match.Rule)
			}
			b.WriteString(header + "\n")
			b.WriteString(highlightRuleText(match.Entry.Definition, width, m.rules) + "\n\n")

		case matchType:
			types = append(types, fmt.Sprintf("%s %s", match.Rule, match.Term))
		}
	}

	if len(types) > 0 {
		b.WriteString(labelStyle.Render("Types  ") +
			dimStyle.Render(strings.Join(types, " · ")) + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// ── Shared helpers ──────────────────────────────────────────────

// scrollView trims a rendered block to a window of exactly height lines,
// padding short content so anything drawn after it stays put.
func scrollView(s string, offset, height int) string {
	if height < 1 {
		height = 1
	}
	lines := strings.Split(s, "\n")
	if offset > len(lines)-height {
		offset = len(lines) - height
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}

	window := lines[offset:end]
	for len(window) < height {
		window = append(window, "")
	}
	return strings.Join(window, "\n")
}

// ruleGutter is the column the text of a sub-rule starts in, leaving room
// for numbers like "702.10c".
const ruleGutter = 8

// hangingBlock puts a label in a fixed-width gutter with the wrapped body
// lined up underneath it.
func hangingBlock(label string, labelStyle lipgloss.Style, body string, gutter int) string {
	pad := gutter - len(label)
	if pad < 1 {
		pad = 1
	}
	lines := strings.Split(body, "\n")

	var b strings.Builder
	b.WriteString(labelStyle.Render(label) + strings.Repeat(" ", pad) + lines[0])
	for _, l := range lines[1:] {
		b.WriteString("\n" + strings.Repeat(" ", gutter) + l)
	}
	return b.String()
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}
