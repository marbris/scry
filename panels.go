package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/ui"
)

// The way into the panel workspace while it's being built. The old two-pane
// screen is still the default; this is how to see what has replaced it so
// far without the two getting in each other's way.
func runPanels() {
	p := tea.NewProgram(ui.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
