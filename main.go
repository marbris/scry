package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The entry point and the non-TUI paths out of it: the one-shot card lookup
// that prints to stdout, and the `scry rules` / `scry deck` subcommands.

// ── Stdout output ───────────────────────────────────────────────

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

// ── Entry point ─────────────────────────────────────────────────

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
