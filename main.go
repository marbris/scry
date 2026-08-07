package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// ── Gruvbox palette ─────────────────────────────────────────────

var (
	gruvBg      = lipgloss.Color("#282828")
	gruvFg      = lipgloss.Color("#ebdbb2")
	gruvRed     = lipgloss.Color("#cc241d")
	gruvGreen   = lipgloss.Color("#98971a")
	gruvYellow  = lipgloss.Color("#d79921")
	gruvBlue    = lipgloss.Color("#458588")
	gruvPurple  = lipgloss.Color("#b16286")
	gruvAqua    = lipgloss.Color("#689d6a")
	gruvOrange  = lipgloss.Color("#d65d0e")
	gruvGray    = lipgloss.Color("#928374")
	gruvFgDim   = lipgloss.Color("#a89984")
	gruvBgLight = lipgloss.Color("#3c3836")
)

const (
	// Below this width the panel sits under the list instead of beside it
	compactWidth = 120

	// Scryfall returns 175 cards per page; that's the whole result set we keep.
	maxResults = 175

	// How long a card has to stay under the cursor before its rulings
	// are fetched, so scrolling past a card costs nothing.
	rulingsDelay = 100 * time.Millisecond
)

// ── Scryfall types ──────────────────────────────────────────────

type ScryfallResponse struct {
	Data       []ScryfallCard `json:"data"`
	TotalCards int            `json:"total_cards"`
	HasMore    bool           `json:"has_more"`
	NextPage   string         `json:"next_page"`
}

type ScryfallCard struct {
	ID              string `json:"id"`
	OracleID        string `json:"oracle_id"`
	PrintsSearchURI string `json:"prints_search_uri"`
	Set             string `json:"set"`
	ReleasedAt      string `json:"released_at"`
	Lang            string `json:"lang"`
	Digital         bool   `json:"digital"`

	Name          string            `json:"name"`
	ManaCost      string            `json:"mana_cost"`
	TypeLine      string            `json:"type_line"`
	OracleText    string            `json:"oracle_text"`
	Colors        []string          `json:"colors"`
	ColorIdentity []string          `json:"color_identity"`
	Power         string            `json:"power"`
	Toughness     string            `json:"toughness"`
	Loyalty       string            `json:"loyalty"`
	SetName       string            `json:"set_name"`
	Rarity        string            `json:"rarity"`
	RulingsURI    string            `json:"rulings_uri"`
	Legalities    map[string]string `json:"legalities"`
	CMC           float64           `json:"cmc"`
	EDHRECRank    int               `json:"edhrec_rank"`
}

type Ruling struct {
	Source  string `json:"source"`
	Comment string `json:"comment"`
}

type RulingsResponse struct {
	Data []Ruling `json:"data"`
}

// ── Sort options ────────────────────────────────────────────────

var sortOptions = []string{
	"name",
	"released",
	"set",
	"rarity",
	"color",
	"usd",
	"cmc",
	"power",
	"toughness",
	"edhrec",
	"artist",
	"review",
}

// ── Single-line delegate ────────────────────────────────────────

type compactDelegate struct{}

func (d compactDelegate) Height() int                             { return 1 }
func (d compactDelegate) Spacing() int                            { return 0 }
func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ci, ok := item.(cardItem)
	if !ok {
		return
	}

	selected := index == m.Index()
	c := ci.card

	col := colorForCard(c.Colors)
	mana := c.ManaCost
	if mana == "" {
		mana = "·"
	}

	nameW := 30
	typeW := 30
	manaW := 15

	name := truncate(c.Name, nameW)
	typeLine := truncate(c.TypeLine, typeW)
	mana = truncate(mana, manaW)

	nameStyle := lipgloss.NewStyle().
		Width(nameW).
		Foreground(col)
	typeStyle := lipgloss.NewStyle().
		Width(typeW).
		Foreground(gruvFgDim)

	renderedMana := renderMana(c.ManaCost)
	if renderedMana == "" {
		renderedMana = lipgloss.NewStyle().Foreground(gruvFgDim).Render("·")
	}
	// Pad to keep columns aligned
	rawMana := strings.ReplaceAll(strings.ReplaceAll(c.ManaCost, "{", ""), "}", "")
	if rawMana == "" {
		rawMana = "·"
	}
	pad := 10 - len(rawMana)
	if pad < 0 {
		pad = 0
	}

	line := nameStyle.Render(name) + "  " +
		renderedMana + strings.Repeat(" ", pad) + "  " +
		typeStyle.Render(typeLine)

	if selected {
		cursor := lipgloss.NewStyle().Foreground(gruvOrange).Render("▸ ")
		line = cursor + lipgloss.NewStyle().
			Background(gruvBgLight).
			Render(line)
	} else {
		line = "  " + line
	}

	fmt.Fprint(w, line)
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-1] + "…"
	}
	return s
}

// ── List item adapter ───────────────────────────────────────────

type cardItem struct {
	card ScryfallCard
}

func (c cardItem) Title() string       { return c.card.Name }
func (c cardItem) Description() string { return c.card.TypeLine }
func (c cardItem) FilterValue() string {
	return c.card.Name + " " + c.card.OracleText
}

// ── Color helpers ───────────────────────────────────────────────

func colorForCard(colors []string) lipgloss.Color {
	if len(colors) > 1 {
		return gruvYellow // multicolor
	}
	if len(colors) == 1 {
		switch colors[0] {
		case "W":
			return lipgloss.Color("#fbf1c7")
		case "U":
			return gruvBlue
		case "B":
			return gruvPurple
		case "R":
			return gruvRed
		case "G":
			return gruvGreen
		}
	}
	return gruvFgDim // colorless
}

func renderMana(manaCost string) string {
	if manaCost == "" {
		return ""
	}

	colorMap := map[string]lipgloss.Color{
		"W": lipgloss.Color("#fbf1c7"),
		"U": gruvBlue,
		"B": gruvPurple,
		"R": gruvRed,
		"G": gruvGreen,
		"C": gruvFgDim,
		"X": gruvYellow,
		"S": gruvGray,
	}

	// Strip braces, keep symbols
	stripped := strings.ReplaceAll(manaCost, "{", "")
	symbols := strings.Split(stripped, "}")

	var result strings.Builder
	for _, s := range symbols {
		if s == "" {
			continue
		}
		if col, ok := colorMap[s]; ok {
			result.WriteString(lipgloss.NewStyle().Foreground(col).Render(s))
		} else {
			// Generic/numeric mana
			result.WriteString(lipgloss.NewStyle().Foreground(gruvFgDim).Render(s))
		}
	}
	return result.String()
}

// ── App state ───────────────────────────────────────────────────

type state int

const (
	stateResults state = iota // the only main screen: search bar, list, panel
	stateRules
	stateHelp
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
	// searchFocused routes typing to the search bar instead of the list
	searchFocused bool
	searching     bool
	sortIndex     int
	resultList    list.Model
	cards         []ScryfallCard
	totalCards    int
	err           error
	width         int
	height        int
	initialQuery  string
	helpScroll    int

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

// ── helpScroll ────────────────────────────────────────────────────
func syntaxHelp() string {
	return `╔══════════════════════════════════════════════════════════════╗
║                    Scryfall Syntax Guide                     ║
╚══════════════════════════════════════════════════════════════╝

KEYS — SEARCH BAR
  enter                   Run the search, then move to the results
  tab / shift+tab         Cycle sort order
  ↑/↓                     Move through the results while typing
  esc                     Back to the results (quits if there are none)

KEYS — RESULTS
  j/k, ↑/↓                Move through cards
  /                       Filter the results (name + oracle text)
  i, esc                  Edit the search query
  J/K, shift+↑/↓          Scroll the panel
  r                       Rules for this card (again for the card view)
  s                       Statistics for these results (again for the card)
  t                       Printed text history — how the card's wording
                          changed across printings (again for the card view)
  enter                   Browse the rules matched by this card
  ctrl+r                  Browse all comprehensive rules
  ?                       This help

KEYS — RULES BROWSER
  /                       Search rules and glossary
  g                       Toggle rules / glossary
  J/K                     Scroll the rule text
  esc                     Back

BASIC SEARCH
  lightning bolt          Fuzzy name search
  !"Lightning Bolt"       Exact name match
  o:"draw a card"         Oracle text contains
  t:creature              Type line contains
  t:"legendary creature"  Multi-word type

COLORS
  c:R                     Color is red
  c:RG                    Color is exactly red and green
  c>=RG                   Color includes red and green (and maybe more)
  c<=WUB                  Color is within white/blue/black
  c:m                     Is multicolored
  c:c                     Is colorless

COLOR IDENTITY (for Commander)
  id:RG                   Identity is exactly red/green
  id<=BRG                 Identity within Jund
  id:c                    Colorless identity
  id>=W                   Identity includes white

MANA & CMC
  cmc=3                   Converted mana cost equals 3
  cmc<=5                  CMC is 5 or less
  cmc>=2                  CMC is 2 or more
  m:{2}{W}{W}             Mana cost contains specific symbols
  m:{C}                   Requires colorless mana

FORMATS & LEGALITY
  f:commander             Legal in Commander/EDH
  f:standard              Legal in Standard
  f:modern                Legal in Modern
  f:pioneer               Legal in Pioneer
  f:pauper                Legal in Pauper
  f:legacy                Legal in Legacy
  banned:commander        Banned in Commander
  restricted:vintage      Restricted in Vintage

COMMANDER / EDH
  is:commander            Can be a commander
  is:brawler              Can be a Brawl commander
  is:companion            Is a companion
  is:partner              Has partner
  is:background           Is a background
  id<=RG is:commander     Commanders within Gruul identity
  otag:removal f:commander  Removal cards legal in Commander
  t:legendary t:creature  Legendary creatures

CARD TYPES & SUPERTYPES
  t:creature              Creatures
  t:instant               Instants
  t:sorcery               Sorceries
  t:artifact              Artifacts
  t:enchantment           Enchantments
  t:planeswalker          Planeswalkers
  t:land                  Lands
  t:legendary             Legendary permanents
  t:snow                  Snow permanents
  t:equipment             Equipment
  t:aura                  Auras
  t:saga                  Sagas

POWER, TOUGHNESS, LOYALTY
  pow=4                   Power equals 4
  pow>=7                  Power 7 or more
  tou<=3                  Toughness 3 or less
  loy=5                   Loyalty equals 5

RARITY
  r:common                Commons
  r:uncommon              Uncommons
  r:rare                  Rares
  r:mythic                Mythic rares

ORACLE TAGS (Tagger)
  otag:removal            Cards tagged as removal
  otag:ramp               Cards tagged as ramp
  otag:draw               Cards tagged as card draw
  otag:boardwipe          Board wipes
  otag:counterspell       Counterspells
  otag:graveyard-hate     Graveyard hate
  otag:sacrifice-outlet   Sacrifice outlets
  otag:token-maker        Token generators
  otag:lifegain           Lifegain cards
  otag:evasion            Evasive creatures

SETS & PRINTS
  s:mkm                   From a specific set
  year=2024               Printed in year
  is:reprint              Is a reprint
  is:firstprint           First printing
  new:art                 New art in recent set

MISC FILTERS
  is:modal                Modal cards (MDFCs, split, etc.)
  is:transform            Transforming cards
  is:flip                 Flip cards
  is:token                Token cards
  is:funny                Un-cards / acorn
  is:reserved             Reserved list
  is:fetchland            Fetch lands
  is:dual                 Dual lands
  has:watermark            Has a watermark
  art:greg-staples        Art by specific artist

LOGIC & GROUPING
  t:creature t:dragon     AND (implicit between terms)
  t:creature c:R          AND (both must be true)
  t:angel or t:demon      OR
  t:creature -c:G         NOT (exclude green creatures)
  (t:goblin or t:elf) c:R Grouping with parentheses
  -is:reprint             Exclude reprints

EXAMPLES FOR COMMANDER
  id<=BRG is:commander t:creature
      Jund commanders that are creatures

  id<=WU otag:draw f:commander cmc<=3
      Azorius card draw under 3 CMC

  otag:removal id<=R f:commander
      Red removal legal in Commander

  t:land id<=WUBRG produces>=RG
      Lands that produce red and green

  is:partner id<=WB
      Partners within Orzhov identity

  otag:boardwipe cmc<=5 f:commander
      Cheap board wipes for Commander

Full syntax docs: https://scryfall.com/docs/syntax
`
}

func (m model) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "?":
			m.state = stateResults
			return m, nil
		case "down", "j":
			m.helpScroll++
			return m, nil
		case "up", "k":
			if m.helpScroll > 0 {
				m.helpScroll--
			}
			return m, nil
		case "d":
			m.helpScroll += 10
			return m, nil
		case "u":
			m.helpScroll -= 10
			if m.helpScroll < 0 {
				m.helpScroll = 0
			}
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewHelp() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(gruvOrange)

	dimStyle := lipgloss.NewStyle().
		Foreground(gruvGray)

	textStyle := lipgloss.NewStyle().
		Foreground(gruvFg).
		Width(m.width - 6)

	lines := strings.Split(textStyle.Render(syntaxHelp()), "\n")

	viewH := m.height - 4
	if viewH < 5 {
		viewH = 5
	}
	if m.helpScroll > len(lines)-viewH {
		m.helpScroll = len(lines) - viewH
	}
	if m.helpScroll < 0 {
		m.helpScroll = 0
	}

	end := m.helpScroll + viewH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[m.helpScroll:end]

	footer := "\n" + dimStyle.Render("j/k: scroll  d/u: page  esc/q/?: back") +
		"  " + titleStyle.Render(fmt.Sprintf("[%d/%d]", m.helpScroll+1, len(lines)))

	return lipgloss.NewStyle().Padding(1, 2).Render(
		strings.Join(visible, "\n") + footer,
	)
}

// ── HTTP helper ─────────────────────────────────────────────────

type notFoundError struct{}

func (e notFoundError) Error() string { return "no results found" }

func doGet(u string) ([]byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "scry/1.0")
	req.Header.Set("Accept", "application/json;q=0.9,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, notFoundError{}
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("scryfall (%d): %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// ── Commands ────────────────────────────────────────────────────

func searchScryfall(query string, sort string, limit int) tea.Cmd {
	return func() tea.Msg {
		var allCards []ScryfallCard
		page := 1

		for {
			u := fmt.Sprintf(
				"https://api.scryfall.com/cards/search?q=%s&order=%s&page=%d",
				url.QueryEscape(query), url.QueryEscape(sort), page,
			)
			body, err := doGet(u)
			if err != nil {
				if _, ok := err.(notFoundError); ok {
					return searchResultMsg{cards: []ScryfallCard{}, totalCards: 0}
				}
				if len(allCards) > 0 {
					break
				}
				return searchResultMsg{err: err}
			}

			var sr ScryfallResponse
			if err := json.Unmarshal(body, &sr); err != nil {
				return searchResultMsg{err: err}
			}

			allCards = append(allCards, sr.Data...)

			if len(allCards) >= limit || !sr.HasMore {
				if len(allCards) > limit {
					allCards = allCards[:limit]
				}
				return searchResultMsg{cards: allCards, totalCards: sr.TotalCards}
			}
			page++
		}

		if len(allCards) > limit {
			allCards = allCards[:limit]
		}
		return searchResultMsg{cards: allCards, totalCards: len(allCards)}
	}
}

func fetchRulings(key, uri string) tea.Cmd {
	return func() tea.Msg {
		rulings, err := getRulings(uri)
		if err != nil {
			return rulingsMsg{key: key, err: err}
		}
		return rulingsMsg{key: key, rulings: rulings}
	}
}

// getRulings fetches a card's rulings, returning an empty (non-nil) slice
// when the card simply has none.
func getRulings(uri string) ([]Ruling, error) {
	if uri == "" {
		return []Ruling{}, nil
	}
	body, err := doGet(uri)
	if err != nil {
		return nil, err
	}
	var rr RulingsResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, err
	}
	if rr.Data == nil {
		rr.Data = []Ruling{}
	}
	return rr.Data, nil
}

// loadRulesCmd parses the comprehensive rules off the main thread — it's
// a megabyte of text, and nothing needs it until a card is on screen.
func loadRulesCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := loadRules()
		return rulesLoadedMsg{data: data, err: err}
	}
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

	l := list.New([]list.Item{}, compactDelegate{}, 40, 30)
	// The header above the list carries query/sort/count now.
	l.SetShowTitle(false)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(gruvYellow)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(gruvOrange)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

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

	return model{
		state:         stateResults,
		searchInput:   ti,
		searchFocused: true,
		resultList:    l,
		rulesList:     rl,
		sortIndex:     9,
		rulings:       make(map[string][]Ruling),
		rulingErr:     make(map[string]error),
		inflight:      make(map[string]bool),
		histories:     make(map[string]*cardHistory),
		originals:     make(map[string]map[string]string),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if !m.rules.loaded() {
		cmds = append(cmds, loadRulesCmd())
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
			return m, tea.Quit
		}

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
	}
	return m, nil
}

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
		m.cards = msg.cards
		m.totalCards = msg.totalCards
		items := make([]list.Item, len(msg.cards))
		for i, c := range msg.cards {
			items[i] = cardItem{card: c}
		}
		m.resultList.SetItems(items)
		m.resultList.ResetSelected()

		// Hand focus to the list so the results are immediately navigable.
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

// updateList handles keys while the result list has focus.
func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.resultList.FilterState() != list.Filtering {
		switch msg.String() {
		case "esc", "i", "ctrl+f":
			return m.focusSearch(), textinput.Blink
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
		case "J", "shift+down":
			switch m.panel {
			case panelRules:
				m.rulesScroll++
			case panelHistory:
				m.historyScroll++
			default:
				m.previewScroll++
			}
			return m, nil
		case "K", "shift+up":
			switch m.panel {
			case panelRules:
				if m.rulesScroll > 0 {
					m.rulesScroll--
				}
			case panelHistory:
				if m.historyScroll > 0 {
					m.historyScroll--
				}
			default:
				if m.previewScroll > 0 {
					m.previewScroll--
				}
			}
			return m, nil
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
	}

	return base.Render(content)
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
	b.WriteString(labelStyle.Render("Sort") + valueStyle.Render(sortOptions[m.sortIndex]))
	if m.searchFocused {
		b.WriteString(dimStyle.Render("  tab: cycle  enter: search"))
	} else {
		b.WriteString(dimStyle.Render("  i: edit search  ?: help"))
	}
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Results") + m.resultsSummary())

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

	shown := len(m.cards)
	out := formatInt(shown)
	if m.totalCards > shown {
		out = fmt.Sprintf("%s of %s", formatInt(shown), formatInt(m.totalCards))
	}
	out = valueStyle.Render(out)

	if m.resultList.FilterState() != list.Unfiltered {
		out += dimStyle.Render(fmt.Sprintf("  (%d filtered)", len(m.resultList.VisibleItems())))
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatInt(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteString(",")
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
func (m model) viewResults() string {
	l := m.resultsLayout()
	m.resultList.SetSize(l.listW, l.listH)

	// The list's own help line can render wider than the width it was
	// given, which would reflow everything beside it.
	listView := lipgloss.NewStyle().MaxWidth(l.listW).Render(m.resultList.View())

	card, _ := m.resultList.SelectedItem().(cardItem)
	inner := l.panelW - 4

	var content string
	var scroll int
	switch m.panel {
	case panelStats:
		content, scroll = m.renderStats(inner), m.previewScroll
	case panelRules:
		content, scroll = m.renderCardRules(card.card, inner), m.rulesScroll
	case panelHistory:
		content, scroll = m.renderTextHistory(card.card, inner), m.historyScroll
	default:
		content, scroll = m.renderPreview(card.card, inner), m.previewScroll
	}
	panel := scrollView(content, scroll, l.panelH-1) + "\n" + m.panelHint()

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
		parts = append(parts, "s: card", "r: rules", "t: text history")
	case panelRules:
		parts = append(parts, "r: card", "s: stats", "enter: browse")
	case panelHistory:
		parts = append(parts, "t: card", "r: rules", "s: stats")
	default:
		parts = append(parts, "r: rules", "s: stats", "t: text history")
	}
	return lipgloss.NewStyle().Foreground(gruvGray).Render(strings.Join(parts, "  "))
}

func (m model) renderPreview(c ScryfallCard, maxW int) string {
	if c.Name == "" {
		return lipgloss.NewStyle().Foreground(gruvGray).Render("No card selected")
	}
	if maxW < 24 {
		maxW = 24
	}

	col := colorForCard(c.Colors)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(col)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua).MarginTop(1)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	var b strings.Builder

	nameLine := nameStyle.Render(c.Name)
	if c.ManaCost != "" {
		nameLine += "  " + renderMana(c.ManaCost)
	}
	b.WriteString(nameLine + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(gruvFgDim).Render(c.TypeLine) + "\n")

	if c.Power != "" && c.Toughness != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(gruvPurple).
			Render(fmt.Sprintf("P/T: %s/%s", c.Power, c.Toughness)) + "\n")
	}
	if c.Loyalty != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(gruvPurple).
			Render(fmt.Sprintf("Loyalty: %s", c.Loyalty)) + "\n")
	}

	b.WriteString(dimStyle.Render(fmt.Sprintf("%s · %s · CMC %.0f", c.SetName, c.Rarity, c.CMC)) + "\n")

	if c.EDHRECRank > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("EDHREC Rank: #%d", c.EDHRECRank)) + "\n")
	}

	if c.OracleText != "" {
		b.WriteString(headerStyle.Render("Oracle Text") + "\n")
		b.WriteString(highlightOracle(c, maxW, m.rules) + "\n")
	}

	// Rulings — fetched in the background once the cursor settles here.
	b.WriteString(m.renderRulings(c, maxW))

	// Legalities
	if len(c.Legalities) > 0 {
		b.WriteString(headerStyle.Render("Legalities") + "\n")
		formats := []string{"standard", "pioneer", "modern", "legacy", "vintage", "commander", "pauper"}
		for _, f := range formats {
			if v, ok := c.Legalities[f]; ok {
				indicator := lipgloss.NewStyle().Foreground(gruvRed).Render("✘")
				label := lipgloss.NewStyle().Foreground(gruvRed)
				if v == "legal" {
					indicator = lipgloss.NewStyle().Foreground(gruvGreen).Render("✔")
					label = lipgloss.NewStyle().Foreground(gruvGreen)
				}
				b.WriteString(fmt.Sprintf("  %s %s\n", indicator, label.Render(f)))
			}
		}
	}

	return b.String()
}

func (m model) renderRulings(c ScryfallCard, maxW int) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua).MarginTop(1)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)
	key := cardKey(c)

	rulings, ok := m.rulings[key]
	if !ok {
		// A failed fetch only matters while there's nothing cached.
		if err := m.rulingErr[key]; err != nil {
			return headerStyle.Render("Rulings") + "\n" +
				lipgloss.NewStyle().Foreground(gruvRed).Render(fmt.Sprintf("unavailable: %v", err)) + "\n"
		}
		return headerStyle.Render("Rulings") + "\n" + dimStyle.Render("loading…") + "\n"
	}
	if len(rulings) == 0 {
		return headerStyle.Render("Rulings") + "\n" + dimStyle.Render("none") + "\n"
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("Rulings (%d)", len(rulings))) + "\n")
	for _, r := range rulings {
		bullet := lipgloss.NewStyle().Foreground(gruvOrange).Render("• ")
		body := highlightRuleText(r.Comment, maxW-2, m.rules)
		b.WriteString(bullet + strings.TrimPrefix(indent(body, 2), "  ") + "\n")
	}
	return b.String()
}

// ── Statistics helpers ──────────────────────────────────────────

func (m model) getVisibleCards() []ScryfallCard {
	items := m.resultList.VisibleItems()
	cards := make([]ScryfallCard, 0, len(items))
	for _, item := range items {
		if ci, ok := item.(cardItem); ok {
			cards = append(cards, ci.card)
		}
	}
	return cards
}

func renderHistogram(title string, counts map[string]int, order []string, maxW int, labelW int, colorFn func(string) lipgloss.Color) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	var b strings.Builder
	b.WriteString(headerStyle.Render(title) + "\n")

	if len(counts) == 0 {
		b.WriteString(dimStyle.Render("  (none)") + "\n")
		return b.String()
	}

	// Find max count for scaling
	maxCount := 0
	for _, k := range order {
		if c, ok := counts[k]; ok && c > maxCount {
			maxCount = c
		}
	}
	// Also check for keys not in order
	for k, c := range counts {
		found := false
		for _, o := range order {
			if o == k {
				found = true
				break
			}
		}
		if !found {
			order = append(order, k)
		}
		if c > maxCount {
			maxCount = c
		}
	}

	if maxCount == 0 {
		b.WriteString(dimStyle.Render("  (none)") + "\n")
		return b.String()
	}

	// Bar area width
	countW := len(fmt.Sprintf("%d", maxCount))
	barMaxW := maxW - labelW - countW - 5
	if barMaxW < 5 {
		barMaxW = 5
	}

	for _, k := range order {
		c, ok := counts[k]
		if !ok || c == 0 {
			continue
		}

		barLen := (c * barMaxW) / maxCount
		if barLen < 1 {
			barLen = 1
		}

		col := gruvFg
		if colorFn != nil {
			col = colorFn(k)
		}

		label := lipgloss.NewStyle().
			Width(labelW).
			Foreground(col).
			Render(k)

		bar := lipgloss.NewStyle().
			Foreground(col).
			Render(strings.Repeat("█", barLen))

		count := lipgloss.NewStyle().
			Foreground(gruvFgDim).
			Render(fmt.Sprintf(" %d", c))

		b.WriteString(fmt.Sprintf("  %s %s%s\n", label, bar, count))
	}

	return b.String()
}

func (m model) renderStats(maxW int) string {
	cards := m.getVisibleCards()

	if maxW < 30 {
		maxW = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvOrange)

	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf(" Statistics (%d cards)", len(cards))) + "\n\n")

	// Gather all label names to compute a global label width
	allLabels := []string{
		"White", "Blue", "Black", "Red", "Green", "Colorless", "Multi",
		"common", "uncommon", "rare", "mythic", "special", "bonus",
		"0", "1", "2", "3", "4", "5", "6", "7+",
		"Creature", "Instant", "Sorcery", "Artifact", "Enchantment", "Planeswalker", "Land",
	}
	globalLabelW := 0
	for _, l := range allLabels {
		if len(l) > globalLabelW {
			globalLabelW = len(l)
		}
	}

	// ── Color Distribution ──
	colorCounts := map[string]int{}
	for _, c := range cards {
		if len(c.Colors) == 0 {
			colorCounts["Colorless"]++
		} else {
			for _, col := range c.Colors {
				switch col {
				case "W":
					colorCounts["White"]++
				case "U":
					colorCounts["Blue"]++
				case "B":
					colorCounts["Black"]++
				case "R":
					colorCounts["Red"]++
				case "G":
					colorCounts["Green"]++
				}
			}
		}
		if len(c.Colors) > 1 {
			colorCounts["Multi"]++
		}
	}
	colorOrder := []string{"White", "Blue", "Black", "Red", "Green", "Colorless", "Multi"}
	colorFn := func(k string) lipgloss.Color {
		switch k {
		case "White":
			return lipgloss.Color("#fbf1c7")
		case "Blue":
			return gruvBlue
		case "Black":
			return gruvGray
		case "Red":
			return gruvRed
		case "Green":
			return gruvGreen
		case "Multi":
			return gruvYellow
		default:
			return gruvFgDim
		}
	}
	b.WriteString(renderHistogram("Color", colorCounts, colorOrder, maxW, globalLabelW, colorFn))
	b.WriteString("\n")

	// ── Rarity Distribution ──
	rarityCounts := map[string]int{}
	for _, c := range cards {
		r := c.Rarity
		if r == "" {
			r = "unknown"
		}
		rarityCounts[r]++
	}
	rarityOrder := []string{"common", "uncommon", "rare", "mythic", "special", "bonus"}
	rarityFn := func(k string) lipgloss.Color {
		switch k {
		case "common":
			return gruvFg
		case "uncommon":
			return gruvFgDim
		case "rare":
			return gruvYellow
		case "mythic":
			return gruvOrange
		case "special":
			return gruvPurple
		default:
			return gruvGray
		}
	}
	b.WriteString(renderHistogram("Rarity", rarityCounts, rarityOrder, maxW, globalLabelW, rarityFn))
	b.WriteString("\n")

	// ── CMC Distribution ──
	cmcCounts := map[string]int{}
	for _, c := range cards {
		cmc := int(c.CMC)
		key := strconv.Itoa(cmc)
		if cmc >= 7 {
			key = "7+"
		}
		cmcCounts[key]++
	}
	cmcOrder := []string{"0", "1", "2", "3", "4", "5", "6", "7+"}
	cmcFn := func(k string) lipgloss.Color {
		return gruvAqua
	}
	b.WriteString(renderHistogram("CMC (Mana Value)", cmcCounts, cmcOrder, maxW, globalLabelW, cmcFn))
	b.WriteString("\n")

	// ── Type Distribution ──
	typeCounts := map[string]int{}
	typeOrder := []string{"Creature", "Instant", "Sorcery", "Artifact", "Enchantment", "Planeswalker", "Land"}
	for _, c := range cards {
		for _, t := range typeOrder {
			if strings.Contains(c.TypeLine, t) {
				typeCounts[t]++
			}
		}
	}
	typeFn := func(k string) lipgloss.Color {
		return gruvPurple
	}
	b.WriteString(renderHistogram("Type", typeCounts, typeOrder, maxW, globalLabelW, typeFn))

	return b.String()
}

// ── Main ────────────────────────────────────────────────────────

// printCardStdout renders a single card for the terminal, with the same
// highlighting the TUI uses. Lipgloss drops the colour automatically when
// stdout isn't a terminal, so piping still yields plain text.
func printCardStdout(c ScryfallCard) {
	rules := loadCachedRules()
	width := stdoutWidth()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	name := lipgloss.NewStyle().Bold(true).Foreground(colorForCard(c.Colors)).Render(c.Name)
	if c.ManaCost != "" {
		name += "  " + renderMana(c.ManaCost)
	}
	fmt.Println(name)
	fmt.Println(lipgloss.NewStyle().Foreground(gruvFgDim).Render(c.TypeLine))

	if c.Power != "" && c.Toughness != "" {
		fmt.Println(lipgloss.NewStyle().Foreground(gruvPurple).
			Render(fmt.Sprintf("P/T: %s/%s", c.Power, c.Toughness)))
	}
	if c.Loyalty != "" {
		fmt.Println(lipgloss.NewStyle().Foreground(gruvPurple).
			Render(fmt.Sprintf("Loyalty: %s", c.Loyalty)))
	}

	fmt.Println(dimStyle.Render(fmt.Sprintf("%s · %s · CMC %.0f", c.SetName, c.Rarity, c.CMC)))
	if c.EDHRECRank > 0 {
		fmt.Println(dimStyle.Render(fmt.Sprintf("EDHREC Rank: #%d", c.EDHRECRank)))
	}

	if c.OracleText != "" {
		fmt.Println()
		fmt.Println(headerStyle.Render("Oracle Text"))
		fmt.Println(highlightOracle(c, width, rules))
	}

	fmt.Println()
	printRulingsStdout(c, width, rules)

	if len(c.Legalities) > 0 {
		fmt.Println()
		fmt.Println(headerStyle.Render("Legalities"))
		formats := []string{"standard", "pioneer", "modern", "legacy", "vintage", "commander", "pauper"}
		for _, f := range formats {
			v, ok := c.Legalities[f]
			if !ok {
				continue
			}
			style := lipgloss.NewStyle().Foreground(gruvRed)
			mark := "✘"
			if v == "legal" {
				style = lipgloss.NewStyle().Foreground(gruvGreen)
				mark = "✔"
			}
			fmt.Printf("  %s %s\n", style.Render(mark), style.Render(f))
		}
	}
}

func printRulingsStdout(c ScryfallCard, width int, rules RulesData) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	rulings, err := getRulings(c.RulingsURI)
	switch {
	case err != nil:
		fmt.Println(headerStyle.Render("Rulings"))
		fmt.Println(lipgloss.NewStyle().Foreground(gruvRed).
			Render(fmt.Sprintf("  unavailable: %v", err)))
		return
	case len(rulings) == 0:
		fmt.Println(headerStyle.Render("Rulings"))
		fmt.Println(dimStyle.Render("  none"))
		return
	}

	fmt.Println(headerStyle.Render(fmt.Sprintf("Rulings (%d)", len(rulings))))
	bullet := lipgloss.NewStyle().Foreground(gruvOrange)
	for _, r := range rulings {
		body := highlightRuleText(r.Comment, width-4, rules)
		fmt.Println(indent(hangingBlock("•", bullet, body, 2), 2))
	}
}

// loadCachedRules parses the comprehensive rules only when they're already
// on disk — a one-shot lookup shouldn't stall on a download. Without them
// the highlighting simply skips keywords.
func loadCachedRules() RulesData {
	if _, err := os.Stat(rulesFilePath()); err != nil {
		return RulesData{}
	}
	data, err := loadRules()
	if err != nil {
		return RulesData{}
	}
	return data
}

func stdoutWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		w = 80
	}
	w -= 2
	if w > 100 {
		w = 100 // long lines are hard to read however wide the terminal is
	}
	if w < 30 {
		w = 30
	}
	return w
}

func main() {
	m := initialModel()

	// `scry rules [update | query…]` — the old mtg-rules entry point.
	if len(os.Args) > 1 && os.Args[1] == "rules" {
		runRules(m, os.Args[2:])
		return
	}

	if len(os.Args) > 1 {
		query := strings.Join(os.Args[1:], " ")

		// Check if single result — print to stdout and exit
		u := fmt.Sprintf("https://api.scryfall.com/cards/search?q=%s&order=%s",
			url.QueryEscape(query), url.QueryEscape(sortOptions[m.sortIndex]))
		body, err := doGet(u)
		if err != nil {
			if _, ok := err.(notFoundError); ok {
				fmt.Println("No results found.")
				return
			}
		} else {
			var sr ScryfallResponse
			if json.Unmarshal(body, &sr) == nil && sr.TotalCards == 1 {
				printCardStdout(sr.Data[0])
				return
			}
		}

		m.searchInput.SetValue(query)
		m.initialQuery = query
		m.searching = true
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// runRules opens the comprehensive-rules browser directly.
func runRules(m model, args []string) {
	if len(args) > 0 && args[0] == "update" {
		fmt.Println("Downloading latest comprehensive rules...")
		if err := downloadRules(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Done! Saved to", rulesFilePath())
		return
	}

	if _, err := os.Stat(rulesFilePath()); os.IsNotExist(err) {
		fmt.Println("Downloading comprehensive rules...")
	}

	data, err := loadRules()
	if err != nil {
		fmt.Printf("Error loading rules: %v\n", err)
		os.Exit(1)
	}

	m.rules = data
	m.state = stateRules
	m.prevState = stateResults

	items := buildRuleItems(data)
	if len(args) > 0 {
		items = searchRules(data, strings.Join(args, " "))
		m.rulesList.Title = fmt.Sprintf("Results (%d)", len(items))
	} else {
		m.rulesList.Title = fmt.Sprintf("Rules (%d)", len(items))
	}
	m.rulesList.SetItems(items)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
