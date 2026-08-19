package main

import (
	"scry/internal/deck"
	"scry/internal/mtg"

	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// A card, end to end: the shape Scryfall sends it in, the list row it turns
// into, and the panel block it renders as. Nothing here knows about search
// or decks — both feed the same ScryfallCard through the same renderers.

// ── Single-line delegate ────────────────────────────────────────

// compactDelegate draws one card per line.
//
//   - blurred is set on whichever list doesn't have focus.
//   - marks are the cards picked out for tagging.
//   - inOther says which cards the list beside this one is showing, so a
//     search result you already run — or a deck card your search just
//     turned up — is obvious without reading across.
type compactDelegate struct {
	blurred bool
	marks   map[string]bool
	inOther map[string]bool
}

// gutterWidth is the two columns in front of every row: one for the tagging
// mark, one for the card's standing in the deck. Both are always drawn, so
// rows never shift sideways underneath you.
const gutterWidth = 2

// gutter is those two columns for one card.
func (d compactDelegate) gutter(it cardItem) string {
	key := strings.ToLower(it.Card.Name)

	mark := " "
	if d.marks[key] {
		mark = lipgloss.NewStyle().Foreground(gruvGreen).Bold(true).Render("●")
	}

	role := " "
	switch {
	case it.Commander:
		role = lipgloss.NewStyle().Foreground(gruvYellow).Bold(true).Render("★")
	case d.inOther[key]:
		role = lipgloss.NewStyle().Foreground(gruvAqua).Render("▪")
	}
	return mark + role
}

func (d compactDelegate) Height() int                             { return 1 }
func (d compactDelegate) Spacing() int                            { return 0 }
func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// listColumns divides the list's width between the three columns. Mana gets
// a fixed share, wide enough for most costs, and the name and type line
// share what's left — name first, since that's what you're scanning for.
func listColumns(total int) (nameW, manaW, typeW int) {
	// Six is enough for almost every cost once the braces are stripped —
	// {3}{W}{U} is three characters wide. The handful that run longer are
	// truncated rather than made everything else sit behind a gap.
	manaW = 6
	// The gutter, 2 for the cursor, and 2 between each pair of columns.
	rest := total - gutterWidth - 2 - manaW - 4
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
	c := ci.Card

	// With two lists on screen only one of them is taking keys. The inactive
	// one drops to a single dim colour and loses its cursor highlight, so
	// which list you're driving is obvious at a glance rather than a matter
	// of noticing the colour of a rule between columns.
	if d.blurred {
		name := c.Name
		if ci.Qty > 1 {
			name = fmt.Sprintf("%dx %s", ci.Qty, name)
		}
		nameW, manaW, typeW := listColumns(m.Width())
		dim := lipgloss.NewStyle().Foreground(gruvGray)

		mana, manaLen := renderManaWidth(c.DisplayManaCost(), manaW)
		mana = stripStyle(mana)
		if manaLen == 0 {
			mana, manaLen = "·", 1
		}
		pad := manaW - manaLen
		if pad < 0 {
			pad = 0
		}

		// A hollow marker, so the two lists still read differently where
		// colour alone won't carry it — a mono terminal, or a screenshot.
		cursor := "  "
		if selected {
			cursor = "▹ "
		}
		fmt.Fprint(w, d.gutter(ci)+dim.Render(cursor+
			padTo(truncate(name, nameW), nameW)+"  "+
			mana+strings.Repeat(" ", pad)+"  "+
			truncate(c.TypeLine, typeW)))
		return
	}

	nameW, manaW, typeW := listColumns(m.Width())

	// A deck's repeat cards carry their count in the name column, so the
	// columns stay where search results put them.
	name := c.Name
	if ci.Qty > 1 {
		name = fmt.Sprintf("%dx %s", ci.Qty, name)
	}
	name = truncate(name, nameW)
	typeLine := truncate(c.TypeLine, typeW)

	nameStyle := lipgloss.NewStyle().
		Width(nameW).
		Foreground(colorForCard(c.DisplayColors()))
	typeStyle := lipgloss.NewStyle().
		Width(typeW).
		Foreground(gruvFgDim)

	renderedMana, manaLen := renderManaWidth(c.DisplayManaCost(), manaW)
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

	fmt.Fprint(w, d.gutter(ci)+line)
}

// ── List item adapter ───────────────────────────────────────────

// cardItem is a card as a row in a list.
//
// Field for field this is deck.Card, and the two ought to be one type. They
// can't quite be yet: embedding deck.Card would put its own Card field a
// level down, and every reader of a row would have to say so. The list item
// is being redesigned when the panel workspace lands, which is the moment to
// merge them.
type cardItem struct {
	// Commander is set on a deck's own cards, so the row can be flagged.
	// It has no bearing on sorting — a commander sorts by mana value like
	// anything else.
	Commander bool
	Card      mtg.Card
	// Qty is how many copies a deck runs; zero for search results.
	Qty int
	// Tags are the deck author's own, and only ever set for deck cards.
	Tags []string
}

// deckCard is cardItem as the domain sees it. The two are field for field
// the same; this is the seam until they're merged.
func (c cardItem) deckCard() deck.Card {
	return deck.Card{Card: c.Card, Qty: c.Qty, Tags: c.Tags, Commander: c.Commander}
}

func (c cardItem) Title() string       { return c.Card.Name }
func (c cardItem) Description() string { return c.Card.TypeLine }
func (c cardItem) FilterValue() string {
	return c.Card.Name + " " + c.Card.CombinedOracle()
}

// ── Color helpers ───────────────────────────────────────────────

func colorForCard(colors []string) lipgloss.Color {
	if len(colors) > 1 {
		return gruvYellow // multicolor
	}
	if len(colors) == 1 {
		switch colors[0] {
		case "W":
			return gruvWhite
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
		"W": gruvWhite,
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
	faces := c.Faces()

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
	front := c.Faces()[0]

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
