package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateHelp: the scrollable Scryfall syntax reference behind `?`.

// ── Syntax reference ────────────────────────────────────────────
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
