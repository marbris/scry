package main

import "time"

// The handful of tuning constants every screen shares. The palette moved to
// internal/theme.

const (
	// Below this width the panel sits under the list instead of beside it
	compactWidth = 120

	// Scryfall returns 175 cards per page; that's the whole result set we keep.
	maxResults = 175

	// How long a card has to stay under the cursor before its rulings
	// are fetched, so scrolling past a card costs nothing.
	rulingsDelay = 100 * time.Millisecond
)
