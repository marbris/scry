package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/ui"
)

// The way into the panel workspace while it's being built. The old two-pane
// screen is still the default; this is how to see what has replaced it so
// far without the two getting in each other's way.
func runPanels(args []string) {
	m := ui.New()
	var cmd tea.Cmd
	if len(args) > 0 && args[0] != "" {
		// `scry --panels <query>` opens straight onto the search, the way
		// `scry <query>` always has.
		m, cmd = ui.NewWithQuery(strings.Join(args, " "))
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if cmd != nil {
		go func() { p.Send(cmd()) }()
	}
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
