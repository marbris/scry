package ui

import "strings"

// What works right now.
//
// One source for every hint on screen. There used to be two: the reference asked each view what its keys
// were, and the bar had three hardcoded strings picked by a three-way
// switch. So the bar offered `s statistics` in a decks panel, where there is
// nothing to count, and offered `a/x/t` on a deck borrowed from Moxfield,
// where all three refuse — while saying nothing at all about `a`, `y`, `w`
// or `gv` on a search result, which fell through to the default branch.
//
// A hint that lies is worse than no hint, and two lists describing one
// keymap is how one of them comes to lie. There is one now: hintGroups.
//
// The keys are grouped — navigation, select, edit, info panel — so each
// block can lead each line with what it is rather than running the whole
// keymap together. Each view declares its own groups; the workspace folds in the
// keys it owns everywhere (the bar, the edit target, the information panel,
// and the way between and out of panels), so no view repeats them and none
// can offer esc under two different names.

// With ? on, the keys are drawn where they act: at the bottom of the focused
// panel, the keys for that panel; at the bottom of the information panel, the
// keys for what it shows. The footer keeps only the three that reach
// everything else — and, while a search bar has the cursor, the bar's own.

// contextKeys is every key that does something where you are, flattened out
// of the groups.
func (m Model) contextKeys() [][2]string {
	var out [][2]string
	for _, g := range m.hintGroups() {
		out = append(out, g.keys...)
	}
	return out
}

// restingKeys are the keys the footer always shows: the way to everything.
var restingKeys = [][2]string{
	{"space", "menu"},
	{"?", "keys"},
	{"q", "quit"},
}

// hintGroups is the whole keymap for where you are: the focused panel's
// keys, the information panel's, and the footer's. The panels draw their
// shares of it; this is the union, for the tests that check them against
// each other.
func (m Model) hintGroups() []hintGroup {
	p := m.ws.current()
	if p == nil {
		return []hintGroup{{"", restingKeys}}
	}

	// While the bar has the cursor it has every key, so nothing else is
	// worth offering: a printable key types rather than acting.
	if p.searchOpen && p.search.Focused() {
		return []hintGroup{{"search", m.barKeys(p)}}
	}

	groups := m.panelHintGroups(p)
	groups = append(groups, m.infoHintGroups()...)
	return addHints(groups, "navigation", restingKeys...)
}

// statsTaken is every key the statistics claim while they're up. The list
// behind them still has its other keys, so its hints stay — less these.
var statsTaken = map[string]bool{
	"j": true, "k": true, "J": true, "K": true,
	"a": true, "o": true, "O": true, "n": true, "x": true, "b": true,
	"p": true, "P": true, "s": true, "esc": true,
}

// panelHintGroups is what the focused panel shows at its bottom: its own
// keys, the editing keys against the deck they change, and the way between
// and out of panels.
func (m Model) panelHintGroups(p *panel) []hintGroup {
	statsUp := m.info.mode == infoStats && m.statList() != nil

	var groups []hintGroup
	if v := p.top(); v != nil {
		for _, g := range v.keys() {
			// The information panel's keys are drawn in that panel.
			if g.title != "info panel" {
				groups = append(groups, hintGroup{g.title, append([][2]string(nil), g.keys...)})
			}
		}
	}

	// i reaches the panel's search bar — except on a deck, where it fetches
	// a card into it, and on somebody else's deck, where it does neither.
	if i, ok := m.iHint(p); ok {
		groups = addHints(groups, "navigation", [2]string{"i", i})
	}

	// b clears the narrowings, but only earns a hint while there is one to
	// clear — otherwise it is a key that does nothing, offered next to esc.
	if m.focusNarrowed() {
		groups = addHints(groups, "navigation", [2]string{"b", "clear filters"})
	}

	// The editing keys, against the deck they change — which is routinely
	// not the list under the cursor.
	if title, keys := m.editGroup(p); len(keys) > 0 {
		groups = append(groups, hintGroup{title, keys})
	}

	// The tail of navigation: between panels, and out of this one.
	var tail [][2]string
	if m.ws.count() > 1 {
		tail = append(tail,
			[2]string{"h l", "left/right panel"},
			[2]string{"ctrl+h/l", "move panel"},
		)
	}
	esc, _ := (&m).escStep(p)
	tail = append(tail, [2]string{"esc", esc})
	groups = addHints(groups, "navigation", tail...)

	if statsUp {
		groups = withoutTaken(groups)
	}
	return groups
}

// withoutTaken drops the keys the statistics have claimed, and any group
// left with nothing in it.
func withoutTaken(groups []hintGroup) []hintGroup {
	var out []hintGroup
	for _, g := range groups {
		var keys [][2]string
		for _, k := range g.keys {
			taken := false
			for _, f := range strings.Fields(k[0]) {
				if statsTaken[f] {
					taken = true
				}
			}
			if !taken {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			out = append(out, hintGroup{g.title, keys})
		}
	}
	return out
}

// iHint is what i does in this panel, or false where it does nothing.
func (m Model) iHint(p *panel) (string, bool) {
	if l := p.cardsView(); l != nil && l.deck != nil {
		if !l.deck.Local() {
			return "", false
		}
		return "add from scryfall", true
	}
	return p.kind.barLabel(), true
}

// infoHintGroups is what the information panel shows at its bottom: the
// statistics' keys while they're up, otherwise the keys that move within
// what it shows.
func (m Model) infoHintGroups() []hintGroup {
	p := m.ws.current()
	if p == nil || (p.searchOpen && p.search.Focused()) {
		return nil
	}
	if m.info.mode == infoStats && m.statList() != nil {
		return []hintGroup{{"statistics", statsHints}}
	}
	// Untitled: the keys sit in the panel they move, so a heading naming
	// that panel says nothing.
	if keys := m.infoKeys(p); len(keys) > 0 {
		return []hintGroup{{"", keys}}
	}
	// A view that reads its own rows into the panel — a rule, a change —
	// declares how to move through them.
	if v := p.top(); v != nil {
		for _, g := range v.keys() {
			if g.title == "info panel" {
				return []hintGroup{{"", g.keys}}
			}
		}
	}
	return nil
}

// focusNarrowed reports whether the focused list has a narrowing b would
// clear: a text filter, or — for a card list — the statistics category too.
func (m Model) focusNarrowed() bool {
	p := m.ws.current()
	if p == nil {
		return false
	}
	if l := p.cardsView(); l != nil {
		return l.filter != "" || len(l.statFilter) > 0
	}
	switch v := p.top().(type) {
	case *deckList:
		return v.filter != ""
	case *versionList:
		return v.filter != ""
	case *rulesView:
		return v.filter != ""
	}
	return false
}

// addHints appends keys to the group with the given title, making it at the
// end if there isn't one yet.
func addHints(groups []hintGroup, title string, keys ...[2]string) []hintGroup {
	for i := range groups {
		if groups[i].title == title {
			groups[i].keys = append(groups[i].keys, keys...)
			return groups
		}
	}
	return append(groups, hintGroup{title, keys})
}

// barKeys is the search bar's own keymap. tab only means something where
// there is another kind to become, and the history and the ordering belong
// to a card query — the bar that follows a Moxfield user has neither.
func (m Model) barKeys(p *panel) [][2]string {
	out := [][2]string{{"tab", "another target: scryfall, decks, rules"}}
	if p.kind == KindFind {
		out = append(out,
			[2]string{"↑ ↓", "queries you've run"},
			[2]string{"ctrl+o", "order the results"},
		)
	}
	return append(out,
		[2]string{"enter", p.kind.prompt()},
		[2]string{"esc", "leave the bar"},
	)
}

// editGroup is the keys that change a deck, and the name of the deck they
// change. They act on the *editing* deck from whatever panel you are in — so
// the deck they change is routinely not the list in front of you, which is
// exactly why the group is headed with its name.
//
// Only offered where the focused panel is a list of cards: a, x, t and the
// rest do nothing from a decks panel or the rules, so they are not named
// there. With no deck being edited they explain themselves away and are
// replaced by the way to choose one.
func (m Model) editGroup(p *panel) (string, [][2]string) {
	if p.cardsView() == nil {
		return "", nil
	}

	target := m.ws.editingList()
	if target == nil || target.deck == nil {
		return "edit", [][2]string{{"e E", "choose a deck to edit"}}
	}

	if m.ws.editing == m.ws.focused {
		return "edit · this deck", [][2]string{
			{"a", "add a copy"},
			{"A", "add + tag latest"},
			{"x", "remove"},
			{"c", "commander"},
			{"u", "undo"},
			{"e E", m.editingLabel()},
		}
	}
	return "edit · " + target.deck.Name, [][2]string{
		{"a", "add to deck"},
		{"A", "add + tag latest"},
		{"x", "remove from deck"},
		{"c", "commander of deck"},
		{"u", "undo"},
		{"e E", m.editingLabel()},
	}
}

// infoKeys are the information-panel keys for a card list — statistics, and
// the two axes of moving through what the panel shows. The rules panel
// declares its own, since what K and J read there is a rule, not a card.
func (m Model) infoKeys(p *panel) [][2]string {
	if p.cardsView() == nil {
		return nil
	}
	return [][2]string{
		{"s", "stats"},
		{"space s", "editing deck stats"},
		{"K J", "up/down"},
		{"ctrl+k/j", "paragraph"},
	}
}

// statsHints are the keys while the statistics have them.
var statsHints = [][2]string{
	{"j k", "category"},
	{"J K", "turn groups"},
	{"a o n", "filter and/or/not"},
	{"x", "remove from filter"},
	{"b", "clear stats-filter"},
	{"p P", "odds"},
	{"s", "close"},
	{"esc", "back"},
}

// editingLabel says what e and E will do, which depends on whether there is
// anywhere else for them to go. "the next deck" with one deck open names a
// deck that isn't there.
func (m Model) editingLabel() string {
	n := 0
	for i := range m.ws.panels {
		if m.ws.editable(i) {
			n++
		}
	}
	if n > 1 {
		return "next/prev deck"
	}
	return "choose deck"
}
