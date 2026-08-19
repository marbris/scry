package ui

import (
	"strings"

	"scry/internal/deck"
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

	cursor int // index into rows
	offset int // first visible row

	// filter is a literal narrowing. Terms are substrings, all of them have
	// to appear, and they're matched against the name and the rules text —
	// which is the one thing a fuzzy match makes useless, since over a
	// hundred cards of oracle text almost anything matches something.
	filter string

	// marks are the cards picked out with v, by lowercased name so they
	// survive the list being filtered or re-sorted underneath them.
	marks map[string]bool
}

func newCardList(cards []deck.Card, order cardSort) *cardList {
	l := &cardList{all: cards, order: order, marks: map[string]bool{}}
	l.refresh()
	return l
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
	if l.cursor < 0 || l.cursor >= len(l.rows) {
		return deck.Card{}, false
	}
	return l.rows[l.cursor], true
}

// ── Moving ──────────────────────────────────────────────────────

func (l *cardList) move(delta int) {
	l.cursor += delta
	l.clampCursor()
}

func (l *cardList) top()    { l.cursor = 0 }
func (l *cardList) bottom() { l.cursor = len(l.rows) - 1; l.clampCursor() }

func (l *cardList) clampCursor() {
	if l.cursor >= len(l.rows) {
		l.cursor = len(l.rows) - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
}

// scrollInto brings the cursor into view for a window of the given height,
// keeping it there with as little movement as possible — a list that
// recentred on every step would slide under the eye.
func (l *cardList) scrollInto(height int) {
	if height < 1 {
		height = 1
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+height {
		l.offset = l.cursor - height + 1
	}
	if max := len(l.rows) - height; l.offset > max {
		l.offset = max
	}
	if l.offset < 0 {
		l.offset = 0
	}
}

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
		l.cursor, l.offset = 0, 0
	}
}

func (l *cardList) selectByName(name string) bool {
	for i, c := range l.rows {
		if c.Card.Name == name {
			l.cursor = i
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
	l.scrollInto(height)

	lines := make([]string, 0, height)
	for i := l.offset; i < len(l.rows) && len(lines) < height; i++ {
		c := l.rows[i]
		lines = append(lines, renderRow(c, l.order, rowState{
			selected: l.marked(c),
			member:   members[markKey(c)],
			cursor:   focused && i == l.cursor,
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
