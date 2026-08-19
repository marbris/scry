package ui

// How the row is divided.
//
// The workspace is a row of panels with the information panel pinned to the
// right, so every panel opened makes every other one narrower. That holds
// until they reach a floor, past which narrowing them further would leave
// each one too thin to read a card name in; after that the row scrolls
// instead, and the panels you can't see are still there.

const (
	// minPanel is the narrowest a panel is allowed to become, borders
	// included. Below about this, a card row is an abbreviation of an
	// abbreviation and the panel stops earning its column.
	minPanel = 16

	// The information panel is the one thing on screen whose content is
	// prose, so it gets a width that prose survives at.
	infoMin = 24
	infoMax = 38

	// hintHeight is the line along the bottom saying which keys apply.
	hintHeight = 1
)

// layout is the arithmetic for one frame: how wide everything is, and which
// panels are on screen when they don't all fit.
type layout struct {
	// info is the information panel's width, or 0 when the terminal is too
	// narrow to afford one.
	info int
	// panels are the widths of the visible panels, left to right.
	panels []int
	// first is the index of the leftmost visible panel.
	first int
	// height is the room a panel has, the hint line excluded.
	height int
}

// visible is how many panels the layout is showing.
func (l layout) visible() int { return len(l.panels) }

// shows reports whether a panel index is on screen.
func (l layout) shows(i int) bool { return i >= l.first && i < l.first+l.visible() }

// computeLayout divides the terminal between the panels and the information
// panel.
//
// scroll is where the row was scrolled to last frame; it's honoured unless
// it would put the focused panel off screen, because the panel you're typing
// into has to be one you can see.
func computeLayout(width, height, count, focused, scroll int) layout {
	l := layout{height: maxInt(height-hintHeight, 1)}
	if count == 0 {
		return l
	}

	l.info = infoWidth(width)
	// A terminal too narrow for both gives up the information panel first:
	// the panels are what you're working in.
	if width-l.info < minPanel {
		l.info = 0
	}

	avail := width - l.info
	fit := avail / minPanel
	if fit < 1 {
		fit = 1
	}
	visible := count
	if visible > fit {
		visible = fit
	}

	l.first = clampFirst(scroll, focused, visible, count)
	l.panels = share(avail, visible)
	return l
}

// infoWidth is a quarter of the terminal, within reason.
func infoWidth(total int) int {
	w := total / 4
	if w < infoMin {
		w = infoMin
	}
	if w > infoMax {
		w = infoMax
	}
	return w
}

// clampFirst keeps the focused panel on screen while otherwise leaving the
// scroll position alone — a row that recentred itself every time focus moved
// would shuffle panels under the cursor for no reason.
func clampFirst(scroll, focused, visible, count int) int {
	first := scroll
	if first > count-visible {
		first = count - visible
	}
	if first < 0 {
		first = 0
	}
	if focused < first {
		first = focused
	}
	if focused >= first+visible {
		first = focused - visible + 1
	}
	return first
}

// share divides a width into n columns, giving the leftmost the remainder so
// the total is exact — a column short of the terminal's width leaves a seam
// down the right-hand side.
func share(total, n int) []int {
	if n <= 0 {
		return nil
	}
	base, extra := total/n, total%n
	out := make([]int, n)
	for i := range out {
		out[i] = base
		if i < extra {
			out[i]++
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
