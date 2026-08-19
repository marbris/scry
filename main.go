package main

import (
	"scry/internal/paths"
	"scry/internal/theme"

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

	front := c.Faces()[0]
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
	// Files used to live in one directory; put any left there where they
	// now belong, before anything goes looking for them.
	paths.Migrate()

	// A broken theme file is worth saying so about, but not worth refusing
	// to start over: the default is already in force.
	if err := theme.Load(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	}

	m := initialModel()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage()
			return
		}
	}

	// `scry --panels` — the panel workspace, which is replacing the
	// two-pane screen a phase at a time. Behind a flag until it can do
	// everything the old one can.
	if len(os.Args) > 1 && (os.Args[1] == "--panels" || os.Args[1] == "panels") {
		runPanels()
		return
	}

	// `scry theme [name | edit name]` — colours.
	if len(os.Args) > 1 && os.Args[1] == "theme" {
		runTheme(os.Args[2:])
		return
	}

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
		m.lastQuery = query
		m.searching = true
	}

	runTUI(m)
}

const deckUsage = `Usage:
  scry deck <name>                Open one of your decks
  scry deck <id | url>            Browse a deck on Moxfield, without saving it
  scry deck list                  List your decks
  scry deck new <name> [format]   Start an empty deck
  scry deck import <id|url> [as]  Copy a Moxfield deck in so you can edit it
  scry deck rm <name>             Delete a deck
  scry deck log <name>            What you've changed, and when
  scry deck restore <name> <ref>  Bring back an earlier version

Decks are files in ` + "`" + `scry deck dir` + "`" + `, one card per line — edit them here or in
your editor. That directory is a git repository, so every change is kept and
` + "`" + `git log` + "`" + ` works on your decks like anything else. Inside the app, w imports
the Moxfield deck you're looking at.`

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
	case "new", "create":
		runDeckNew(args[1:])
		return
	case "import", "save":
		runDeckImport(args[1:])
		return
	case "rm", "remove", "forget", "delete":
		runDeckRemove(args[1:])
		return
	case "log", "history":
		runDeckLog(args[1:])
		return
	case "restore":
		runDeckRestore(args[1:])
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
		runTUI(m)
		return
	}

	id, ok := deckRef(arg)
	if !ok {
		fmt.Printf("No deck called %q, and that isn't a Moxfield id or URL.\n", arg)
		fmt.Println("Your decks:")
		runDeckList()
		os.Exit(1)
	}

	m.searching = true
	m.deckLoading = true
	m.initialDeck = id
	runTUI(m)
}

func runTUI(m model) {
	m = m.restore().startWithLastQuery()
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

	notice := legacyNotice()
	if len(slugs) == 0 {
		fmt.Println("No decks yet. Copy one in from Moxfield with:")
		fmt.Println("  scry deck import <moxfield url>")
		fmt.Print(notice)
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

	for _, slug := range slugs {
		pad := strings.Repeat(" ", width-len(slug))
		d, err := readDeck(slug)
		if err != nil {
			fmt.Printf("%s  %s\n", nameStyle.Render(slug+pad), dimStyle.Render(err.Error()))
			continue
		}
		total, unique := d.Counts()
		fmt.Printf("%s  %s\n", nameStyle.Render(slug+pad), d.Name)

		detail := fmt.Sprintf("%d cards, %d distinct", total, unique)
		if d.Format != "" {
			detail = d.Format + " · " + detail
		}
		fmt.Printf("%s  %s\n", strings.Repeat(" ", width), dimStyle.Render(detail))
	}

	fmt.Print(notice)
}

func runDeckNew(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: scry deck new <name> [format]")
		os.Exit(1)
	}

	format := ""
	name := strings.Join(args, " ")
	// A trailing format is a convenience, not a requirement: "scry deck new
	// Ghen commander" and "scry deck new Ghen" both work.
	if len(args) > 1 && knownFormat(args[len(args)-1]) {
		format = strings.ToLower(args[len(args)-1])
		name = strings.Join(args[:len(args)-1], " ")
	}

	slug, d, err := newDeck(name, format)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if _, warning, err := saveDeckVersioned(slug, d); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	} else if warning != "" {
		fmt.Println(lipgloss.NewStyle().Foreground(gruvOrange).Render("  " + warning))
	}

	fmt.Printf("Created %q (%s)\n", d.Name, d.Format)
	fmt.Printf("  %s\n", deckFilePath(slug))
	fmt.Printf("Open it with: scry deck %s\n", slug)
	fmt.Println(lipgloss.NewStyle().Foreground(gruvGray).
		Render("Search for a card and press a to add it, or c to make it a commander."))
}

// knownFormat is the set of formats worth recognising as a trailing word.
func knownFormat(s string) bool {
	switch strings.ToLower(s) {
	case "commander", "standard", "pioneer", "modern", "legacy",
		"vintage", "pauper", "brawl", "oathbreaker", "limited":
		return true
	}
	return false
}

func runDeckImport(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: scry deck import <deck-id | moxfield url> [name]")
		os.Exit(1)
	}

	id, ok := deckRef(args[0])
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
	subject, warning, err := saveDeckVersioned(slug, d)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	total, unique := d.Counts()
	fmt.Printf("%s %q — %d cards, %d distinct\n", verb, d.Name, total, unique)
	fmt.Printf("  %s\n", deckFilePath(slug))
	if warning != "" {
		fmt.Printf("  %s\n", lipgloss.NewStyle().Foreground(gruvOrange).Render(warning))
	} else {
		fmt.Printf("  %s\n", lipgloss.NewStyle().Foreground(gruvGray).Render("committed: "+subject))
	}
	fmt.Printf("Open it with: scry deck %s\n", slug)
}

const deckLogLimit = 50

func runDeckLog(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: scry deck log <name>")
		os.Exit(1)
	}
	slug := args[0]
	if !deckExists(slug) {
		fmt.Printf("No deck called %q.\n", slug)
		os.Exit(1)
	}

	commits, err := deckHistory(slug, deckLogLimit)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if len(commits) == 0 {
		fmt.Printf("No history for %s yet.\n", slug)
		return
	}

	hashStyle := lipgloss.NewStyle().Foreground(gruvYellow)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)
	for _, c := range commits {
		fmt.Printf("%s  %s\n", hashStyle.Render(c.Short), c.Subject)
		fmt.Printf("%s  %s\n", strings.Repeat(" ", len(c.Short)), dimStyle.Render(c.When))
	}
	fmt.Printf("\n%s\n", dimStyle.Render("scry deck restore "+slug+" <ref>  ·  git -C "+deckRepoPath()+" show <ref>"))
}

func runDeckRestore(args []string) {
	if len(args) != 2 {
		fmt.Println("Usage: scry deck restore <name> <ref>")
		os.Exit(1)
	}
	slug, ref := args[0], args[1]
	if !deckExists(slug) {
		fmt.Printf("No deck called %q.\n", slug)
		os.Exit(1)
	}

	// Show what's coming back before it lands, since this overwrites the
	// deck you have open.
	old, err := deckAt(slug, ref)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if err := restoreDeck(slug, ref); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	total, unique := old.Counts()
	fmt.Printf("Restored %s to %s — %d cards, %d distinct\n", slug, ref, total, unique)
	fmt.Println(lipgloss.NewStyle().Foreground(gruvGray).
		Render("The version you restored over is still in the history."))
}

func runDeckRemove(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: scry deck rm <name>")
		os.Exit(1)
	}
	name := args[0]

	removed := false
	if deckExists(name) {
		if err := deleteDeckCommitted(name); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
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
	m.backStack = []state{stateResults}

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
