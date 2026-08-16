package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The Bubbletea model itself — what the app is holding, the messages that
// change it, and the Init/Update/View trio that dispatch to a screen. The
// screens live in results.go, help.go and rulesview.go.

// ── App state ───────────────────────────────────────────────────

type state int

const (
	stateResults state = iota // the main screen: search bar, list, panel
	stateRules
	stateHelp
	stateDeckHistory
	stateDecks
	stateKeys
	stateMoxUser
)

// panelMode is what the panel beside the result list is showing.
type panelMode int

const (
	panelCard panelMode = iota
	panelStats
	panelRules
	panelHistory
)

// toggle switches to mode, or back to the card if already there.
func (p panelMode) toggle(mode panelMode) panelMode {
	if p == mode {
		return panelCard
	}
	return mode
}

type model struct {
	state       state
	prevState   state // where stateRules was entered from
	searchInput textinput.Model
	// focus routes keys to the search bar, the results, or the deck;
	// returnFocus is the list to go back to when leaving the search bar.
	focus        focusArea
	returnFocus  focusArea
	searching    bool
	sortIndex    int
	cards        []ScryfallCard
	totalCards   int
	err          error
	width        int
	height       int
	initialQuery string
	helpScroll   int

	// The two lists of cards on screen: what you searched for, and the deck
	// you're building. The deck pane is empty until one is open, and the
	// column only appears when it isn't.
	results  pane
	deckPane pane

	// The open deck, or nil when the list is only holding search results.
	deck *deckInfo
	// deckCards is the open deck as a deck, keeping the quantities, tags and
	// command zone that the flat card list drops. Saving and editing both
	// work from this rather than going back to where the deck came from.
	deckCards   []deckCard
	deckLoading bool
	// deckDirty means there are edits not yet written. deckSeq rises with
	// every edit so a save scheduled by an earlier one can tell it's stale.
	deckDirty bool
	deckSeq   int
	// What to open on startup: a Moxfield id to browse, or the slug of a
	// deck file to open. At most one is ever set.
	initialDeck     string
	initialDeckSlug string
	// notice is a one-off confirmation ("saved as …") shown beside the
	// counts until the next keypress.
	notice string

	// marks are the cards picked out for tagging, by lowercased name so
	// they survive the list being filtered or rebuilt beneath them.
	// tagging is the inline tag prompt, open only while you're typing in it.
	marks    map[string]bool
	tagging  bool
	tagInput textinput.Model

	// Someone's decks on Moxfield: whose, what they have, and whether we're
	// mid-question or mid-request.
	moxUser        string
	moxUserList    list.Model
	moxUserInput   textinput.Model
	moxUserAsking  bool
	moxUserLoading bool
	moxUserErr     error

	// undo holds the deck as it was before each edit, so u can walk back.
	undo []undoStep

	// leader is set between pressing the leader key and the key that says
	// what to do; keysScroll is the key reference's scroll position.
	leader     bool
	keysScroll int

	// The panel beside the list shows one of card / stats / rules,
	// each keeping its own scroll position.
	panel         panelMode
	previewScroll int
	rulesScroll   int
	historyScroll int

	// Printed-text history, keyed by oracle id, plus the per-set printed
	// text it's assembled from
	histories map[string]*cardHistory
	originals map[string]map[string]string

	// Rulings, fetched lazily as the cursor moves
	rulings   map[string][]Ruling
	rulingErr map[string]error
	inflight  map[string]bool
	hoverKey  string
	hoverSeq  int

	// The deck picker: your decks, for opening one without going back to
	// the shell to remember what it was called.
	deckPicker    list.Model
	deckPickerErr error
	// naming is the inline prompt for a new deck's name, open only while
	// you're typing one.
	deckNameInput textinput.Model
	naming        bool

	// The open deck's git history, and the diff of whichever commit is
	// under the cursor.
	historyList list.Model
	historyDiff string
	historyErr  error
	diffScroll  int

	// Comprehensive rules
	rules        RulesData
	rulesErr     error
	rulesList    list.Model
	showGlossary bool
	browseScroll int
}

func cardKey(c ScryfallCard) string {
	if c.ID != "" {
		return c.ID
	}
	return c.Name + "|" + c.SetName
}

// ── Messages ────────────────────────────────────────────────────

type searchResultMsg struct {
	cards      []ScryfallCard
	totalCards int
	err        error
}

// rulingsTickMsg fires once a card has been hovered long enough to be
// worth a request; seq is compared against the model to drop stale ticks.
type rulingsTickMsg struct {
	key string
	uri string
	seq int
}

type rulingsMsg struct {
	key     string
	rulings []Ruling
	err     error
}

type rulesLoadedMsg struct {
	data RulesData
	err  error
}

// ── Init ────────────────────────────────────────────────────────

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "e.g. t:creature c:R cmc<=3 otag:removal"
	ti.Prompt = "⌕ "
	ti.Focus()
	ti.Width = 60
	ti.TextStyle = lipgloss.NewStyle().Foreground(gruvFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(gruvGray)
	ti.PromptStyle = lipgloss.NewStyle().Foreground(gruvOrange)

	newCardList := func(help bool) list.Model {
		l := list.New([]list.Item{}, compactDelegate{}, 40, 30)
		// The header above the list carries query/sort/count now.
		l.SetShowTitle(false)
		l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(gruvYellow)
		l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(gruvOrange)
		l.SetShowStatusBar(help)
		l.SetFilteringEnabled(true)
		l.SetShowHelp(help)
		// Literal rather than fuzzy: see filter.go.
		l.Filter = literalFilter
		return l
	}
	// Only the results list carries the help line; two copies of it in one
	// screen is noise, and the deck column is the narrower of the two.
	l := newCardList(true)
	dl := newCardList(false)

	rl := list.New([]list.Item{}, ruleDelegate{}, 40, 30)
	rl.Title = "Rules"
	rl.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(gruvOrange).
		Background(gruvBgLight).
		Padding(0, 1)
	rl.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(gruvYellow)
	rl.Styles.FilterCursor = lipgloss.NewStyle().Foreground(gruvOrange)
	rl.SetShowStatusBar(true)
	rl.SetFilteringEnabled(true)
	rl.SetShowHelp(true)
	rl.Filter = literalFilter

	return model{
		state:       stateResults,
		searchInput: ti,
		focus:       focusSearch,
		results:     pane{list: l, statIndex: -1},
		deckPane:    pane{list: dl, statIndex: -1},
		rulesList:   rl,
		sortIndex:   9,
		rulings:     make(map[string][]Ruling),
		rulingErr:   make(map[string]error),
		inflight:    make(map[string]bool),
		histories:   make(map[string]*cardHistory),
		originals:   make(map[string]map[string]string),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if !m.rules.loaded() {
		cmds = append(cmds, loadRulesCmd())
	}
	if m.initialDeck != "" {
		cmds = append(cmds, loadDeckCmd(m.initialDeck))
	}
	if m.initialDeckSlug != "" {
		cmds = append(cmds, openLocalDeckCmd(m.initialDeckSlug))
	}
	if m.initialQuery != "" {
		cmds = append(cmds, searchScryfall(m.initialQuery, sortOptions[m.sortIndex], maxResults))
	}
	return tea.Batch(cmds...)
}

// ── Update ──────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyLayout()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m.quitAfterSaving()
		}

	// Saving the open deck happens on a delay, so a burst of edits is one
	// commit. Both arrive whichever screen is up.
	case deckSaveTickMsg:
		return m.handleDeckSaveTick(msg)

	case deckSavedMsg:
		return m.handleDeckSaved(msg)

	// Rulings and rules arrive regardless of which screen is up, so they
	// are handled here rather than in a per-state update.
	case rulesLoadedMsg:
		if msg.err != nil {
			m.rulesErr = msg.err
			return m, nil
		}
		m.rules = msg.data
		// Don't clobber a list that was already populated (e.g. the
		// `rules <query>` entry point starts out pre-filtered).
		if len(m.rulesList.Items()) == 0 {
			items := buildRuleItems(m.rules)
			m.rulesList.SetItems(items)
			m.rulesList.Title = fmt.Sprintf("Rules (%d)", len(items))
		}
		return m, nil

	case rulingsTickMsg:
		if msg.seq != m.hoverSeq || msg.key != m.hoverKey {
			return m, nil // cursor moved on; never mind
		}
		if _, done := m.rulings[msg.key]; done || m.inflight[msg.key] {
			return m, nil
		}
		m.inflight[msg.key] = true
		return m, fetchRulings(msg.key, msg.uri)

	case printingsMsg:
		return m.handlePrintings(msg)

	case setOriginalsMsg:
		return m.handleSetOriginals(msg)

	case rulingsMsg:
		delete(m.inflight, msg.key)
		if msg.err != nil {
			m.rulingErr[msg.key] = msg.err
			return m, nil
		}
		delete(m.rulingErr, msg.key)
		m.rulings[msg.key] = msg.rulings
		return m, nil
	}

	switch m.state {
	case stateResults:
		return m.updateResults(msg)
	case stateRules:
		return m.updateRulesBrowse(msg)
	case stateHelp:
		return m.updateHelp(msg)
	case stateDeckHistory:
		return m.updateDeckHistory(msg)
	case stateDecks:
		return m.updateDeckPicker(msg)
	case stateKeys:
		return m.updateKeyReference(msg)
	case stateMoxUser:
		return m.updateMoxUser(msg)
	}
	return m, nil
}

// ── View ────────────────────────────────────────────────────────

func (m model) View() string {
	base := lipgloss.NewStyle().
		Background(gruvBg).
		Foreground(gruvFg).
		Width(m.width).
		Height(m.height).
		MaxWidth(m.width).
		MaxHeight(m.height)

	var content string
	switch m.state {
	case stateResults:
		content = m.viewResults()
	case stateRules:
		content = m.viewRulesBrowse()
	case stateHelp:
		content = m.viewHelp()
	case stateDeckHistory:
		content = m.viewDeckHistory()
	case stateDecks:
		content = m.viewDeckPicker()
	case stateKeys:
		content = m.viewKeyReference()
	case stateMoxUser:
		content = m.viewMoxUser()
	}

	return base.Render(content)
}
