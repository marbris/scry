package ui

// The workspace: a row of panels, one of which has focus, with the
// information panel pinned to the right.
//
// Panels are siblings, not a stack. Opening one doesn't hide another, and
// closing one doesn't reveal what was underneath — there is nothing
// underneath. That is the whole difference from the screen-and-back-stack
// arrangement this replaces.

type workspace struct {
	panels  []*panel
	focused int // index into panels; meaningless when there are none
	// editing is the panel whose deck is the target of a/x/t. Derived
	// rather than chosen: see editingPanel.
	editing int
	// pinned says the editing panel was chosen deliberately, and shouldn't
	// move when focus does.
	pinned bool

	// scroll is the leftmost visible panel when they don't all fit.
	scroll int

	width, height int
}

func newWorkspace() workspace {
	return workspace{editing: -1}
}

func (w workspace) count() int { return len(w.panels) }

func (w workspace) empty() bool { return len(w.panels) == 0 }

// current is the focused panel, or nil when the splash is up.
func (w *workspace) current() *panel {
	if w.empty() {
		return nil
	}
	return w.panels[w.focused]
}

// ── Opening and closing ─────────────────────────────────────────

// open puts a new panel to the right of the focused one and moves focus
// into it. To the right rather than at the end, because a panel is opened
// in the middle of doing something with the one you're on, and it belongs
// beside it.
func (w *workspace) open(kind Kind) {
	p := newPanel(kind)
	at := w.focused + 1
	if w.empty() {
		at = 0
	}
	w.panels = append(w.panels, nil)
	copy(w.panels[at+1:], w.panels[at:])
	w.panels[at] = p
	w.focus(at)
}

// close removes the focused panel. Focus goes to its left neighbour, which
// is where you came from if you opened this one.
func (w *workspace) close() {
	if w.empty() {
		return
	}
	at := w.focused
	w.panels = append(w.panels[:at], w.panels[at+1:]...)

	if w.editing == at {
		w.editing, w.pinned = -1, false
	} else if w.editing > at {
		w.editing--
	}

	if at >= len(w.panels) {
		at = len(w.panels) - 1
	}
	w.focus(maxInt(at, 0))
}

// only closes everything but the focused panel.
func (w *workspace) only() {
	if w.empty() {
		return
	}
	keep := w.panels[w.focused]
	wasEditing := w.editing == w.focused
	w.panels = []*panel{keep}
	w.focused, w.scroll = 0, 0
	if wasEditing {
		w.editing = 0
	} else {
		w.editing, w.pinned = -1, false
	}
}

// ── Moving around ───────────────────────────────────────────────

// focus moves to a panel by index, ignoring one that isn't there. The
// search bar's cursor follows, so it only ever blinks in one place.
func (w *workspace) focus(at int) {
	if at < 0 || at >= len(w.panels) {
		return
	}
	for i, p := range w.panels {
		if i == at && p.searchOpen {
			p.search.Focus()
		} else {
			p.search.Blur()
		}
	}
	w.focused = at
	if !w.pinned {
		w.deriveEditing()
	}
}

// step moves focus along the row. It stops at the ends rather than wrapping:
// the panels are laid out in space, and a left that lands you on the far
// right is a surprise every time.
func (w *workspace) step(delta int) {
	if w.empty() {
		return
	}
	at := w.focused + delta
	if at < 0 || at >= len(w.panels) {
		return
	}
	w.focus(at)
}

// movePanel carries the focused panel along the row, focus going with it.
func (w *workspace) movePanel(delta int) {
	if w.empty() {
		return
	}
	at := w.focused
	to := at + delta
	if to < 0 || to >= len(w.panels) {
		return
	}
	w.panels[at], w.panels[to] = w.panels[to], w.panels[at]

	switch w.editing {
	case at:
		w.editing = to
	case to:
		w.editing = at
	}
	w.focused = to
}

// ── The editing deck ────────────────────────────────────────────

// deriveEditing picks the target for a/x/t without being asked. With one
// deck panel open it is obviously that one; with several it is the one you
// were last in, which is where you were about to work anyway. Pinning with
// e overrides this, and is the only time the concept needs a keystroke.
func (w *workspace) deriveEditing() {
	if w.editable(w.focused) {
		w.editing = w.focused
		return
	}
	// Focus is somewhere else — a search, the rules. Keep the deck we had,
	// unless it has stopped being one.
	if w.editable(w.editing) {
		return
	}
	w.editing = -1
	for i := range w.panels {
		if w.editable(i) {
			w.editing = i
			return
		}
	}
}

// editable reports whether a panel holds a local deck, the only thing that
// can be edited.
func (w *workspace) editable(i int) bool {
	if i < 0 || i >= len(w.panels) {
		return false
	}
	// Until card lists arrive there is nothing editable; phase 7 makes this
	// ask the panel's view whether it's a local deck.
	return false
}

// editingList is the cards of the editing panel, or nil when there is no
// deck being edited.
func (w *workspace) editingList() *cardList {
	if w.editing < 0 || w.editing >= len(w.panels) {
		return nil
	}
	return w.panels[w.editing].cards
}

// pin fixes the editing deck on the focused panel, or lets go of it.
func (w *workspace) pin() {
	if !w.editable(w.focused) {
		return
	}
	if w.pinned && w.editing == w.focused {
		w.pinned = false
		w.deriveEditing()
		return
	}
	w.editing, w.pinned = w.focused, true
}

// ── Layout ──────────────────────────────────────────────────────

// layout works out this frame's widths and remembers where the row was
// scrolled to, so the next frame starts from the same place.
func (w *workspace) layout() layout {
	l := computeLayout(w.width, w.height, w.count(), w.focused, w.scroll)
	w.scroll = l.first
	return l
}
