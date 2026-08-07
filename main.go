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
	// Sent to Scryfall, Moxfield, MTGJSON and Wizards alike.
	userAgent = "scry/2.0"

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

	// Transforming and modal double-faced cards carry no top-level oracle
	// text, mana cost or colors at all — it's per face.
	CardFaces []CardFace `json:"card_faces"`
}

type CardFace struct {
	Name       string   `json:"name"`
	ManaCost   string   `json:"mana_cost"`
	TypeLine   string   `json:"type_line"`
	OracleText string   `json:"oracle_text"`
	Colors     []string `json:"colors"`
	Power      string   `json:"power"`
	Toughness  string   `json:"toughness"`
	Loyalty    string   `json:"loyalty"`
}

// faces returns a card's printed faces as cards in their own right, so
// anything that renders a card can work a face at a time. A single-faced
// card comes back as itself.
func (c ScryfallCard) faces() []ScryfallCard {
	if len(c.CardFaces) < 2 {
		return []ScryfallCard{c}
	}
	out := make([]ScryfallCard, 0, len(c.CardFaces))
	for _, f := range c.CardFaces {
		fc := c
		fc.CardFaces = nil
		fc.Name = f.Name
		fc.ManaCost = f.ManaCost
		fc.TypeLine = f.TypeLine
		fc.OracleText = f.OracleText
		fc.Power = f.Power
		fc.Toughness = f.Toughness
		fc.Loyalty = f.Loyalty
		if len(f.Colors) > 0 {
			fc.Colors = f.Colors
		}
		out = append(out, fc)
	}
	return out
}

// combinedOracle is every face's text at once, for the places that match
// against a card's wording rather than display it.
func (c ScryfallCard) combinedOracle() string {
	if len(c.CardFaces) < 2 {
		return c.OracleText
	}
	var parts []string
	for _, f := range c.CardFaces {
		if f.OracleText != "" {
			parts = append(parts, f.OracleText)
		}
	}
	return strings.Join(parts, "\n")
}

// displayColors and displayManaCost fall back to the front face, which is
// where a transforming card keeps them.
func (c ScryfallCard) displayColors() []string {
	if len(c.Colors) > 0 || len(c.CardFaces) == 0 {
		return c.Colors
	}
	return c.CardFaces[0].Colors
}

func (c ScryfallCard) displayManaCost() string {
	if c.ManaCost != "" || len(c.CardFaces) == 0 {
		return c.ManaCost
	}
	return c.CardFaces[0].ManaCost
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

// listColumns divides the list's width between the three columns. Mana gets
// a fixed share, wide enough for most costs, and the name and type line
// share what's left — name first, since that's what you're scanning for.
func listColumns(total int) (nameW, manaW, typeW int) {
	manaW = 10
	// 2 columns for the cursor, and 2 between each pair of columns.
	rest := total - 2 - manaW - 4
	if rest < 18 {
		rest = 18
	}
	nameW = rest * 55 / 100
	typeW = rest - nameW
	if nameW < 10 {
		nameW = 10
	}
	if typeW < 8 {
		typeW = 8
	}
	return nameW, manaW, typeW
}

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ci, ok := item.(cardItem)
	if !ok {
		return
	}

	selected := index == m.Index()
	c := ci.card

	nameW, manaW, typeW := listColumns(m.Width())

	// A deck's repeat cards carry their count in the name column, so the
	// columns stay where search results put them.
	name := c.Name
	if ci.qty > 1 {
		name = fmt.Sprintf("%dx %s", ci.qty, name)
	}
	name = truncate(name, nameW)
	typeLine := truncate(c.TypeLine, typeW)

	nameStyle := lipgloss.NewStyle().
		Width(nameW).
		Foreground(colorForCard(c.displayColors()))
	typeStyle := lipgloss.NewStyle().
		Width(typeW).
		Foreground(gruvFgDim)

	renderedMana, manaLen := renderManaWidth(c.displayManaCost(), manaW)
	if manaLen == 0 {
		renderedMana = lipgloss.NewStyle().Foreground(gruvFgDim).Render("·")
		manaLen = 1
	}
	pad := manaW - manaLen
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

// truncate cuts to a rune count, not a byte count — an em dash in a type
// line is three bytes, and slicing through one renders as a replacement
// character.
func truncate(s string, maxLen int) string {
	if runeLen(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}

// ── List item adapter ───────────────────────────────────────────

type cardItem struct {
	card ScryfallCard
	// qty is how many copies a deck runs; zero for search results.
	qty int
	// tags are the deck author's own, and only ever set for deck cards.
	tags []string
}

func (c cardItem) Title() string       { return c.card.Name }
func (c cardItem) Description() string { return c.card.TypeLine }
func (c cardItem) FilterValue() string {
	return c.card.Name + " " + c.card.combinedOracle()
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
	s, _ := renderManaWidth(manaCost, 0)
	return s
}

// renderManaWidth colours a mana cost symbol by symbol. A limit above zero
// stops it there, so a long hybrid cost like {R/W}{R/W}{R/W}{R/W} can't push
// the columns beside it out of line; it reports the width it actually used,
// since the styling makes that impossible to measure afterwards.
func renderManaWidth(manaCost string, limit int) (string, int) {
	if manaCost == "" {
		return "", 0
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
	used := 0
	for _, s := range symbols {
		if s == "" {
			continue
		}
		// Cut between symbols, never through one — half a hybrid symbol
		// reads as a different cost entirely.
		if limit > 0 && used+runeLen(s) > limit {
			result.WriteString(lipgloss.NewStyle().Foreground(gruvFgDim).Render("…"))
			used++
			break
		}
		if col, ok := colorMap[s]; ok {
			result.WriteString(lipgloss.NewStyle().Foreground(col).Render(s))
		} else {
			// Generic/numeric mana
			result.WriteString(lipgloss.NewStyle().Foreground(gruvFgDim).Render(s))
		}
		used += runeLen(s)
	}
	return result.String(), used
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

	// A loaded Moxfield deck replaces the search results with the deck's
	// cards; nil whenever the list is holding search results instead.
	deck        *deckInfo
	deckLoading bool
	initialDeck string
	// notice is a one-off confirmation ("saved as …") shown beside the
	// counts until the next keypress.
	notice string

	// The result list before the statistics panel narrows it, and which
	// category it's narrowed to. statIndex is -1 when the panel's category
	// list hasn't been entered.
	baseItems  []list.Item
	statIndex  int
	statFilter *statRow

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

DECKS
  Paste a Moxfield deck URL into the search bar to load that deck
  instead of running a search — moxfield.com/decks/<id>. The deck is
  listed in decklist order, and r / s / t / enter work on its cards
  as they do on search results. The author's own card tags, if the
  deck has any, are broken down under s.

  w                       Save the deck you're looking at

  From the shell:
    scry deck <name>      A deck you've saved
    scry deck <id>        The id out of the deck's URL
    scry <moxfield url>   Same thing, pasted whole
    scry deck list        The decks you've saved
    scry deck save <name> <id|url>
    scry deck rm <name>

KEYS — RESULTS
  j/k, ↑/↓                Move through cards
  /                       Filter the results (name + oracle text)
  i                       Edit the search query
  esc                     Clear the filter, or quit if there isn't one
  J/K, shift+↑/↓          Scroll the panel — in the statistics panel,
                          walk the categories and filter the cards to
                          whichever one the cursor is on
  ctrl+d / ctrl+u         Scroll the panel half a screen; in the
                          statistics panel this moves the view without
                          moving the selected category
  r                       Rules for this card (again for the card view)
  s                       Statistics for these results (again for the card).
                          J/K there filters to a colour, rarity, mana
                          value, type or — for a deck — an author's tag,
                          and the histograms redraw for the cards that
                          category leaves on screen
  t                       Printed text history — how the card's wording
                          changed across printings (again for the card view)
  w                       Save the current deck (decks only)
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
	req.Header.Set("User-Agent", userAgent)
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
		return nil, fmt.Errorf("%s (%d): %s", hostOf(u), resp.StatusCode, string(body))
	}
	return body, nil
}

// hostOf labels an error with the service that produced it — cards and
// rulings come from Scryfall, decks from Moxfield.
func hostOf(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return "request"
	}
	return strings.TrimPrefix(parsed.Host, "api.")
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
		statIndex:     -1,
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
	if m.initialDeck != "" {
		cmds = append(cmds, loadDeckCmd(m.initialDeck))
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
		m.deck = nil
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
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		m.err = nil
		info := msg.info
		m.deck = &info
		m.cards = make([]ScryfallCard, 0, len(msg.cards))
		for _, dc := range msg.cards {
			m.cards = append(m.cards, dc.card)
		}
		m.totalCards = info.total

		m = m.setResults(deckItems(msg.cards))

		m.searchInput.SetValue(info.url)
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

// saveCurrentDeck files the deck on screen under a name made from its title,
// so it can be reopened with `scry deck <name>`.
func (m model) saveCurrentDeck() model {
	if m.deck == nil {
		m.notice = "nothing to save — open a deck first"
		return m
	}

	alias := slugify(m.deck.name)
	if alias == "" {
		alias = m.deck.id
	}
	err := saveDeck(alias, savedDeck{Name: m.deck.name, ID: m.deck.id, URL: m.deck.url})
	if err != nil {
		m.notice = fmt.Sprintf("could not save: %v", err)
		return m
	}
	m.notice = fmt.Sprintf("saved · scry deck %s", alias)
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

// faceHeading is a face's name, cost, type line and P/T — the block that
// identifies which side of a card you're looking at.
func faceHeading(f ScryfallCard, nameStyle lipgloss.Style) string {
	var b strings.Builder

	line := nameStyle.Render(f.Name)
	if f.ManaCost != "" {
		line += "  " + renderMana(f.ManaCost)
	}
	b.WriteString(line + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(gruvFgDim).Render(f.TypeLine) + "\n")

	if f.Power != "" && f.Toughness != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(gruvPurple).
			Render(fmt.Sprintf("P/T: %s/%s", f.Power, f.Toughness)) + "\n")
	}
	if f.Loyalty != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(gruvPurple).
			Render(fmt.Sprintf("Loyalty: %s", f.Loyalty)) + "\n")
	}
	return b.String()
}

// oracleBlock is a card's rules text: one block for an ordinary card, and
// one per face for a double-faced one, each labelled with the face it
// belongs to, since Scryfall keeps no text on the card itself.
func oracleBlock(c ScryfallCard, width int, rules RulesData) string {
	faces := c.faces()

	var b strings.Builder
	for i, f := range faces {
		if i > 0 {
			// The back face needs its own heading — its name, cost and
			// P/T are all different from the front's.
			b.WriteString("\n")
			b.WriteString(faceHeading(f, lipgloss.NewStyle().Bold(true).Foreground(colorForCard(f.Colors))))
		}
		if f.OracleText != "" {
			b.WriteString(highlightOracle(f, width, rules) + "\n")
		}
	}
	return b.String()
}

func (m model) renderPreview(c ScryfallCard, maxW int) string {
	if c.Name == "" {
		return lipgloss.NewStyle().Foreground(gruvGray).Render("No card selected")
	}
	if maxW < 24 {
		maxW = 24
	}

	// A double-faced card's name, cost and P/T live on its faces; the front
	// one stands in for the card at the top of the panel.
	front := c.faces()[0]

	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorForCard(front.Colors))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua).MarginTop(1)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	var b strings.Builder

	b.WriteString(faceHeading(front, nameStyle))
	b.WriteString(dimStyle.Render(fmt.Sprintf("%s · %s · CMC %.0f", c.SetName, c.Rarity, c.CMC)) + "\n")

	if c.EDHRECRank > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("EDHREC Rank: #%d", c.EDHRECRank)) + "\n")
	}

	if body := oracleBlock(c, maxW, m.rules); body != "" {
		b.WriteString(headerStyle.Render("Oracle Text") + "\n")
		b.WriteString(body)
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

// ── Main ────────────────────────────────────────────────────────

// printCardStdout renders a single card for the terminal, with the same
// highlighting the TUI uses. Lipgloss drops the colour automatically when
// stdout isn't a terminal, so piping still yields plain text.
func printCardStdout(c ScryfallCard) {
	rules := loadCachedRules()
	width := stdoutWidth()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	front := c.faces()[0]
	fmt.Print(faceHeading(front, lipgloss.NewStyle().Bold(true).Foreground(colorForCard(front.Colors))))

	fmt.Println(dimStyle.Render(fmt.Sprintf("%s · %s · CMC %.0f", c.SetName, c.Rarity, c.CMC)))
	if c.EDHRECRank > 0 {
		fmt.Println(dimStyle.Render(fmt.Sprintf("EDHREC Rank: #%d", c.EDHRECRank)))
	}

	if body := oracleBlock(c, width, rules); body != "" {
		fmt.Println()
		fmt.Println(headerStyle.Render("Oracle Text"))
		fmt.Print(body)
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

	// `scry deck <id | url>` — the id out of a deck's public URL is enough.
	if len(os.Args) > 1 && os.Args[1] == "deck" {
		runDeck(m, os.Args[2:])
		return
	}

	if len(os.Args) > 1 {
		query := strings.Join(os.Args[1:], " ")

		// A Moxfield link passed straight in opens that deck.
		if id, ok := moxfieldURLID(query); ok {
			runDeck(m, []string{id})
			return
		}

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

const deckUsage = `Usage:
  scry deck <name | id | url>     Open a saved deck, or one off Moxfield
  scry deck list                  List the decks you've saved
  scry deck save <name> <id|url>  Save a deck under a short name
  scry deck rm <name>             Forget a saved deck

Inside the app, w saves the deck you're looking at.`

// runDeck opens a Moxfield deck directly, or handles one of the subcommands
// for the saved-deck list. The reference is checked here so a typo fails on
// the terminal; the deck itself loads once the TUI is up, which keeps the
// "loading deck…" line on screen while it does.
func runDeck(m model, args []string) {
	if len(args) == 0 {
		fmt.Println(deckUsage)
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		runDeckList()
		return
	case "save":
		runDeckSave(args[1:])
		return
	case "rm", "remove", "forget":
		runDeckForget(args[1:])
		return
	}

	arg := strings.Join(args, " ")

	// A saved name wins over reading the same text as a deck id, so saving
	// a deck as "ghen" doesn't collide with anything on Moxfield.
	id := ""
	if saved, ok := lookupSavedDeck(arg); ok {
		id = saved.ID
	} else {
		var ok bool
		if id, ok = deckRef(arg); !ok {
			fmt.Println("Not a saved deck, a Moxfield id, or a Moxfield URL:", arg)
			os.Exit(1)
		}
	}

	m.searching = true
	m.deckLoading = true
	m.initialDeck = id
	m.searchInput.SetValue("https://moxfield.com/decks/" + id)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func runDeckList() {
	decks, err := loadSavedDecks()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if len(decks) == 0 {
		fmt.Println("No saved decks yet. Save one with:")
		fmt.Println("  scry deck save <name> <moxfield url>")
		return
	}

	aliasStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvYellow)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	aliases := aliasesSorted(decks)
	width := 0
	for _, a := range aliases {
		if len(a) > width {
			width = len(a)
		}
	}
	for _, a := range aliases {
		d := decks[a]
		fmt.Printf("%s  %s\n",
			aliasStyle.Render(a+strings.Repeat(" ", width-len(a))),
			d.Name)
		fmt.Printf("%s  %s\n", strings.Repeat(" ", width), dimStyle.Render(d.URL))
	}
}

func runDeckSave(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: scry deck save <name> <deck-id | moxfield url>")
		os.Exit(1)
	}

	alias := args[0]
	id, ok := deckRef(strings.Join(args[1:], " "))
	if !ok {
		fmt.Println("Not a Moxfield deck id or URL:", strings.Join(args[1:], " "))
		os.Exit(1)
	}

	// Fetch it once before saving, so a bad id fails now rather than the
	// next time the deck is opened — and so the deck's real title is stored.
	fmt.Println("Checking deck…")
	info, _, err := loadDeck(id)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if err := saveDeck(alias, savedDeck{Name: info.name, ID: id, URL: info.url}); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved %q as %s — open it with: scry deck %s\n", info.name, alias, alias)
}

func runDeckForget(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: scry deck rm <name>")
		os.Exit(1)
	}

	found, err := forgetDeck(args[0])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if !found {
		fmt.Printf("No saved deck called %q.\n", args[0])
		os.Exit(1)
	}
	fmt.Printf("Forgot %s.\n", args[0])
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
