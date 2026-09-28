package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/keymap"
	"scry/internal/theme"
)

const keysUsage = `Usage:
  scry keys                 List every key, as it is bound now
  scry keys --defaults      Print the default keymap as a keys.json

Your rebindings go in %s,
grouped by where a key acts, then by what it does. Name only what you change;
everything else keeps its default:

  {
    "cards":  { "sort1.next": ["."], "sort1.prev": [">"] },
    "global": { "leader": ["space"] }
  }

A key is written the way the terminal reports it: "a", "A", "ctrl+k",
"shift+tab", "enter", "esc", "space". An empty list unbinds an action. Two
actions on one key in the same group is a conflict, and that group keeps its
defaults until it's fixed.`

func runKeys(args []string) {
	switch {
	case len(args) == 0:
		listKeys()
	case args[0] == "--defaults":
		os.Stdout.Write(keymap.DefaultsJSON())
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Printf(keysUsage+"\n", keymap.Path())
	default:
		fmt.Printf(keysUsage+"\n", keymap.Path())
		os.Exit(1)
	}
}

// listKeys prints the keymap in force, a heading per group.
func listKeys() {
	heading := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	key := lipgloss.NewStyle().Foreground(theme.Highlight).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var scope keymap.Scope
	for _, e := range keymap.Current() {
		if e.Scope != scope {
			if scope != "" {
				fmt.Println()
			}
			scope = e.Scope
			fmt.Println(heading.Render(string(scope)))
		}
		shown := make([]string, len(e.Keys))
		for i, k := range e.Keys {
			shown[i] = keymap.Display(k)
		}
		keys := strings.Join(shown, " ")
		if keys == "" {
			keys = dim.Render("unbound")
		} else {
			keys = key.Render(keys)
		}
		fmt.Printf("  %-24s %s\n", e.Action, keys)
	}
}
