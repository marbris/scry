package main

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Colours and the handful of tuning constants every screen shares.

// ── Gruvbox palette ─────────────────────────────────────────────

var (
	gruvBg      = lipgloss.Color("#282828")
	gruvFg      = lipgloss.Color("#ebdbb2")
	gruvWhite   = lipgloss.Color("#fbf1c7")
	gruvRed     = lipgloss.Color("#cc241d")
	gruvGreen   = lipgloss.Color("#98971a")
	gruvYellow  = lipgloss.Color("#d79921")
	gruvBlue    = lipgloss.Color("#458588")
	gruvPurple  = lipgloss.Color("#b16286")
	gruvAqua    = lipgloss.Color("#689d6a")
	gruvOrange  = lipgloss.Color("#d65d0e")
	gruvGray    = lipgloss.Color("#928374")
	gruvFgDim   = lipgloss.Color("#a89984")
	gruvBgLight = lipgloss.Color("#3c3836")
)

const (
	// Sent to Scryfall, Moxfield, MTGJSON and Wizards alike.
	userAgent = "scry/2.1"

	// Below this width the panel sits under the list instead of beside it
	compactWidth = 120

	// Scryfall returns 175 cards per page; that's the whole result set we keep.
	maxResults = 175

	// How long a card has to stay under the cursor before its rulings
	// are fetched, so scrolling past a card costs nothing.
	rulingsDelay = 100 * time.Millisecond
)
