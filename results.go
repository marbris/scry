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
		// The leader is waiting for the key that says what to do.
		if m.leader {
			if msg.String() == "esc" {
				m.leader = false
				return m, nil
			}
			return m.handleLeader(msg.String())
		}
		if m.searchFocused() {
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
		m.cards = msg.cards
		m.totalCards = msg.totalCards
		items := make([]list.Item, len(msg.cards))
		for i, c := range msg.cards {
			items[i] = cardItem{card: c}
		}
		m.results.setItems(items)
		m.previewScroll = 0

		// Hand focus to the results so they're immediately navigable. A
		// search no longer closes the deck: having both on screen at once is
		// the point of the deck column.
		m = m.setFocus(focusResults)
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
		m.deckPane.setItems(deckItems(msg.cards))
		m.previewScroll = 0

		// The deck gets its own column, so the search results stay where
		// they are. With nothing searched yet the deck is all there is to
		// look at, which is what an empty results list means.
		//
		// The search bar is left alone either way. It goes to Scryfall, and
		// "deck ghen" is not something Scryfall has ever heard of — putting
		// it there made the bar unusable until you cleared it. The deck's
		// name is on the header line and its column caption already.
		if m.results.empty() {
			m.cards = make([]ScryfallCard, 0, len(msg.cards))
			for _, dc := range msg.cards {
				m.cards = append(m.cards, dc.card)
			}
			m.totalCards = info.total
		}

		m = m.setFocus(focusDeck)
		m.applyLayout()
		next, cmd := m.syncHover()
		return next, cmd
	}

	// Anything else (cursor blink, spinner ticks) goes to whatever has focus.
	if m.searchFocused() {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.active().list, cmd = m.active().list.Update(msg)
	next, hoverCmd := m.syncHover()
	return next, tea.Batch(cmd, hoverCmd)
}

// updateSearchBar handles keys while the query is being edited.
func (m model) updateSearchBar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A comma is a character you might type in a query, so it only opens
	// the leader menu when there's no query to type it into. That is the
	// case that needs it: a fresh launch, where the search bar has focus
	// and there's no list to press a letter in.
	if msg.String() == leaderKey && m.searchInput.Value() == "" {
		m.leader = true
		return m, nil
	}

	switch msg.String() {
	case "enter":
		query := strings.TrimSpace(m.searchInput.Value())
		if query == "" {
			return m.setFocus(m.lastListFocus()), nil
		}
		m.err = nil
		m.searching = true
		m.leaveHistory()

		// A pasted Moxfield link loads that deck instead of being run as a
		// (hopeless) Scryfall query.
		if id, ok := moxfieldURLID(query); ok {
			m.deckLoading = true
			return m, loadDeckCmd(id)
		}

		m.queryHistory = rememberQuery(m.queryHistory, query)
		// A failed write costs the history, never the search.
		_ = saveQueryHistory(m.queryHistory)

		return m, searchScryfall(query, sortOptions[m.sortIndex], maxResults)

	case "esc":
		if len(m.cards) == 0 && !m.deckOpen() {
			return m.quitAfterSaving()
		}
		return m.setFocus(m.lastListFocus()), nil

	case "tab":
		m.sortIndex = (m.sortIndex + 1) % len(sortOptions)
		return m, nil

	case "shift+tab":
		m.sortIndex = (m.sortIndex - 1 + len(sortOptions)) % len(sortOptions)
		return m, nil

	// Up and down walk the queries you've run, the way a shell prompt
	// does. Browsing the results without leaving the search bar moved to
	// pgup/pgdown, which is the less used of the two by a distance.
	case "up":
		return m.recallQuery(-1), nil
	case "down":
		return m.recallQuery(1), nil

	case "pgup", "pgdown":
		var cmd tea.Cmd
		m.results.list, cmd = m.results.list.Update(msg)
		next, hoverCmd := m.syncHover()
		return next, tea.Batch(cmd, hoverCmd)
	}

	// Typing means you've stopped walking and started editing.
	m.leaveHistory()
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

	_, warning, err := saveDeckVersioned(slug, deckFileFrom(*m.deck, m.deckCards))
	if err != nil {
		m.notice = fmt.Sprintf("could not save: %v", err)
		return m
	}

	m.notice = fmt.Sprintf("%s · scry deck %s", verb, slug)
	if warning != "" {
		m.notice += " · " + warning
	}
	return m
}

// updateList handles keys while the result list has focus.
func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key moves on from the last confirmation, except the ones that
	// leave one behind.
	if !leavesNotice(msg.String()) {
		m.notice = ""
	}

	// The tag prompt takes every key while it's open.
	if m.tagging {
		return m.updateTagging(msg)
	}

	if !m.active().filtering() {
		switch msg.String() {
		case leaderKey:
			m.leader = true
			return m, nil
		case "q":
			return m.quitAfterSaving()
		case " ":
			return m.toggleMark()
		case "v":
			return m.markVisible()
		case "V":
			return m.clearMarks()
		case "T":
			return m.openTagPrompt()
		case "i", "ctrl+f":
			return m.setFocus(focusSearch), textinput.Blink
		case "tab", "shift+tab":
			// Between the results and the deck beside them.
			return m.cycleFocus(), nil
		case "esc":
			// esc peels one layer off at a time: the marks, then the typed
			// filter, then the statistics category, then the deck column,
			// then the app.
			if len(m.marks) > 0 {
				return m.clearMarks()
			}
			if m.active().list.FilterState() != list.Unfiltered {
				m.active().list.ResetFilter()
				return m, nil
			}
			if m.active().statFilter != nil {
				return m.clearStatFilter()
			}
			if m.focus == focusDeck && m.bothLists() {
				return m.setFocus(focusResults), nil
			}
			return m.quitAfterSaving()
		case "?":
			return m.openKeyReference()
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
			// w writes the open deck. Importing a Moxfield deck as one of
			// your own is a different thing and lives on the leader, so
			// one key no longer means two things.
			next, cmd := m.saveDeckNow()
			return next, cmd
		case "a":
			return m.addToDeck()
		case "x":
			return m.removeFromDeck()
		case "+", "=":
			return m.changeQty(1)
		case "-", "_":
			return m.changeQty(-1)
		case "c":
			return m.toggleCommander()
		case "u":
			return m.undoLast()
		case "o", "O":
			// Reorders what's on screen. The sort order in the header above
			// belongs to the next Scryfall query, which is a different
			// thing and stays on tab in the search bar.
			return m.cycleSort(map[bool]int{true: 1, false: -1}[msg.String() == "o"])
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
			if item, ok := m.active().selected(); ok {
				return m.openRulesBrowser(m.cardRuleItems(item.card), item.card.Name)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.active().list, cmd = m.active().list.Update(msg)
	next, hoverCmd := m.syncHover()
	return next, tea.Batch(cmd, hoverCmd)
}

// syncHover notices when the cursor lands on a different card and queues
// that card's rulings behind a short delay, so scrolling through a list
// doesn't fire a request per row.
func (m model) syncHover() (model, tea.Cmd) {
	item, ok := m.active().selected()
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
		m = m.enterState(stateRules)
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
	m = m.enterState(stateRules)
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
	headerH int
	bodyH   int
	listW   int
	listH   int
	deckW   int // 0 when the deck has no column of its own
	panelW  int
	panelH  int

	vertical bool // panel sits under the list (narrow terminals)
}

const (
	headerLines = 4 // search bar, sort, results, and the rule below them

	// Above this width the deck gets a column of its own beside the search
	// results, with the card panel still to the right of both. Below it
	// there's only room for one of the two, and `tab` swaps between them.
	// Three columns need roughly 50 each to be worth having.
	threeColumnWidth = 160
)

// deckColumn reports whether the deck is drawn beside the results rather
// than in place of the panel. It takes both a deck and something to put
// beside it: opening a deck on its own should show the deck, not the deck
// and an empty column where a search isn't.
func (m model) deckColumn() bool {
	return m.deckOpen() && !m.results.empty() && m.width >= threeColumnWidth
}

func (m model) resultsLayout() resultsLayout {
	l := resultsLayout{headerH: headerLines}

	// One row at the bottom belongs to the hint line.
	l.bodyH = m.height - l.headerH - 1
	if l.bodyH < 6 {
		l.bodyH = 6
	}

	// The panel box is one column (or row) wider than its Width()/Height()
	// because of its border, which the -1s below account for.
	switch {
	case m.width < compactWidth:
		// Narrow: list on top, panel underneath, one at a time.
		l.vertical = true
		l.listW = m.width
		l.listH = l.bodyH / 2
		l.panelW = m.width
		l.panelH = l.bodyH - l.listH - 1

	case m.deckColumn():
		// Wide: results, deck and panel side by side. The deck column is
		// the narrowest of the three — it's a list of names, where the
		// other two carry mana costs and type lines or whole rules texts.
		l.listH, l.panelH = l.bodyH, l.bodyH
		l.deckW = m.width * 28 / 100
		l.listW = (m.width - l.deckW - 2) / 2
		l.panelW = m.width - l.listW - l.deckW - 2

	default:
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
	if l.deckW != 0 && l.deckW < 18 {
		l.deckW = 18
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
	m.results.list.SetSize(l.listW, l.listH)
	if l.deckW > 0 {
		m.deckPane.list.SetSize(l.deckW-deckChrome, l.listH-1)
	} else {
		// Without a column of its own the deck takes the list's slot when
		// it has focus, so it's sized for that.
		m.deckPane.list.SetSize(l.listW, l.listH)
	}

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
	if m.searchFocused() {
		m.searchInput.PromptStyle = lipgloss.NewStyle().Foreground(gruvOrange)
		m.searchInput.TextStyle = lipgloss.NewStyle().Foreground(gruvFg)
	} else {
		m.searchInput.PromptStyle = lipgloss.NewStyle().Foreground(gruvGray)
		m.searchInput.TextStyle = lipgloss.NewStyle().Foreground(gruvFgDim)
	}

	var b strings.Builder
	b.WriteString(m.searchInput.View() + "\n")

	// A deck's own name and author are more use on this line than a sort
	// order that only applies to the next search — but only while the deck
	// is what the main list is showing. Once it has a column of its own the
	// column is captioned with its name, and this line goes back to the
	// sort order, which is about the search beside it.
	// Padded to the label column when it starts the line, plain when it's
	// tacked onto the end of the sort order.
	deckLine := func(padded bool) {
		label := labelStyle.Render("Deck")
		if !padded {
			label = lipgloss.NewStyle().Foreground(gruvAqua).Bold(true).Render("  Deck ")
		}
		b.WriteString(label + valueStyle.Render(truncate(m.deck.name, 40)))
		if by := m.deck.author; by != "" {
			b.WriteString(dimStyle.Render("  by " + by))
		}
		if f := m.deck.format; f != "" {
			b.WriteString(dimStyle.Render("  · " + f))
		}
	}

	switch {
	case m.deckInMainList():
		deckLine(true)

	case m.deckColumn():
		// Both are on screen, so this line carries both: the sort order the
		// search beside it will use, and whose deck the column is.
		b.WriteString(labelStyle.Render("Sort") + valueStyle.Render(sortOptions[m.sortIndex]))
		deckLine(false)

	default:
		b.WriteString(labelStyle.Render("Sort") + valueStyle.Render(sortOptions[m.sortIndex]))
	}
	b.WriteString("\n")

	label := "Results"
	if m.deckInMainList() {
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
	if m.deckInMainList() {
		out := valueStyle.Render(fmt.Sprintf("%d cards", m.deck.total))
		if m.deck.unique != m.deck.total {
			out += dimStyle.Render(fmt.Sprintf(" · %d unique", m.deck.unique))
		}
		if m.active().list.FilterState() != list.Unfiltered {
			out += dimStyle.Render(fmt.Sprintf("  (%d filtered)", len(m.active().list.VisibleItems())))
		}
		return out + m.noticeText()
	}

	shown := len(m.cards)
	out := formatInt(shown)
	if m.totalCards > shown {
		out = fmt.Sprintf("%s of %s", formatInt(shown), formatInt(m.totalCards))
	}
	out = valueStyle.Render(out)

	if m.active().list.FilterState() != list.Unfiltered {
		out += dimStyle.Render(fmt.Sprintf("  (%d filtered)", len(m.active().list.VisibleItems())))
	}
	return out + m.noticeText()
}

// noticeText is the last confirmation, if one is still standing, preceded by
// whichever statistics category the list is narrowed to.
func (m model) noticeText() string {
	out := ""
	if p := m.activeView(); p.sort != sortNone {
		out += lipgloss.NewStyle().Foreground(gruvBlue).Render("  ↕ " + p.sortName())
	}
	if n := m.markCount(); n > 0 {
		out += lipgloss.NewStyle().Foreground(gruvGreen).
			Render(fmt.Sprintf("  ● %d marked", n))
	}
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

	// Which list occupies the main slot. With a deck column both are on
	// screen at once; without one, the slot shows whichever has focus, and
	// tab swaps them.
	main := &m.results
	if m.deckInMainList() {
		main = &m.deckPane
	}
	main.list.SetSize(l.listW, l.listH)

	// Only the list taking keys is drawn in full colour. Whichever isn't
	// goes dim, which is a far clearer signal than the colour of the rule
	// between the columns.
	// The results are flagged with what the deck already holds; a deck row
	// carries its own commander flag, so it needs nothing passed in.
	inDeck := m.deckMembership()
	m.results.list.SetDelegate(compactDelegate{
		blurred: m.focus == focusDeck, marks: m.marks, inDeck: inDeck,
	})
	m.deckPane.list.SetDelegate(compactDelegate{
		blurred: m.focus != focusDeck, marks: m.marks,
	})

	// The list's own help line can render wider than the width it was
	// given, which would reflow everything beside it.
	listView := lipgloss.NewStyle().MaxWidth(l.listW).Render(main.list.View())

	panel := scrollView(m.panelContent(l.panelW-4), m.panelScroll(), l.panelH)

	var body string
	switch {
	case l.vertical:
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			listView,
			m.panelBox(panel, l.panelW, l.panelH, false, true),
		)

	case l.deckW > 0:
		// One line of the column goes to its heading.
		m.deckPane.list.SetSize(l.deckW-deckChrome, l.listH-1)
		deckView := m.deckPane.list.View()
		body = lipgloss.JoinHorizontal(
			lipgloss.Top,
			listView,
			m.deckBox(deckView, l.deckW, l.listH),
			m.panelBox(panel, l.panelW, l.panelH, true, false),
		)

	default:
		body = lipgloss.JoinHorizontal(
			lipgloss.Top,
			listView,
			m.panelBox(panel, l.panelW, l.panelH, true, false),
		)
	}

	// The bottom row is the hint line, or the tag prompt while one is open.
	bottom := []string{m.hintLine(m.width)}
	if m.tagging {
		bottom = []string{" " + m.tagInput.View()}
	}
	frame := lipgloss.JoinVertical(lipgloss.Left,
		m.resultsHeader(), body, strings.Join(bottom, "\n"))

	if m.leader {
		// The menu needs the whole width — squeezed into a column it loses
		// half its entries — and the frame is a fixed height, so it takes
		// the bottom lines rather than adding any.
		frame = replaceLastLines(frame, m.leaderBarLines(m.width))
	}
	return frame
}

// replaceLastLines swaps the final lines of a rendered block, keeping the
// block the same height.
func replaceLastLines(block string, with []string) string {
	lines := strings.Split(block, "\n")
	for i, l := range with {
		at := len(lines) - len(with) + i
		if at < 0 {
			continue
		}
		lines[at] = l
	}
	return strings.Join(lines, "\n")
}

// deckChrome is what the deck column's frame costs its contents: one column
// for the rule down its left edge and one for the padding inside it. The
// list has to be sized to what's left, or every row wraps.
const deckChrome = 2

// deckBox frames the deck column, with a heading naming the deck and a rule
// down its left edge. The border picks up the focus colour so it's obvious
// which of the two lists the keys are going to.
func (m model) deckBox(content string, w, h int) string {
	border := gruvGray
	if m.focus == focusDeck {
		border = gruvOrange
	}

	inner := w - deckChrome
	if inner < 1 {
		inner = 1
	}

	title, counts := "Deck", ""
	if m.deck != nil {
		counts = fmt.Sprintf(" %d · %d", m.deck.total, m.deck.unique)
		title = truncate(m.deck.name, inner-runeLen(counts))
	}

	// The caption carries the focus too: filled in when the column is
	// taking keys, dim when it isn't.
	titleStyle := lipgloss.NewStyle().Foreground(gruvGray)
	countStyle := titleStyle
	if m.focus == focusDeck {
		titleStyle = lipgloss.NewStyle().Foreground(gruvBg).Background(gruvOrange).Bold(true)
		countStyle = lipgloss.NewStyle().Foreground(gruvOrange)
	}
	heading := titleStyle.Render(" "+title+" ") + countStyle.Render(counts)

	// Hard-clip, as the panel does: one over-long row would otherwise wrap
	// and push everything below it down a line.
	content = lipgloss.NewStyle().MaxWidth(inner).MaxHeight(h).Render(heading + "\n" + content)

	return lipgloss.NewStyle().
		Width(w).
		Height(h).
		PaddingLeft(1).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(border).
		Render(content)
}

// panelContent renders whichever panel is on screen.
func (m model) panelContent(inner int) string {
	card, _ := m.active().selected()

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

// leavesNotice reports whether a key is one that says something afterwards.
// Every other key clears the last message rather than letting it linger over
// something it no longer describes.
func leavesNotice(key string) bool {
	switch key {
	case "w", "a", "x", "c", "u", "v", "V", "T", "+", "=", "-", "_":
		return true
	}
	return false
}
