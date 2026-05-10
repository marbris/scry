package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

const compactWidth = 120

// ── Scryfall types ──────────────────────────────────────────────

type ScryfallResponse struct {
	Data       []ScryfallCard `json:"data"`
	TotalCards int            `json:"total_cards"`
	HasMore    bool           `json:"has_more"`
	NextPage   string         `json:"next_page"`
}

type ScryfallCard struct {
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
			return gruvGray
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
		"B": gruvGray,
		"R": gruvRed,
		"G": gruvGreen,
		"C": gruvFgDim,
		"X": gruvYellow,
		"S": gruvPurple,
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
	stateSearch state = iota
	stateResults
	stateDetail
	stateHelp
)

type model struct {
	state       state
	searchInput textinput.Model
	sortIndex   int
	maxResults  int
	resultList  list.Model
	cards       []ScryfallCard
	totalCards  int
	selected    *ScryfallCard
	rulings     []Ruling
	err         error
	width       int
	height      int
	detailScroll int
	initialQuery string
	helpScroll int
}

// ── Messages ────────────────────────────────────────────────────

type searchResultMsg struct {
	cards      []ScryfallCard
	totalCards int
	err        error
}

type rulingsMsg struct {
	rulings []Ruling
	err     error
}

// ── helpScroll ────────────────────────────────────────────────────
func syntaxHelp() string {
	return `╔══════════════════════════════════════════════════════════════╗
║                    Scryfall Syntax Guide                     ║
╚══════════════════════════════════════════════════════════════╝

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
			m.state = stateSearch
			m.searchInput.Focus()
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
	req.Header.Set("User-Agent", "scryfall-tui/1.0")
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

func searchScryfall(query string, sort string, maxResults int) tea.Cmd {
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

			if len(allCards) >= maxResults || !sr.HasMore {
				if len(allCards) > maxResults {
					allCards = allCards[:maxResults]
				}
				return searchResultMsg{cards: allCards, totalCards: sr.TotalCards}
			}
			page++
		}

		if len(allCards) > maxResults {
			allCards = allCards[:maxResults]
		}
		return searchResultMsg{cards: allCards, totalCards: len(allCards)}
	}
}

func fetchRulings(uri string) tea.Cmd {
	return func() tea.Msg {
		if uri == "" {
			return rulingsMsg{}
		}
		body, err := doGet(uri)
		if err != nil {
			return rulingsMsg{err: err}
		}
		var rr RulingsResponse
		json.Unmarshal(body, &rr)
		return rulingsMsg{rulings: rr.Data}
	}
}

// ── Init ────────────────────────────────────────────────────────

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "e.g. t:creature c:R cmc<=3 otag:removal"
	ti.Focus()
	ti.Width = 60
	ti.TextStyle = lipgloss.NewStyle().Foreground(gruvFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(gruvGray)
	ti.PromptStyle = lipgloss.NewStyle().Foreground(gruvOrange)

	l := list.New([]list.Item{}, compactDelegate{}, 40, 30)
	l.Title = "Results"
	l.Styles.Title = lipgloss.NewStyle().
			Bold(true).
			Foreground(gruvOrange).
			Background(gruvBgLight).
			Padding(0, 1).
			MarginBottom(1)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(gruvYellow)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(gruvOrange)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

	return model{
		state:       stateSearch,
		searchInput: ti,
		resultList:  l,
		sortIndex:   9,
		maxResults:  50,
	}
}

func (m model) Init() tea.Cmd {
	if m.initialQuery != "" {
		return tea.Batch(
			textinput.Blink,
			searchScryfall(m.initialQuery, sortOptions[m.sortIndex], m.maxResults),
		)
	}
	return textinput.Blink
}

// ── Update ──────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width < compactWidth {
			m.resultList.SetSize(msg.Width, msg.Height/2)
		} else {
			m.resultList.SetSize(msg.Width/2, msg.Height-2)
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	switch m.state {
	case stateSearch:
		return m.updateSearch(msg)
	case stateResults:
		return m.updateResults(msg)
	case stateDetail:
		return m.updateDetail(msg)
	case stateHelp:
		return m.updateHelp(msg)
	}
	return m, nil
}

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			query := strings.TrimSpace(m.searchInput.Value())
			if query != "" {
				m.err = nil
				return m, searchScryfall(query, sortOptions[m.sortIndex], m.maxResults)
			}
			return m, nil
		case "esc":
			return m, tea.Quit
		case "tab":
			m.sortIndex = (m.sortIndex + 1) % len(sortOptions)
			return m, nil
		case "shift+tab":
			m.sortIndex = (m.sortIndex - 1 + len(sortOptions)) % len(sortOptions)
			return m, nil
		case "ctrl+up":
			m.maxResults += 25
			if m.maxResults > 175 {
				m.maxResults = 175
			}
			return m, nil
		case "ctrl+down":
			m.maxResults -= 25
			if m.maxResults < 25 {
				m.maxResults = 25
			}
			return m, nil
		case "?":
			m.helpScroll = 0
			m.state = stateHelp
			return m, nil
		}
	case searchResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if len(msg.cards) == 0 {
			m.err = fmt.Errorf("no results found")
			return m, nil
		}
		m.cards = msg.cards
		m.totalCards = msg.totalCards
		items := make([]list.Item, len(msg.cards))
		for i, c := range msg.cards {
			items[i] = cardItem{card: c}
		}
		m.resultList.SetItems(items)
		m.resultList.ResetSelected()
m.resultList.Title = fmt.Sprintf(
    "Results (%d/%d) · q: %s · sort: %s · max: %d",
    len(msg.cards),
    msg.totalCards,
    m.searchInput.Value(),
    sortOptions[m.sortIndex],
    m.maxResults,
)
		m.state = stateResults
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m model) updateResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.resultList.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "esc":
			m.state = stateSearch
			m.searchInput.Focus()
			return m, nil
		case "enter":
			if item, ok := m.resultList.SelectedItem().(cardItem); ok {
				m.selected = &item.card
				m.rulings = nil
				m.detailScroll = 0
				m.state = stateDetail
				return m, fetchRulings(item.card.RulingsURI)
			}
		}
	}

	var cmd tea.Cmd
	m.resultList, cmd = m.resultList.Update(msg)
	return m, cmd
}

func (m model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.state = stateResults
			return m, nil
		case "up", "k":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
			return m, nil
		case "down", "j":
			m.detailScroll++
			return m, nil
		}
	case rulingsMsg:
		if msg.rulings == nil {
			m.rulings = []Ruling{}
		} else {
			m.rulings = msg.rulings
		}
		return m, nil
	}
	return m, nil
}

// ── View ────────────────────────────────────────────────────────

func (m model) View() string {
	base := lipgloss.NewStyle().
		Background(gruvBg).
		Foreground(gruvFg).
		Width(m.width).
		Height(m.height)

	var content string
	switch m.state {
	case stateSearch:
		content = m.viewSearch()
	case stateResults:
		content = m.viewResults()
	case stateDetail:
		content = m.viewDetail()
	case stateHelp:
		content = m.viewHelp()
	}

	return base.Render(content)
}

func (m model) viewSearch() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(gruvOrange).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(gruvAqua).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(gruvYellow)

	hintStyle := lipgloss.NewStyle().
		Foreground(gruvGray)

	s := titleStyle.Render("⚔  scryfall-tui") + "\n\n"
	s += labelStyle.Render("Search") + " " +
		hintStyle.Render("(full scryfall syntax)") + "\n"
	s += m.searchInput.View() + "\n\n"

	s += labelStyle.Render("Sort: ") +
		valueStyle.Render(sortOptions[m.sortIndex]) +
		hintStyle.Render("  tab/shift+tab to cycle") + "\n"

	s += labelStyle.Render("Max:  ") +
		valueStyle.Render(strconv.Itoa(m.maxResults)) +
		hintStyle.Render("  ctrl+up/down to adjust") + "\n\n"

	s += hintStyle.Render("enter: search  ?: syntax help  esc: quit")

	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(gruvRed).Bold(true)
		s += "\n\n" + errStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(s)
}

func (m model) viewResults() string {
	if m.width < compactWidth {
		// Vertical layout: list on top, preview below
		listH := m.height / 2
		m.resultList.SetSize(m.width, listH)

		listView := m.resultList.View()

		var preview string
		if item, ok := m.resultList.SelectedItem().(cardItem); ok {
			preview = m.renderPreview(item.card)
		}

		previewStyle := lipgloss.NewStyle().
			Width(m.width - 4).
			Height(m.height - listH - 2).
			Padding(0, 2).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(gruvGray)

		return lipgloss.JoinVertical(
			lipgloss.Left,
			listView,
			previewStyle.Render(preview),
		)
	}

	// Horizontal layout: list on left, preview on right
	listView := m.resultList.View()

	var preview string
	if item, ok := m.resultList.SelectedItem().(cardItem); ok {
		preview = m.renderPreview(item.card)
	}

	previewW := m.width/2 - 2
	if previewW < 30 {
		previewW = 30
	}

	previewStyle := lipgloss.NewStyle().
		Width(previewW).
		Height(m.height - 2).
		Padding(1, 2).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(gruvGray)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		listView,
		previewStyle.Render(preview),
	)
}

func (m model) renderPreview(c ScryfallCard) string {
	col := colorForCard(c.Colors)
	var maxW int
	if m.width < compactWidth {
		maxW = m.width - 6
	} else {
		maxW = m.width/2 - 6
	}
	if maxW < 30 {
		maxW = 30
	}

	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(col)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua).MarginTop(1)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)
	textStyle := lipgloss.NewStyle().Foreground(gruvFg).Width(maxW)

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
		b.WriteString(textStyle.Render(c.OracleText) + "\n")
	}

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

func (m model) viewDetail() string {
	if m.selected == nil {
		return "No card selected"
	}
	c := m.selected
	col := colorForCard(c.Colors)
	maxW := m.width - 6
	if maxW < 30 {
		maxW = 30
	}

	nameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(col)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(gruvAqua).
		MarginTop(1)

	dimStyle := lipgloss.NewStyle().
		Foreground(gruvGray)

	textStyle := lipgloss.NewStyle().
		Foreground(gruvFg).
		Width(maxW)

	var b strings.Builder

	// Name + mana
	nameLine := nameStyle.Render(c.Name)
	if c.ManaCost != "" {
		nameLine += "  " + renderMana(c.ManaCost)
	}
	b.WriteString(nameLine + "\n")

	// Type line
	b.WriteString(lipgloss.NewStyle().Foreground(gruvFgDim).Render(c.TypeLine) + "\n")

	// Stats
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

	// Oracle text
	if c.OracleText != "" {
		b.WriteString(headerStyle.Render("Oracle Text") + "\n")
		b.WriteString(textStyle.Render(c.OracleText) + "\n")
	}

	// Rulings
	if len(m.rulings) > 0 {
		b.WriteString(headerStyle.Render(fmt.Sprintf("Rulings (%d)", len(m.rulings))) + "\n")
		for _, r := range m.rulings {
			wrapped := textStyle.Render(fmt.Sprintf("• %s", r.Comment))
			b.WriteString(wrapped + "\n")
		}
	} else if m.rulings != nil {
		b.WriteString(dimStyle.Render("\nNo rulings.") + "\n")
	} else {
		b.WriteString(dimStyle.Render("\nLoading rulings...") + "\n")
	}

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

	b.WriteString("\n" + dimStyle.Render("esc/q: back  j/k: scroll"))

	// Apply scrolling
	lines := strings.Split(b.String(), "\n")
	viewH := m.height - 4
	if viewH < 5 {
		viewH = 5
	}
	if m.detailScroll > len(lines)-viewH {
		m.detailScroll = len(lines) - viewH
	}
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}

	end := m.detailScroll + viewH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[m.detailScroll:end]

	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(visible, "\n"))
}

// ── Main ────────────────────────────────────────────────────────

func printCardStdout(c ScryfallCard) {
	mana := strings.ReplaceAll(strings.ReplaceAll(c.ManaCost, "{", ""), "}", "")
	fmt.Printf("%s  %s\n", c.Name, mana)
	fmt.Printf("%s\n", c.TypeLine)

	if c.Power != "" && c.Toughness != "" {
		fmt.Printf("P/T: %s/%s\n", c.Power, c.Toughness)
	}
	if c.Loyalty != "" {
		fmt.Printf("Loyalty: %s\n", c.Loyalty)
	}

	fmt.Printf("%s · %s · CMC %.0f\n", c.SetName, c.Rarity, c.CMC)

	if c.EDHRECRank > 0 {
		fmt.Printf("EDHREC Rank: #%d\n", c.EDHRECRank)
	}

	if c.OracleText != "" {
		fmt.Printf("\nOracle Text:\n%s\n", c.OracleText)
	}

	if len(c.Legalities) > 0 {
		fmt.Printf("\nLegalities:\n")
		formats := []string{"standard", "pioneer", "modern", "legacy", "vintage", "commander", "pauper"}
		for _, f := range formats {
			if v, ok := c.Legalities[f]; ok {
				mark := "✘"
				if v == "legal" {
					mark = "✔"
				}
				fmt.Printf("  %s %s\n", mark, f)
			}
		}
	}

	// Fetch and print rulings synchronously
	if c.RulingsURI != "" {
		body, err := doGet(c.RulingsURI)
		if err == nil {
			var rr RulingsResponse
			json.Unmarshal(body, &rr)
			if len(rr.Data) > 0 {
				fmt.Printf("\nRulings:\n")
				for _, r := range rr.Data {
					fmt.Printf("  • %s\n", r.Comment)
				}
			}
		}
	}
}

func main() {
	m := initialModel()

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
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
