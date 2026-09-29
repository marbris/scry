package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/deck"
	"scry/internal/fetch"
	"scry/internal/scryfall"
)

// Searching Scryfall.
//
// Two orders are in play and they are not the same thing. The query sort —
// ctrl+o, with ctrl+r for its direction — is part of the request: it
// decides which cards come back when a query matches more than one page.
// Both only change what the next enter asks for; neither sends anything. The list sort — o and O — decides
// how the cards already in front of you are arranged. Confusing them means
// re-fetching to reorder, or reordering and wondering why the cards changed.

// maxResults is one Scryfall page, which is the whole result set kept.
const maxResults = 175

// searchDoneMsg carries a finished search back to the panel that asked for
// it. Panels are identified by id rather than by index because the row can
// be reordered or closed while a request is in flight.
type searchDoneMsg struct {
	panel int
	query string
	cards []deck.Card
	total int
	err   error
}

// runSearch asks Scryfall, off the main thread.
func runSearch(panelID int, query, order, dir string) tea.Cmd {
	return func() tea.Msg {
		cards, total, err := scryfall.Search(query, order, dir, maxResults)
		if err != nil {
			return searchDoneMsg{panel: panelID, query: query, err: err}
		}

		// A search result is a deck card with no quantity and no tags,
		// which is exactly what it is.
		out := make([]deck.Card, len(cards))
		for i, c := range cards {
			out[i] = deck.Card{Card: c}
		}
		return searchDoneMsg{panel: panelID, query: query, cards: out, total: total}
	}
}

// search starts one for the focused panel and puts it into its waiting state.
func (m *Model) search(p *panel) tea.Cmd {
	query := p.search.Value()
	if query == "" {
		return nil
	}

	m.history = RememberQuery(m.history, query)
	SaveQueryHistory(m.history)
	p.leaveHistory()

	p.loading = true
	p.err = nil
	p.searchOpen = false
	p.search.Blur()
	p.title = query

	return runSearch(p.id, query, p.queryOrder(), p.queryDirection())
}

// handleSearchDone files a finished search, if the panel that asked for it
// is still there and still waiting for this query.
func (m Model) handleSearchDone(msg searchDoneMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil || p.title != msg.query {
		// The panel closed, or moved on to another search. Either way this
		// answer is to a question nobody is asking any more.
		return m, nil
	}

	p.loading = false
	if msg.err != nil {
		p.err = msg.err
		p.stack = nil
		return m, nil
	}

	// Kept in the order Scryfall sent them. The query asked for an order —
	// EDHREC rank, by default — and re-sorting on arrival would throw away
	// the answer to the question just asked.
	l := newCardList(msg.cards, sortArrival, "scryfall order")
	l.name = msg.query
	l.matched = msg.total
	p.show(l)
	return m, nil
}

// errorText is what a failed search says. "No results" is an answer rather
// than a failure, and reads better as one.
func errorText(err error) string {
	if _, ok := err.(fetch.NotFound); ok {
		return "no results"
	}
	return err.Error()
}

// cycleQuerySort changes which cards a large query will come back with. It
// sends nothing: the bar may hold a query you haven't finished, and the
// panel may be showing results for a different one. Enter sends.
func (p *panel) cycleQuerySort(delta int) {
	n := len(scryfall.SortOptions)
	p.querySort = ((p.querySort+delta)%n + n) % n
}

// cycleQueryDir changes which way the next search asks for its order to
// run: auto, ascending, descending. Like the order, it waits for enter.
func (p *panel) cycleQueryDir(delta int) {
	n := len(scryfall.DirOptions)
	p.queryDir = ((p.queryDir+delta)%n + n) % n
}

// queryOrder is the name of the order the request will ask for.
func (p *panel) queryOrder() string { return scryfall.SortOptions[p.querySort] }

// queryDirection is the name of the direction the request will ask for.
func (p *panel) queryDirection() string { return scryfall.DirOptions[p.queryDir] }

// queryOrderLabel is the order as the header shows it: the name, with an
// arrow when the direction is chosen rather than left to Scryfall.
func (p *panel) queryOrderLabel() string {
	switch p.queryDirection() {
	case "asc":
		return p.queryOrder() + " ↑"
	case "desc":
		return p.queryOrder() + " ↓"
	}
	return p.queryOrder()
}
