package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"scry/internal/theme"
)

const themeUsage = `Usage:
  scry theme                List the themes available
  scry theme <name>         Use a theme
  scry theme edit <name>    Copy a theme into your config to edit

Themes are JSON files in %s.
A theme names its colours; every role — accent, borders, mana, rarity —
falls back to the default mapping, so thirteen colours is a whole theme.`

func runTheme(args []string) {
	switch {
	case len(args) == 0:
		listThemes()

	case args[0] == "edit" && len(args) > 1:
		editTheme(args[1])

	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Printf(themeUsage+"\n", theme.Dir())

	case len(args) == 1:
		if err := theme.Set(args[0]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		fmt.Printf("Theme set to %s.\n", args[0])

	default:
		fmt.Printf(themeUsage+"\n", theme.Dir())
		os.Exit(1)
	}
}

// listThemes shows each theme's palette as swatches, which says more about
// whether you want it than its name does.
func listThemes() {
	current := theme.Current()
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	for _, name := range theme.List() {
		t, err := theme.Find(name)
		if err != nil {
			continue
		}

		marker := "  "
		if name == current {
			marker = lipgloss.NewStyle().Foreground(theme.Accent).Render("▸ ")
		}

		swatches := paletteBar(t)

		where := "built-in"
		if !theme.IsBuiltin(name) {
			where = "yours"
		}
		fmt.Printf("%s%-14s %s  %s\n", marker, name, swatches, dim.Render(where))
	}

	fmt.Println()
	fmt.Println(dim.Render("scry theme <name> to switch · scry theme edit <name> to copy one and change it"))
}

// paletteBar draws a theme's colours as blocks. Piped to a file there is no
// colour to draw with, so it says the values instead — a listing whose whole
// content is colour is a blank page in a pager.
func paletteBar(t theme.Theme) string {
	if !stdoutIsTerminal() {
		var parts []string
		for _, name := range theme.PaletteNames() {
			if value, ok := t.Palette[name]; ok {
				parts = append(parts, value)
			}
		}
		return strings.Join(parts, " ")
	}

	var b strings.Builder
	for _, name := range theme.PaletteNames() {
		value, ok := t.Palette[name]
		if !ok {
			b.WriteString(" ")
			continue
		}
		b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color(value)).Render("  "))
	}
	return b.String()
}

func stdoutIsTerminal() bool { return term.IsTerminal(os.Stdout.Fd()) }

// editTheme copies a theme into the config directory with every role written
// out, so there's something to edit rather than a blank file.
func editTheme(name string) {
	body, err := theme.Export(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	os.MkdirAll(theme.Dir(), 0755)
	path := filepath.Join(theme.Dir(), name+".json")
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "Error: %s already exists — edit it, or delete it first.\n", path)
		os.Exit(1)
	}
	if err := os.WriteFile(path, body, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s\n", path)
	fmt.Println("It shadows the built-in of the same name. Edit and run `scry theme " + name + "`.")
}
