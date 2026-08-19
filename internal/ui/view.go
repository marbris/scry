package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// What a panel is showing.
//
// A panel holds a stack of these rather than one, because some of them are
// reached from inside another: a deck's versions from the deck, a Moxfield
// user's decks from the user. esc pops back rather than closing the panel,
// which is the difference between stepping into something and opening it.
//
// The stack is per panel, not global. Two decks panels can be at different
// depths, and neither knows about the other — the whole point of the row.

type view interface {
	// title is what the header says once the search bar has closed.
	title() string
	// subtitle is the line under it: counts, ordering, what's been picked.
	subtitle() string
	// lines draws the body, exactly height lines of exactly width columns.
	lines(width, height int, focused bool, m *Model) []string
	// key offers a keypress. Returning false lets the workspace have it.
	key(k string, m *Model, p *panel) (bool, tea.Cmd)
	// clear undoes one level of narrowing for esc — a selection, then a
	// filter. Returning false means there is nothing left to clear and esc
	// should move on to popping the stack.
	clear() bool
}

// cursor is the part every list has: where you are, and how far the view has
// scrolled to keep you visible.
type cursor struct {
	at     int
	offset int
}

func (c *cursor) move(delta, n int) {
	c.at += delta
	c.clamp(n)
}

func (c *cursor) top() { c.at = 0 }

func (c *cursor) bottom(n int) {
	c.at = n - 1
	c.clamp(n)
}

func (c *cursor) clamp(n int) {
	if c.at >= n {
		c.at = n - 1
	}
	if c.at < 0 {
		c.at = 0
	}
}

// scrollInto brings the cursor into view with as little movement as
// possible. A list that recentred on every step would slide under the eye.
func (c *cursor) scrollInto(height, n int) {
	if height < 1 {
		height = 1
	}
	if c.at < c.offset {
		c.offset = c.at
	}
	if c.at >= c.offset+height {
		c.offset = c.at - height + 1
	}
	if max := n - height; c.offset > max {
		c.offset = max
	}
	if c.offset < 0 {
		c.offset = 0
	}
}

// navKey handles the movement keys every list shares, so no view has to
// implement j and k for itself.
func (c *cursor) navKey(k string, n, page int) bool {
	switch k {
	case "j", "down":
		c.move(1, n)
	case "k", "up":
		c.move(-1, n)
	case "g":
		c.top()
	case "G":
		c.bottom(n)
	case "ctrl+d":
		c.move(page, n)
	case "ctrl+u":
		c.move(-page, n)
	default:
		return false
	}
	return true
}
