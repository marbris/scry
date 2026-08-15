package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateResults: the main screen — search bar, card list and the panel
// beside it. Geometry lives in resultsLayout() so that Update and View
// always agree on where things are.

// ── Results screen ──────────────────────────────────────────────

// updateResults drives the only main screen. Keys go to the search bar or
// to the card list depending on where focus is.
func (m model) updateResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+r" {
			return m.openRulesBrowser(nil, "")
		}
		if m.searchFocused {
			return m.updateSearchBar(msg)
		}
		return m.updateList(msg)

	case searchResultMsg:
		m.searching = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if len(msg.cards) == 0 {
			m.err = fmt.Errorf("no results found")
			return m, nil
		}

		m.err = nil
		m.deck = nil
		m.deckCards = nil
		m.cards = msg.cards
		m.totalCards = msg.totalCards
		items := make([]list.Item, len(msg.cards))
		for i, c := range msg.cards {
			items[i] = cardItem{card: c}
		}
		m = m.setResults(items)

		// Hand focus to the list so the results are immediately navigable.
		m.searchFocused = false
		m.searchInput.Blur()
		m.applyLayout()
		next, cmd := m.syncHover()
		return next, cmd

	case deckLoadedMsg:
		m.searching = false
		m.deckLoading = false
		// A deck can open and still have something to say — cards whose
		// names no longer resolve, or a lookup that couldn't be made. That's
		// a notice beside the counts, not a screen instead of the deck.
		if msg.err != nil && len(msg.cards) == 0 {
			m.err = msg.err
			return m, nil
		}
		m.notice = ""
		if msg.err != nil {
			m.notice = msg.err.Error()
		}

		m.err = nil
		info := msg.info
		m.deck = &info
		m.deckCards = msg.cards
		m.cards = make([]ScryfallCard, 0, len(msg.cards))
		for _, dc := range msg.cards {
			m.cards = append(m.cards, dc.card)
		}
		m.totalCards = info.total

		m = m.setResults(deckItems(msg.cards))

		m.searchInput.SetValue(info.ref())
		m.searchFocused = false
		m.searchInput.Blur()
		m.applyLayout()
		next, cmd := m.syncHover()
		return next, cmd
	}

	// Anything else (cursor blink, spinner ticks) goes to whatever has focus.
	if m.searchFocused {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.resultList, cmd = m.resultList.Update(msg)
	next, hoverCmd := m.syncHover()
	return next, tea.Batch(cmd, hoverCmd)
}

// updateSearchBar handles keys while the query is being edited.
func (m model) updateSearchBar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		query := strings.TrimSpace(m.searchInput.Value())
		if query == "" {
			return m.focusList(), nil
		}
		m.err = nil
		m.searching = true

		// A pasted Moxfield link loads that deck instead of being run as a
		// (hopeless) Scryfall query.
		if id, ok := moxfieldURLID(query); ok {
			m.deckLoading = true
			return m, loadDeckCmd(id)
		}
		return m, searchScryfall(query, sortOptions[m.sortIndex], maxResults)

	case "esc":
		if len(m.cards) == 0 {
			return m, tea.Quit
		}
		return m.focusList(), nil

	case "tab":
		m.sortIndex = (m.sortIndex + 1) % len(sortOptions)
		return m, nil

	case "shift+tab":
		m.sortIndex = (m.sortIndex - 1 + len(sortOptions)) % len(sortOptions)
		return m, nil

	// Browse the results without leaving the search bar.
	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		m.resultList, cmd = m.resultList.Update(msg)
		next, hoverCmd := m.syncHover()
		return next, tea.Batch(cmd, hoverCmd)
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// saveCurrentDeck copies the Moxfield deck on screen into a deck file of
// your own, which is the point at which it becomes editable. A deck that is
// already local has nothing to import.
func (m model) saveCurrentDeck() model {
	if m.deck == nil {
		m.notice = "nothing to save — open a deck first"
		return m
	}
	if m.deck.local() {
		m.notice = fmt.Sprintf("already saved · %s", m.deck.slug)
		return m
	}

	slug := slugify(m.deck.name)
	if slug == "" {
		slug = m.deck.id
	}

	// Re-importing an existing deck is how you pull changes down from
	// Moxfield, so it overwrites — but say which happened, because the two
	// read very differently when you didn't mean the second.
	verb := "saved"
	if deckExists(slug) {
		verb = "updated"
	}

	if err := writeDeck(slug, deckFileFrom(*m.deck, m.deckCards)); err != nil {
		m.notice = fmt.Sprintf("could not save: %v", err)
		return m
	}

	m.notice = fmt.Sprintf("%s · scry deck %s", verb, slug)
	return m
}

// updateList handles keys while the result list has focus.
func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key moves on from the last confirmation.
	if msg.String() != "w" {
		m.notice = ""
	}

	if m.resultList.FilterState() != list.Filtering {
		switch msg.String() {
		case "i", "ctrl+f":
			return m.focusSearch(), textinput.Blink
		case "esc":
			// esc peels one layer off at a time: the typed filter, then
			// the statistics category, then the app itself.
			if m.resultList.FilterState() != list.Unfiltered {
				m.resultList.ResetFilter()
				return m, nil
			}
			if m.statFilter != nil {
				return m.clearStatFilter()
			}
			return m, tea.Quit
		case "?":
			m.helpScroll = 0
			m.state = stateHelp
			return m, nil
		case "s":
			m.panel = m.panel.toggle(panelStats)
			m.previewScroll = 0
			return m, nil
		case "r":
			m.panel = m.panel.toggle(panelRules)
			return m, nil
		case "t":
			return m.toggleHistory()
		case "w":
			return m.saveCurrentDeck(), nil
		case "J", "shift+down":
			// The statistics panel is a list rather than a wall of text,
			// so J/K walks its categories and filters to them.
			if m.panel == panelStats {
				return m.statMove(1)
			}
			return m.scrollPanel(1), nil
		case "K", "shift+up":
			if m.panel == panelStats {
				return m.statMove(-1)
			}
			return m.scrollPanel(-1), nil
		case "ctrl+d", "ctrl+u":
			// Scrolling the panel proper, which in the statistics panel
			// means moving the view without moving the selected category.
			step := (m.resultsLayout().panelH - 1) / 2
			if step < 1 {
				step = 1
			}
			if msg.String() == "ctrl+u" {
				step = -step
			}
			return m.scrollPanel(step), nil
		case "enter":
			// Open the rules browser scoped to this card's keywords.
			if item, ok := m.resultList.SelectedItem().(cardItem); ok {
				return m.openRulesBrowser(m.cardRuleItems(item.card), item.card.Name)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.resultList, cmd = m.resultList.Update(msg)
	next, hoverCmd := m.syncHover()
	return next, tea.Batch(cmd, hoverCmd)
}

// setResults installs a fresh set of cards, forgetting whichever statistics
// category the last lot was narrowed to.
func (m model) setResults(items []list.Item) model {
	m.baseItems = items
	m.statFilter = nil
	m.statIndex = -1
	m.previewScroll = 0
	m.resultList.SetItems(items)
	m.resultList.ResetSelected()
	return m
}

func (m model) focusSearch() model {
	m.searchFocused = true
	m.searchInput.Focus()
	m.searchInput.CursorEnd()
	return m
}

func (m model) focusList() model {
	m.searchFocused = false
	m.searchInput.Blur()
	return m
}

// syncHover notices when the cursor lands on a different card and queues
// that card's rulings behind a short delay, so scrolling through a list
// doesn't fire a request per row.
func (m model) syncHover() (model, tea.Cmd) {
	item, ok := m.resultList.SelectedItem().(cardItem)
	if !ok {
		m.hoverKey = ""
		return m, nil
	}

	key := cardKey(item.card)
	if key == m.hoverKey {
		return m, nil
	}

	m.hoverKey = key
	m.previewScroll = 0
	m.rulesScroll = 0
	m.historyScroll = 0

	if _, done := m.rulings[key]; done {
		return m, nil
	}
	if m.inflight[key] {
		return m, nil
	}

	m.hoverSeq++
	seq := m.hoverSeq
	uri := item.card.RulingsURI
	return m, tea.Tick(rulingsDelay, func(time.Time) tea.Msg {
		return rulingsTickMsg{key: key, uri: uri, seq: seq}
	})
}

// openRulesBrowser switches to the rules browser. With nil items it shows
// every rule; otherwise it shows just the ones passed in.
func (m model) openRulesBrowser(items []list.Item, scope string) (tea.Model, tea.Cmd) {
	if !m.rules.loaded() {
		m.prevState = m.state
		m.state = stateRules
		return m, nil
	}

	// No keyword hits — fall back to the whole rulebook rather than
	// dropping the user into an empty list.
	if len(items) == 0 {
		items = buildRuleItems(m.rules)
		m.showGlossary = false
		m.rulesList.Title = fmt.Sprintf("Rules (%d)", len(items))
	} else {
		m.rulesList.Title = fmt.Sprintf("%s · %d rules", truncate(scope, 30), len(items))
	}

	m.rulesList.SetItems(items)
	m.rulesList.ResetSelected()
	m.browseScroll = 0
	m.prevState = m.state
	m.state = stateRules
	m.applyLayout()
	return m, nil
}

// cardRuleItems turns a card's matched rules into browser list items.
func (m model) cardRuleItems(c ScryfallCard) []list.Item {
	items := []list.Item{}
	for _, match := range m.rules.MatchCard(c) {
		switch match.Kind {
		case matchGlossary:
			items = append(items, glossaryItem{entry: match.Entry})
		default:
			if idx, ok := m.rules.Index[match.Rule]; ok {
				items = append(items, ruleItem{rule: m.rules.Rules[idx], index: idx})
			}
		}
	}
	return items
}

// ── Results layout ──────────────────────────────────────────────

// resultsLayout is the single source of truth for panel geometry, used
// both when sizing the list in Update and when drawing in View.
type resultsLayout struct {
	headerH  int
	bodyH    int
	listW    int
	listH    int
	panelW   int
	panelH   int
	vertical bool // panel sits under the list (narrow terminals)
}

const headerLines = 4 // search bar, sort, results, and the rule below them

func (m model) resultsLayout() resultsLayout {
	l := resultsLayout{headerH: headerLines}

	l.bodyH = m.height - l.headerH
	if l.bodyH < 6 {
		l.bodyH = 6
	}

	// The panel box is one column (or row) wider than its Width()/Height()
	// because of its border, which the -1s below account for.
	if m.width < compactWidth {
		// Narrow: list on top, panel underneath.
		l.vertical = true
		l.listW = m.width
		l.listH = l.bodyH / 2
		l.panelW = m.width
		l.panelH = l.bodyH - l.listH - 1
	} else {
		l.listW = m.width / 2
		l.listH = l.bodyH
		l.panelW = m.width - l.listW - 1
		l.panelH = l.bodyH
	}

	if l.listW < 20 {
		l.listW = 20
	}
	if l.panelW < 24 {
		l.panelW = 24
	}
	if l.listH < 3 {
		l.listH = 3
	}
	if l.panelH < 3 {
		l.panelH = 3
	}
	return l
}

// applyLayout keeps the list models sized to the current geometry so that
// paging and cursor movement agree with what's on screen.
func (m *model) applyLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	l := m.resultsLayout()
	m.resultList.SetSize(l.listW, l.listH)

	inputW := m.width - 12
	if inputW < 20 {
		inputW = 20
	}
	m.searchInput.Width = inputW

	rulesListW := m.width / 2
	if rulesListW < 20 {
		rulesListW = 20
	}
	m.rulesList.SetSize(rulesListW, m.height)
}

func (m model) resultsHeader() string {
	labelStyle := lipgloss.NewStyle().Foreground(gruvAqua).Bold(true).Width(9)
	valueStyle := lipgloss.NewStyle().Foreground(gruvYellow)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	// The search bar dims when the list has focus, so it's always clear
	// where typing will go.
	if m.searchFocused {
		m.searchInput.PromptStyle = lipgloss.NewStyle().Foreground(gruvOrange)
		m.searchInput.TextStyle = lipgloss.NewStyle().Foreground(gruvFg)
	} else {
		m.searchInput.PromptStyle = lipgloss.NewStyle().Foreground(gruvGray)
		m.searchInput.TextStyle = lipgloss.NewStyle().Foreground(gruvFgDim)
	}

	var b strings.Builder
	b.WriteString(m.searchInput.View() + "\n")

	// A deck's own name and author are more use on this line than a sort
	// order that only applies to the next search.
	if m.deck != nil {
		b.WriteString(labelStyle.Render("Deck") + valueStyle.Render(truncate(m.deck.name, 40)))
		if by := m.deck.author; by != "" {
			b.WriteString(dimStyle.Render("  by " + by))
		}
		if f := m.deck.format; f != "" {
			b.WriteString(dimStyle.Render("  · " + f))
		}
	} else {
		b.WriteString(labelStyle.Render("Sort") + valueStyle.Render(sortOptions[m.sortIndex]))
		if m.searchFocused {
			b.WriteString(dimStyle.Render("  tab: cycle  enter: search"))
		} else {
			b.WriteString(dimStyle.Render("  i: edit search  ?: help"))
		}
	}
	b.WriteString("\n")

	label := "Results"
	if m.deck != nil {
		label = "Cards"
	}
	b.WriteString(labelStyle.Render(label) + m.resultsSummary())

	return lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 2).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(gruvBgLight).
		Render(b.String())
}

// resultsSummary is the count line: how many cards, or what went wrong.
func (m model) resultsSummary() string {
	valueStyle := lipgloss.NewStyle().Foreground(gruvYellow)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	if m.deckLoading {
		return dimStyle.Render("loading deck…")
	}
	if m.searching {
		return dimStyle.Render("searching…")
	}
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(gruvRed).
			Render(truncate(fmt.Sprintf("%v", m.err), maxInt(m.width-15, 10)))
	}
	if len(m.cards) == 0 {
		return dimStyle.Render("type a query and press enter")
	}

	// A deck counts copies, so 99 cards can be 63 distinct ones.
	if m.deck != nil {
		out := valueStyle.Render(fmt.Sprintf("%d cards", m.deck.total))
		if m.deck.unique != m.deck.total {
			out += dimStyle.Render(fmt.Sprintf(" · %d unique", m.deck.unique))
		}
		if m.resultList.FilterState() != list.Unfiltered {
			out += dimStyle.Render(fmt.Sprintf("  (%d filtered)", len(m.resultList.VisibleItems())))
		}
		return out + m.noticeText()
	}

	shown := len(m.cards)
	out := formatInt(shown)
	if m.totalCards > shown {
		out = fmt.Sprintf("%s of %s", formatInt(shown), formatInt(m.totalCards))
	}
	out = valueStyle.Render(out)

	if m.resultList.FilterState() != list.Unfiltered {
		out += dimStyle.Render(fmt.Sprintf("  (%d filtered)", len(m.resultList.VisibleItems())))
	}
	return out + m.noticeText()
}

// noticeText is the last confirmation, if one is still standing, preceded by
// whichever statistics category the list is narrowed to.
func (m model) noticeText() string {
	out := ""
	if label := m.statFilterLabel(); label != "" {
		out += lipgloss.NewStyle().Foreground(gruvOrange).Render("  ▸ " + label)
	}
	if m.notice != "" {
		out += lipgloss.NewStyle().Foreground(gruvGreen).Render("  " + m.notice)
	}
	return out
}

func (m model) viewResults() string {
	l := m.resultsLayout()
	m.resultList.SetSize(l.listW, l.listH)

	// The list's own help line can render wider than the width it was
	// given, which would reflow everything beside it.
	listView := lipgloss.NewStyle().MaxWidth(l.listW).Render(m.resultList.View())

	panel := scrollView(m.panelContent(l.panelW-4), m.panelScroll(), l.panelH-1) +
		"\n" + m.panelHint()

	var body string
	if l.vertical {
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			listView,
			m.panelBox(panel, l.panelW, l.panelH, false, true),
		)
	} else {
		body = lipgloss.JoinHorizontal(
			lipgloss.Top,
			listView,
			m.panelBox(panel, l.panelW, l.panelH, true, false),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, m.resultsHeader(), body)
}

// panelContent renders whichever panel is on screen.
func (m model) panelContent(inner int) string {
	card, _ := m.resultList.SelectedItem().(cardItem)

	switch m.panel {
	case panelStats:
		return m.renderStats(inner)
	case panelRules:
		return m.renderCardRules(card.card, inner)
	case panelHistory:
		return m.renderTextHistory(card.card, inner)
	default:
		return m.renderPreview(card.card, inner)
	}
}

// panelScroll is how far the panel on screen is scrolled. The card and
// statistics panels share an offset — only one of them is ever showing.
func (m model) panelScroll() int {
	switch m.panel {
	case panelRules:
		return m.rulesScroll
	case panelHistory:
		return m.historyScroll
	default:
		return m.previewScroll
	}
}

// scrollPanel moves the panel on screen by delta lines, without disturbing
// anything it has selected — the statistics panel keeps its category.
func (m model) scrollPanel(delta int) model {
	l := m.resultsLayout()

	// Stop at the bottom of the content rather than scrolling into blank.
	maxScroll := strings.Count(m.panelContent(l.panelW-4), "\n") - (l.panelH - 1) + 1
	if maxScroll < 0 {
		maxScroll = 0
	}

	target := &m.previewScroll
	switch m.panel {
	case panelRules:
		target = &m.rulesScroll
	case panelHistory:
		target = &m.historyScroll
	}

	*target += delta
	if *target > maxScroll {
		*target = maxScroll
	}
	if *target < 0 {
		*target = 0
	}
	return m
}

// panelBox frames the panel beside (or below) the result list.
func (m model) panelBox(content string, w, h int, left, top bool) string {
	// Hard-clip first: a single over-long line would otherwise wrap and
	// push the rest of the panel out of place.
	inner := w - 4
	if inner < 1 {
		inner = 1
	}
	content = lipgloss.NewStyle().MaxWidth(inner).MaxHeight(h).Render(content)

	return lipgloss.NewStyle().
		Width(w).
		Height(h).
		Padding(0, 2).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(gruvGray).
		BorderLeft(left).
		BorderTop(top).
		BorderRight(false).
		BorderBottom(false).
		Render(content)
}

// panelHint spells out how to reach the panels you're not looking at.
func (m model) panelHint() string {
	parts := []string{"J/K: scroll"}
	switch m.panel {
	case panelStats:
		parts = []string{"J/K: category", "^d/^u: scroll", "s: card"}
		if m.statFilter != nil {
			parts = append(parts, "esc: clear")
		}
	case panelRules:
		parts = append(parts, "r: card", "s: stats", "enter: browse")
	case panelHistory:
		parts = append(parts, "t: card", "r: rules", "s: stats")
	default:
		parts = append(parts, "r: rules", "s: stats", "t: text history")
	}
	if m.deck != nil {
		parts = append(parts, "w: save")
	}
	return lipgloss.NewStyle().Foreground(gruvGray).Render(strings.Join(parts, "  "))
}
