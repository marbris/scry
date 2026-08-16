package main

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// A pane is a list of cards and the statistics narrowing applied to it. The
// search results are one and the open deck is another, so everything that
// works on a list of cards — filtering, the statistics panel, the card
// preview — works the same on either without knowing which it has.

type pane struct {
	list      list.Model
	baseItems []list.Item // the cards before the statistics narrowed them

	// Which statistics category the list is narrowed to. statIndex is -1
	// when the panel's category list hasn't been entered.
	statFilter *statRow
	statIndex  int

	// How the cards on screen are ordered, and what to call the order they
	// arrived in — "search order" for results, "decklist" for a deck.
	sort        cardSort
	arrivedName string
}

// refresh rebuilds the list from baseItems: narrowed to the statistics
// category if one is chosen, then sorted. Everything that changes what's on
// screen goes through here, so the two can't be applied in the wrong order
// or one of them forgotten.
func (p *pane) refresh() {
	items := p.baseItems
	if p.statFilter != nil {
		kept := make([]list.Item, 0, len(items))
		for _, it := range items {
			ci, ok := it.(cardItem)
			if !ok {
				continue
			}
			if p.statFilter.match(ci) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	p.list.SetItems(sortItems(items, p.sort))
}

// sortName is what the header calls the pane's current order.
func (p pane) sortName() string { return p.sort.name(p.arrivedName) }

// setItems installs a fresh set of cards, forgetting whichever statistics
// category the last lot was narrowed to.
// setItems installs a fresh set of cards, forgetting whichever statistics
// category the last lot was narrowed to. The sort is deliberately kept: it's
// a way of reading a list, not a property of one particular set of results.
func (p *pane) setItems(items []list.Item) {
	p.baseItems = items
	p.statFilter = nil
	p.statIndex = -1
	p.refresh()
	p.list.ResetSelected()
}

func (p pane) selected() (cardItem, bool) {
	ci, ok := p.list.SelectedItem().(cardItem)
	return ci, ok
}

func (p pane) empty() bool { return len(p.baseItems) == 0 }

// filtering reports whether the list is mid-way through a typed filter, when
// keys belong to the filter rather than to the app.
func (p pane) filtering() bool { return p.list.FilterState() == list.Filtering }

// ── Focus ───────────────────────────────────────────────────────

// focusArea is which of the three regions keys are going to.
type focusArea int

const (
	focusSearch  focusArea = iota // the search bar
	focusResults                  // the list of search results
	focusDeck                     // the open deck beside it
)

func (m model) searchFocused() bool { return m.focus == focusSearch }

// active is the pane keys and the card panel are working on. The search bar
// types into the results, so it counts as the results pane.
//
// A pointer, so callers can change the pane in place; the receiver is a
// pointer too, or it would hand back a pointer into a copy that's about to
// be thrown away.
func (m *model) active() *pane {
	if m.focus == focusDeck {
		return &m.deckPane
	}
	return &m.results
}

// activeView is the focused pane by value, for the places that only read it.
// active() needs a pointer and an addressable model; this doesn't.
func (m model) activeView() pane {
	if m.focus == focusDeck {
		return m.deckPane
	}
	return m.results
}

// cycleSort reorders the focused list. The sort belongs to the pane, so the
// results and the deck can be read different ways at the same time.
func (m model) cycleSort(delta int) (tea.Model, tea.Cmd) {
	p := m.active()
	p.sort = p.sort.next(delta)

	// Keep the cursor on the card it was on; sorting moves the rows, not
	// what you were looking at.
	on := ""
	if it, ok := p.selected(); ok {
		on = it.card.Name
	}
	p.refresh()
	if on != "" {
		selectCard(p, on)
	}

	// No notice: the header carries the order for as long as it applies,
	// and saying it twice for one keypress is just noise.
	next, cmd := m.syncHover()
	return next, cmd
}

// deckOpen reports whether a deck is open. An empty one still counts: a
// deck you've just created is empty by definition, and you can't fill it if
// nothing will show it to you.
func (m model) deckOpen() bool { return m.deck != nil }

// deckInMainList reports whether the deck is what the main list is showing,
// rather than sitting in a column of its own. The header describes the main
// list, so it needs to know which of the two is in there.
func (m model) deckInMainList() bool {
	if !m.deckOpen() {
		return false
	}
	// A deck opened on its own is all there is to show.
	if m.results.empty() {
		return true
	}
	return m.focus == focusDeck && !m.deckColumn()
}

// bothLists reports whether there are two lists to move between. With only
// one there's nowhere for tab or esc to go.
func (m model) bothLists() bool { return m.deckOpen() && !m.results.empty() }

// cycleFocus moves between the two lists. The search bar isn't in the cycle
// because tab already cycles the sort order while you're typing a query,
// which is the only place sort means anything; i and esc move in and out of
// it as they always have.
func (m model) cycleFocus() model {
	if !m.bothLists() {
		return m
	}
	if m.focus == focusDeck {
		return m.setFocus(focusResults)
	}
	return m.setFocus(focusDeck)
}

// lastListFocus is the list to go back to on leaving the search bar: the one
// you were last on, or whichever has anything in it.
func (m model) lastListFocus() focusArea {
	if m.returnFocus == focusDeck && m.deckOpen() {
		return focusDeck
	}
	if m.results.empty() && m.deckOpen() {
		return focusDeck
	}
	return focusResults
}

// setFocus moves focus, keeping the search input's own focus in step so the
// cursor only ever blinks in one place.
func (m model) setFocus(f focusArea) model {
	// Focusing a deck that isn't there would leave keys going nowhere.
	if f == focusDeck && !m.deckOpen() {
		f = focusResults
	}
	m.focus = f
	if f != focusSearch {
		m.returnFocus = f
	}

	if f == focusSearch {
		m.searchInput.Focus()
		m.searchInput.CursorEnd()
	} else {
		m.searchInput.Blur()
	}
	return m
}
