// Package ui is the panel workspace: a row of panels you can open, close and
// move between, with one information panel pinned to the right.
//
// It deliberately stays one package. Panels hold views, views open panels,
// and the workspace holds panels — mutually referential by nature, so
// splitting it further would mean inventing interfaces to satisfy the
// compiler rather than to explain anything. Filenames do the organising.
package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/rules"
	"scry/internal/theme"
)

// Model is the Bubbletea model for the whole program.
type Model struct {
	ws   workspace
	info infoPanel

	// leader is set between pressing the leader key and the key that says
	// what to do with it; showKeys is the full reference, which is a lid
	// rather than a screen.
	leader   bool
	goPrefix bool
	showKeys bool

	// register is what y picked up, waiting for p. Whole deck cards, so a
	// card moved between decks brings its quantity and tags with it.
	register []deck.Card
	// lastTag is what T reaches for.
	lastTag string

	// quitting is the unsaved-changes question, raised when q would lose
	// something. Explicit saving is only safe if leaving asks.
	quitting bool

	// stats is the statistics mode of the information panel, and the
	// narrowing it is imposing on the lists it counts.
	stats statsState

	// histories are the printed-text histories fetched so far, by oracle id
	// — the identity that survives reprinting, which is the whole subject.
	histories map[string]*cardHistory

	// hoverSeq rises with every move, so a ruling fetched for a card you
	// have since scrolled past can be recognised as stale.
	hoverSeq int

	// notice is a one-line result — "copied", "deleted" — shown along the
	// bottom until the next keypress.
	notice string

	// rules is the comprehensive rulebook, parsed once when something first
	// asks for it. pending are the panels waiting for that to happen.
	rules        rules.Data
	rulesLoading bool
	rulesErr     error
	pending      []wantRules

	// history is every query run, shared by every find panel: searches you
	// ran in one panel are worth recalling in the next.
	history []string

	width, height int
}

// defaultQuerySort is EDHREC rank — for a Commander player the cards other
// people actually play are the ones worth seeing first.
const defaultQuerySort = 9

func New() Model {
	return Model{
		ws:        newWorkspace(),
		history:   LoadQueryHistory(),
		stats:     statsState{row: -1},
		histories: map[string]*cardHistory{},
	}
}

// NewWithQuery opens straight onto a search, for `scry --panels <query>`.
func NewWithQuery(query string) (Model, tea.Cmd) {
	m := New()
	p := m.ws.open(KindFind)
	p.history = m.history
	p.search.SetValue(query)
	p.search.CursorEnd()
	cmd := m.search(p)
	return m, cmd
}

// NewWithDeck opens straight onto one of your decks.
func NewWithDeck(slug string) (Model, tea.Cmd) {
	m := New()
	p := m.ws.open(KindDecks)
	p.searchOpen = false
	p.search.Blur()
	p.loading = true
	p.title = slug
	return m, openLocalDeck(p.id, true, slug)
}

// NewWithRemote opens straight onto a deck on Moxfield.
func NewWithRemote(id string) (Model, tea.Cmd) {
	m := New()
	p := m.ws.open(KindDecks)
	p.searchOpen = false
	p.search.Blur()
	p.loading = true
	p.title = id
	return m, openRemoteDeck(p.id, true, id)
}

// NewWithRules opens straight onto the rules — over a query, or on an empty
// panel with the bar waiting.
func NewWithRules(query string) (Model, tea.Cmd) {
	m := New()
	p := m.ws.open(KindRules)
	if query == "" {
		return m, nil
	}
	cmd := m.searchRules(p, query)
	return m, cmd
}

// NewRestored comes back to the workspace you left, or — with nothing to
// come back to — opens on the splash.
func NewRestored() (Model, tea.Cmd) {
	m := New()
	cmd := m.restore()
	return m, cmd
}

// quit saves the session on the way out. Every path that leaves goes
// through here, so there is one place that remembers to.
func (m Model) quit() tea.Cmd {
	m.saveSession()
	SaveQueryHistory(m.history)
	return tea.Quit
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.EnterAltScreen}
	// Parsed at the start when it's already downloaded, so a card's text is
	// highlighted from the first search. Not downloaded here: a megabyte
	// fetched before anyone has asked about a rule is presumptuous.
	if rules.Cached() {
		cmds = append(cmds, loadRules)
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Views built before the rulebook arrived get it now, so nothing has to
	// remember to ask. Deferred on a value receiver, which works because
	// what it changes is reached through the panel pointers rather than
	// through m itself.
	defer m.adoptRules()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ws.width, m.ws.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case searchDoneMsg:
		return m.handleSearchDone(msg)

	case deckOpenedMsg:
		return m.handleDeckOpened(msg)

	case userDecksMsg:
		return m.handleUserDecks(msg)

	case noticeMsg:
		if msg.err != nil {
			m.notice = "error: " + msg.err.Error()
		} else {
			m.notice = msg.text
		}
		return m, reloadDecks

	case deckSavedMsg:
		return m.handleDeckSaved(msg)

	case deckWrittenMsg:
		return m.handleDeckWritten(msg)

	case printingsMsg:
		return m.handlePrintings(msg)

	case setTextMsg:
		return m.handleSetText(msg)

	case rulingsTickMsg:
		return m.handleRulingsTick(msg)

	case rulingsMsg:
		return m.handleRulings(msg)

	case rulesLoadedMsg:
		return m.handleRulesLoaded(msg)

	case diffMsg:
		return m.handleDiff(msg)

	case versionsMsg:
		return m.handleVersions(msg)

	case legalityMsg:
		return m.handleLegality(msg)

	case reloadDecksMsg:
		var slugs []string
		for _, p := range m.ws.panels {
			if l, ok := p.top().(*deckList); ok {
				l.reload()
				slugs = append(slugs, l.localSlugs()...)
			}
		}
		return m, checkLegality(slugs)
	}
	return m, nil
}

// adoptRules hands the rulebook to every list of cards, so oracle text is
// highlighted wherever it appears.
func (m *Model) adoptRules() {
	if !m.rules.Loaded() {
		return
	}
	for _, p := range m.ws.panels {
		if l := p.cardsView(); l != nil && !l.rules.Loaded() {
			l.rules = m.rules
		}
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "" // no size yet; anything drawn now is drawn at the wrong one
	}

	base := lipgloss.NewStyle().
		Background(theme.Surface).
		Foreground(theme.Text).
		Width(m.width).
		Height(m.height).
		MaxWidth(m.width).
		MaxHeight(m.height)

	if m.showKeys {
		return base.Render(m.viewKeys())
	}
	if m.ws.empty() {
		return base.Render(m.viewSplash())
	}
	return base.Render(m.viewWorkspace())
}
