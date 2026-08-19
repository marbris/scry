// Package theme holds the colours the interface is drawn in.
//
// For now this is the gruvbox palette under its own names, lifted out of the
// UI unchanged. The next step is a layer of semantic roles on top — what a
// colour is *for*, rather than what it is — which is what makes the palette
// swappable. Until then, moving it here at least means there is one place to
// swap.
package theme

import "github.com/charmbracelet/lipgloss"

var (
	Bg      = lipgloss.Color("#282828")
	BgLight = lipgloss.Color("#3c3836")
	Fg      = lipgloss.Color("#ebdbb2")
	FgDim   = lipgloss.Color("#a89984")
	White   = lipgloss.Color("#fbf1c7")
	Gray    = lipgloss.Color("#928374")

	Red    = lipgloss.Color("#cc241d")
	Green  = lipgloss.Color("#98971a")
	Yellow = lipgloss.Color("#d79921")
	Blue   = lipgloss.Color("#458588")
	Purple = lipgloss.Color("#b16286")
	Aqua   = lipgloss.Color("#689d6a")
	Orange = lipgloss.Color("#d65d0e")
)
