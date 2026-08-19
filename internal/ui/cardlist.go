package ui

import (
	"strings"

	"scry/internal/deck"

	tea "github.com/charmbracelet/bubbletea"
)

// A list of cards in a panel: what's in it, how it's ordered, what's been
// narrowed away and what's been picked out.
//
// Search results and decks are the same thing here. A search result is a
// deck card with no quantity and no tags, which is what it is, and having
// one type means every key that works on a list works on both.

type cardList struct {
	// all is every card, in the order it arrived. Nothing reorders this;
	// the sort and the filter build rows from it, so both can be undone.
	all   []deck.Card
	order cardSort

	// rows is what's on screen: all, narrowed and sorted.
	rows []deck.Card

	cursor // where you are in rows, and how far it has scrolled

	// filter is a literal narrowing. Terms are substrings, all of them have
	// to appear, and they're matched against the name and the rules text —
	// which is the one thing a fuzzy match makes useless, since over a
	// hundred cards of oracle text almost anything matches something.
	filter string

	// marks are the cards picked out with v, by lowercased name so they
	// survive the list being filtered or re-sorted underneath them.
	marks map[string]bool

	// name is what the header calls this list, and matched is how many
	// cards the query behind it found — usually more than were fetched.
	name    string
	matched int

	// deck is set when this list is a deck rather than a search result. A
	// local one can be edited; a borrowed one can't.
	deck *deck.Info

	// arrivalName is what to call the order the cards came in — "as found"
	// says nothing, where "scryfall order" and "decklist" say what you're
	// looking at.
	arrivalName string
}

func newCardList(cards []deck.Card, order cardSort, arrivalName string) *cardList {
	if arrivalName == "" {
		arrivalName = "as found"
	}
	l := &cardList{all: cards, order: order, marks: map[string]bool{}, arrivalName: arrivalName}
	l.refresh()
	return l
}

// orderName is what the panel calls its current order.
func (l *cardList) orderName() string {
	if l.order == sortArrival {
		return l.arrivalName
	}
	return l.order.String()
}

// refresh rebuilds what's on screen. Everything that changes the list goes
// through here, so the filter and the sort can't be applied in the wrong
// order or one of them forgotten.
func (l *cardList) refresh() {
	rows := l.all
	if terms := filterTerms(l.filter); len(terms) > 0 {
		kept := make([]deck.Card, 0, len(rows))
		for _, c := range rows {
			if matches(c, terms) {
				kept = append(kept, c)
			}
		}
		rows = kept
	}
	l.rows = sortCards(rows, l.order)
	l.clampCursor()
}

func (l *cardList) count() int  { return len(l.rows) }
func (l *cardList) total() int  { return len(l.all) }
func (l *cardList) empty() bool { return len(l.all) == 0 }

// current is the card under the cursor.
func (l *cardList) current() (deck.Card, bool) {
	if l.cursor.at < 0 || l.cursor.at >= len(l.rows) {
		return deck.Card{}, false
	}
	return l.rows[l.cursor.at], true
}

// ── Moving ──────────────────────────────────────────────────────

func (l *cardList) move(delta int) { l.cursor.move(delta, len(l.rows)) }
func (l *cardList) top()           { l.cursor.top() }
func (l *cardList) bottom()        { l.cursor.bottom(len(l.rows)) }
func (l *cardList) clampCursor()   { l.cursor.clamp(len(l.rows)) }

// ── Ordering and narrowing ──────────────────────────────────────

// cycleSort reorders the list, keeping the cursor on the card it was on.
// Sorting moves the rows, not what you were looking at.
func (l *cardList) cycleSort(delta int) {
	on, had := l.current()
	l.order = l.order.next(delta)
	l.refresh()
	if had {
		l.selectByName(on.Card.Name)
	}
}

func (l *cardList) setFilter(s string) {
	on, had := l.current()
	l.filter = s
	l.refresh()
	// Stay on the same card if it survived the narrowing; otherwise start
	// at the top of what's left.
	if had && !l.selectByName(on.Card.Name) {
		l.cursor.at, l.cursor.offset = 0, 0
	}
}

func (l *cardList) selectByName(name string) bool {
	for i, c := range l.rows {
		if c.Card.Name == name {
			l.cursor.at = i
			return true
		}
	}
	return false
}

// ── Picking cards out ───────────────────────────────────────────

func markKey(c deck.Card) string { return strings.ToLower(c.Card.Name) }

func (l *cardList) marked(c deck.Card) bool { return l.marks[markKey(c)] }

func (l *cardList) markCount() int { return len(l.marks) }

// toggleMark picks the card under the cursor out, or puts it back, and
// steps down — so v v v takes three in a row.
func (l *cardList) toggleMark() {
	c, ok := l.current()
	if !ok {
		return
	}
	key := markKey(c)
	if l.marks[key] {
		delete(l.marks, key)
	} else {
		l.marks[key] = true
	}
	l.move(1)
}

// markAll takes everything the filter has left on screen, or lets it all go
// if it already has. Narrowing to a word and pressing V is the fast way to
// pick out a theme.
func (l *cardList) markAll() {
	all := true
	for _, c := range l.rows {
		if !l.marks[markKey(c)] {
			all = false
			break
		}
	}
	for _, c := range l.rows {
		if all {
			delete(l.marks, markKey(c))
		} else {
			l.marks[markKey(c)] = true
		}
	}
}

func (l *cardList) clearMarks() { l.marks = map[string]bool{} }

// selection is the cards picked out, or — with nothing picked out — the one
// under the cursor. Every command that acts on "the selected cards" goes
// through here, so none of them has to decide what to do with an empty
// selection.
func (l *cardList) selection() []deck.Card {
	if len(l.marks) == 0 {
		if c, ok := l.current(); ok {
			return []deck.Card{c}
		}
		return nil
	}
	var out []deck.Card
	for _, c := range l.all {
		if l.marks[markKey(c)] {
			out = append(out, c)
		}
	}
	return out
}

// ── Filtering ───────────────────────────────────────────────────

// matches reports whether every term appears in the card's name or its
// rules text.
func matches(c deck.Card, terms []string) bool {
	hay := strings.ToLower(c.Card.Name + "\n" + c.Card.CombinedOracle() + "\n" +
		c.Card.TypeLine + "\n" + strings.Join(c.Tags, " "))
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// filterTerms splits a filter into the substrings that all have to match.
// Quoting keeps a phrase together: `"first strike"` is one term, where
// first strike would be two.
func filterTerms(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}

	var terms []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				terms = append(terms, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		terms = append(terms, cur.String())
	}
	return terms
}

// ── Drawing ─────────────────────────────────────────────────────

// render draws the visible rows. members says which cards to flag as living
// in the editing deck as well as here.
func (l *cardList) render(width, height int, members map[string]bool, focused bool) []string {
	l.cursor.scrollInto(height, len(l.rows))

	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.rows) && len(lines) < height; i++ {
		c := l.rows[i]
		lines = append(lines, renderRow(c, l.order, rowState{
			selected: l.marked(c),
			member:   members[markKey(c)],
			cursor:   focused && i == l.cursor.at,
		}, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// names is every card in the list by lowercased name, for the panel beside
// it to flag what they have in common.
func (l *cardList) names() map[string]bool {
	out := make(map[string]bool, len(l.all))
	for _, c := range l.all {
		out[markKey(c)] = true
	}
	return out
}

// ── As a view ───────────────────────────────────────────────────

func (l *cardList) title() string { return l.name }

func (l *cardList) subtitle() string {
	out := itoa(l.count())
	if l.count() != l.total() {
		out += "/" + itoa(l.total())
	} else if l.matched > l.total() {
		// Scryfall matched more than one page; say so, or 175 looks like
		// the whole answer.
		out += "/" + itoa(l.matched)
	}
	out += " · " + l.orderName()
	if n := l.markCount(); n > 0 {
		out += " · " + itoa(n) + " picked"
	}
	return out
}

func (l *cardList) lines(width, height int, focused bool, m *Model) []string {
	if l.count() == 0 {
		what := "nothing matches"
		if l.total() == 0 {
			what = "no results"
		}
		return fillTo([]string{mutedLine(what, width)}, width, height)
	}
	return l.render(width, height, m.membersFor(l), focused)
}

func (l *cardList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if l.cursor.navKey(k, len(l.rows), m.pageStep()) {
		return true, nil
	}
	switch k {
	case "o":
		l.cycleSort(1)
	case "O":
		l.cycleSort(-1)
	case "v":
		l.toggleMark()
	case "V":
		l.markAll()
	case "/":
		p.openFilter(l.filter)
	default:
		return false, nil
	}
	return true, nil
}

// clear undoes one narrowing: the selection first, then the filter.
func (l *cardList) clear() bool {
	switch {
	case l.markCount() > 0:
		l.clearMarks()
	case l.filter != "":
		l.setFilter("")
	default:
		return false
	}
	return true
}
