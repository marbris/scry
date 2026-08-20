package ui

import (
	"strings"

	"scry/internal/deck"
	"scry/internal/mtg"
	"scry/internal/rules"
	"scry/internal/stats"

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

	// statFilter is the statistics category the list is narrowed to, set by
	// walking the bars in the information panel. Separate from the text
	// filter because they compose: filter to "elf", then narrow to lands.
	statFilter *stats.Row

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

	// rules and rulings are what the information panel needs to describe a
	// card properly. They arrive after the cards do, and the panel simply
	// shows less until they have.
	rules     rules.Data
	rulings   map[string][]mtg.Ruling
	rulingErr map[string]error

	// legality is the verdict on this deck, when it is one and someone has
	// worked it out. Recomputed as you edit, since an edit is exactly what
	// changes the answer.
	legality *deck.Legality

	// dirty means there are edits not yet written. Saving is explicit, so
	// this is the only thing standing between an edit and losing it — which
	// is why quitting asks.
	dirty bool
	// undo holds the deck as it stood before each edit.
	undo []undoStep

	// arrivalName is what to call the order the cards came in — "as found"
	// says nothing, where "scryfall order" and "decklist" say what you're
	// looking at.
	arrivalName string
}

func newCardList(cards []deck.Card, order cardSort, arrivalName string) *cardList {
	if arrivalName == "" {
		arrivalName = "as found"
	}
	l := &cardList{
		all: cards, order: order, marks: map[string]bool{}, arrivalName: arrivalName,
		rulings: map[string][]mtg.Ruling{}, rulingErr: map[string]error{},
	}
	l.refresh()
	return l
}

// recheck works out this deck's legality again, from the cards in hand.
// Called after every edit: adding a card is the thing that makes a deck
// illegal, so the answer has to keep up.
func (l *cardList) recheck() {
	if l.deck == nil {
		return
	}
	// The cards are already resolved — they're on screen — so this needs
	// nothing fetched and can happen on every keystroke.
	verdict := deck.Check(l.deck.Format, l.all, true)
	l.legality = &verdict
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
	rows := l.narrowed()
	l.rows = sortCards(rows, l.order)
	l.cursor.clamp(len(l.rows))
}

// narrowed is the cards after both narrowings, before sorting. The
// statistics count this, so what the bars say and what the list shows can
// never disagree.
func (l *cardList) narrowed() []deck.Card {
	rows := l.all
	if l.statFilter != nil {
		kept := make([]deck.Card, 0, len(rows))
		for _, c := range rows {
			if l.statFilter.Match(c) {
				kept = append(kept, c)
			}
		}
		rows = kept
	}
	if terms := filterTerms(l.filter); len(terms) > 0 {
		kept := make([]deck.Card, 0, len(rows))
		for _, c := range rows {
			if matches(c, terms) {
				kept = append(kept, c)
			}
		}
		rows = kept
	}
	return rows
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
	if l.cursor.navKey(k, len(l.rows)) {
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

	// ── Editing ─────────────────────────────────────────────────

	case "a":
		m.add(l.selection())
	case "x":
		m.remove(l.selection())
	case "y":
		m.yank(l.selection())
		l.clearMarks()
	case "p":
		m.put(l)
	case "t":
		p.ask(askTag, "tag", "")
	case "T":
		m.tagWithLast(l.selection())
	case "c":
		return true, m.commander(currentOr(l))
	case "u":
		m.undo()

	case "w":
		return true, m.write(l, p, false)
	case "W":
		return true, m.write(l, p, true)

	default:
		return false, nil
	}
	return true, nil
}

// currentOr is the card c acts on: the one under the cursor. Unlike the
// others, setting a commander is about one card — a deck with four of them
// is a mistake you'd have to mean.
func currentOr(l *cardList) deck.Card {
	c, _ := l.current()
	return c
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

// info is the card under the cursor, in full.
func (l *cardList) info(width int) []string {
	c, ok := l.current()
	if !ok {
		return nil
	}
	return cardInfo(c, width, l.rules, l.rulings[c.Card.ID], l.rulingErr[c.Card.ID])
}

// keys is what this list offers, in two groups: moving about it, and picking
// cards out of it. The editing keys are not here: a, x, t, T, c and u act on
// the editing deck rather than on the list under the cursor, so hintGroups
// lists them against that deck, under its name.
func (l *cardList) keys() []hintGroup {
	nav := [][2]string{
		{"j k", "up/down"},
		{"gg G", "first/last"},
		{"o O", "sort"},
		{"/", "filter"},
	}

	sel := [][2]string{
		{"v V", "pick one/all"},
		{"y", "yank"},
	}
	// p puts into the list in front of you, so it only earns a hint when
	// that list is one of yours to write to.
	if l.deck != nil && l.deck.Local() {
		sel = append(sel, [2]string{"p", "put"})
	}
	if l.deck != nil && l.deck.Local() {
		sel = append(sel, [2]string{"w", "save"})
	} else {
		// Not yours, so writing it asks for a name and makes it yours.
		sel = append(sel, [2]string{"w W", "save as deck"})
	}
	sel = append(sel, [2]string{"gv", "text history"})

	return []hintGroup{{"navigation", nav}, {"select", sel}}
}
