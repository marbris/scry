package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"scry/internal/mtg"
	"scry/internal/rules"
	"scry/internal/scryfall"
	"scry/internal/theme"
)

// One card, printed and gone.
//
// A query that matches exactly one card almost never wanted a whole
// interface: you asked what the card does. So it goes to stdout with the
// same highlighting the panels use, and the program exits. Lipgloss drops
// the colour when stdout isn't a terminal, so piping still yields plain
// text.

// PrintCard writes a card to stdout. rd may be empty, in which case the text
// is simply less colourful.
func PrintCard(c mtg.Card, rd rules.Data) {
	width := stdoutWidth()
	head := lipgloss.NewStyle().Bold(true).Foreground(theme.Info)

	for i, f := range c.Faces() {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(faceHeading(f, width))
		fmt.Println(lipgloss.NewStyle().Foreground(theme.TextDim).Render(typeLine(f, width)))
		if f.OracleText != "" {
			fmt.Println()
			for _, line := range highlightOracle(f.OracleText, f, width, rd) {
				fmt.Println(line)
			}
		}
	}

	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)
	var facts []string
	if c.SetName != "" {
		facts = append(facts, c.SetName)
	}
	if c.Rarity != "" {
		facts = append(facts, c.Rarity)
	}
	if c.EDHRECRank > 0 {
		facts = append(facts, "edhrec #"+itoa(c.EDHRECRank))
	}
	if len(facts) > 0 {
		fmt.Println()
		fmt.Println(dim.Render(strings.Join(facts, " · ")))
	}

	if legal := legalities(c, width); len(legal) > 0 {
		fmt.Println()
		fmt.Println(head.Render("legal in"))
		for _, line := range legal {
			fmt.Println(line)
		}
	}

	fmt.Println()
	fmt.Println(head.Render("rulings"))
	got, err := scryfall.Rulings(c.RulingsURI)
	switch {
	case err != nil:
		fmt.Println(dim.Render("couldn't fetch them"))
	case len(got) == 0:
		fmt.Println(dim.Render("none"))
	default:
		body := lipgloss.NewStyle().Foreground(theme.TextDim)
		for i, r := range got {
			if i > 0 {
				fmt.Println()
			}
			for _, line := range wrapStyled(r.Comment, width, body) {
				fmt.Println(strings.TrimRight(line, " "))
			}
		}
	}
}

// PrintCardIfSingle prints the card when a query matches exactly one, and
// says whether it did. Anything else — no answer, several answers, a failure
// — is the interface's business.
func PrintCardIfSingle(query string) bool {
	cards, total, err := scryfall.Search(query, "edhrec", 2)
	if err != nil || total != 1 || len(cards) != 1 {
		return false
	}

	var rd rules.Data
	if rules.Cached() {
		rd, _ = rules.Load()
	}
	PrintCard(cards[0], rd)
	return true
}

// stdoutWidth is how wide to wrap for. Long lines are hard to read however
// wide the terminal is, so this stops well short of most of them.
func stdoutWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		w = 80
	}
	w -= 2
	if w > 100 {
		w = 100
	}
	if w < 30 {
		w = 30
	}
	return w
}
