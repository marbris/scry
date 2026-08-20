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
	// info is what the information panel shows for the highlighted row,
	// already wrapped to width. A rule is a paragraph and a panel column is
	// not, which is why the rules panel needs this more than anything else.
	info(width int) []string
	// keys are the bindings this view answers to, grouped for the hint bar
	// and the reference — "navigation" and then whatever the view itself
	// does. An interface method rather than a table somewhere central,
	// because a table is what gets forgotten when a view is added — and a
	// reference listing keys that do nothing here is worse than no reference.
	//
	// The view leaves out the keys the workspace owns everywhere the same:
	// i, h/l, esc, space, q, and the information-panel keys for a card list.
	// Those are folded in by hintGroups so no view has to repeat them and
	// none can offer esc under two different names.
	keys() []hintGroup
	// clear undoes one level of narrowing for esc — a selection, then a
	// filter. Returning false means there is nothing left to clear and esc
	// should move on to popping the stack.
	clear() bool
}

// hintGroup is a titled cluster of keys — "navigation", "select", "edit",
// "info panel". Grouping is what lets the hint bar put more on a line and
// read at a glance, each line led by what it is rather than running the whole
// keymap together.
type hintGroup struct {
	title string
	keys  [][2]string
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
func (c *cursor) navKey(k string, n int) bool {
	switch k {
	case "j", "down":
		c.move(1, n)
	case "k", "up":
		c.move(-1, n)
	case "g":
		c.top()
	case "G":
		c.bottom(n)
	default:
		return false
	}
	return true
}
