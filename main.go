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

	runTUI(m)
}

const deckUsage = `Usage:
  scry deck <name>                Open one of your decks
  scry deck <id | url>            Browse a deck on Moxfield, without saving it
  scry deck list                  List your decks
  scry deck import <id|url> [as]  Copy a Moxfield deck in so you can edit it
  scry deck rm <name>             Delete a deck

Decks are files in ` + "`" + `scry deck dir` + "`" + `, one card per line — edit them here or in
your editor. Inside the app, w imports the Moxfield deck you're looking at.`

// runDeck opens a deck, or handles one of the deck subcommands. References
// are checked here so a typo fails on the terminal; the deck itself loads
// once the TUI is up, which keeps the "loading deck…" line on screen.
func runDeck(m model, args []string) {
	if len(args) == 0 {
		fmt.Println(deckUsage)
		os.Exit(1)
	}

	switch args[0] {
	case "list", "ls":
		runDeckList()
		return
	case "import", "save":
		runDeckImport(args[1:])
		return
	case "rm", "remove", "forget", "delete":
		runDeckRemove(args[1:])
		return
	case "dir":
		fmt.Println(decksDir())
		return
	}

	arg := strings.Join(args, " ")

	// One of your own decks wins over reading the same text as a Moxfield
	// id, so a deck called "ghen" can't be shadowed by anything on Moxfield.
	if deckExists(arg) {
		m.searching = true
		m.deckLoading = true
		m.initialDeckSlug = arg
		m.searchInput.SetValue("deck " + arg)
		runTUI(m)
		return
	}

	// A deck saved as a bare Moxfield reference by an older version still
	// opens, straight off Moxfield.
	id := ""
	if saved, ok := lookupSavedDeck(arg); ok {
		id = saved.ID
	} else {
		var ok bool
		if id, ok = deckRef(arg); !ok {
			fmt.Printf("No deck called %q, and that isn't a Moxfield id or URL.\n", arg)
			fmt.Println("Your decks:")
			runDeckList()
			os.Exit(1)
		}
	}

	m.searching = true
	m.deckLoading = true
	m.initialDeck = id
	m.searchInput.SetValue("https://moxfield.com/decks/" + id)
	runTUI(m)
}

func runTUI(m model) {
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func runDeckList() {
	slugs, err := listDecks()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	legacy, _ := loadSavedDecks()

	if len(slugs) == 0 && len(legacy) == 0 {
		fmt.Println("No decks yet. Copy one in from Moxfield with:")
		fmt.Println("  scry deck import <moxfield url>")
		return
	}

	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvYellow)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)

	width := 0
	for _, s := range slugs {
		if len(s) > width {
			width = len(s)
		}
	}
	for a := range legacy {
		if len(a) > width {
			width = len(a)
		}
	}

	pad := func(s string) string { return s + strings.Repeat(" ", width-len(s)) }

	for _, slug := range slugs {
		d, err := readDeck(slug)
		if err != nil {
			fmt.Printf("%s  %s\n", nameStyle.Render(pad(slug)), dimStyle.Render(err.Error()))
			continue
		}
		total, unique := d.counts()
		fmt.Printf("%s  %s\n", nameStyle.Render(pad(slug)), d.Name)

		detail := fmt.Sprintf("%d cards, %d distinct", total, unique)
		if d.Format != "" {
			detail = d.Format + " · " + detail
		}
		fmt.Printf("%s  %s\n", strings.Repeat(" ", width), dimStyle.Render(detail))
	}

	// Decks an older version saved as a Moxfield reference rather than a
	// file. They still open; importing one makes it editable.
	for _, a := range aliasesSorted(legacy) {
		if deckExists(a) {
			continue // already imported, the file above is the real one
		}
		fmt.Printf("%s  %s %s\n", nameStyle.Render(pad(a)), legacy[a].Name,
			dimStyle.Render("(on Moxfield — `scry deck import "+a+"` to edit it here)"))
	}
}

func runDeckImport(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: scry deck import <deck-id | moxfield url> [name]")
		os.Exit(1)
	}

	ref := args[0]
	if saved, ok := lookupSavedDeck(ref); ok {
		ref = saved.ID
	}
	id, ok := deckRef(ref)
	if !ok {
		fmt.Println("Not a Moxfield deck id or URL:", args[0])
		os.Exit(1)
	}

	fmt.Println("Fetching deck…")
	d, err := importMoxfield(id)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	slug := slugify(d.Name)
	if len(args) > 1 {
		slug = slugify(strings.Join(args[1:], " "))
	}
	if slug == "" {
		slug = id
	}

	verb := "Imported"
	if deckExists(slug) {
		verb = "Updated"
	}
	if err := writeDeck(slug, d); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	total, unique := d.counts()
	fmt.Printf("%s %q — %d cards, %d distinct\n", verb, d.Name, total, unique)
	fmt.Printf("  %s\n", deckFilePath(slug))
	fmt.Printf("Open it with: scry deck %s\n", slug)
}

func runDeckRemove(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: scry deck rm <name>")
		os.Exit(1)
	}
	name := args[0]

	removed := false
	if deckExists(name) {
		if err := deleteDeck(name); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		removed = true
	}
	if forgotten, err := forgetDeck(name); err == nil && forgotten {
		removed = true
	}

	if !removed {
		fmt.Printf("No deck called %q.\n", name)
		os.Exit(1)
	}
	fmt.Printf("Deleted %s.\n", name)
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
