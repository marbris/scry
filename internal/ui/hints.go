package ui

// What works right now.
//
// One list, used by the hint bar along the bottom and by the reference `?`
// raises. They used to be two: the reference asked each view what its keys
// were, and the bar had three hardcoded strings picked by a three-way
// switch. So the bar offered `s statistics` in a decks panel, where there is
// nothing to count, and offered `a/x/t` on a deck borrowed from Moxfield,
// where all three refuse — while saying nothing at all about `a`, `y`, `w`
// or `gv` on a search result, which fell through to the default branch.
//
// A hint that lies is worse than no hint, and two lists describing one
// keymap is how one of them comes to lie. There is one now: hintGroups.
//
// The keys are grouped — navigation, select, edit, info panel — so the bar
// can lead each line with what it is rather than running the whole keymap
// together. Each view declares its own groups; the workspace folds in the
// keys it owns everywhere (the bar, the edit target, the information panel,
// and the way between and out of panels), so no view repeats them and none
// can offer esc under two different names.

// contextKeys is every key that does something where you are, flattened out
// of the groups for the reference, which lays them out to be read rather
// than skimmed.
func (m Model) contextKeys() [][2]string {
	var out [][2]string
	for _, g := range m.hintGroups() {
		out = append(out, g.keys...)
	}
	return out
}

// hintGroups is the grouped keymap for where you are: the source both the
// hint bar and the reference draw from.
func (m Model) hintGroups() []hintGroup {
	p := m.ws.current()
	if p == nil {
		return []hintGroup{{"", [][2]string{
			{"space", "menu"},
			{"?", "keys"},
			{"q", "quit"},
		}}}
	}

	// While the bar has the cursor it has every key, so nothing else is
	// worth offering: a printable key types rather than acting.
	if p.searchOpen && p.search.Focused() {
		return []hintGroup{{"search", m.barKeys(p)}}
	}

	// The view's own groups first — navigation, and whatever the view does.
	// While the statistics have the keys, they are the view.
	var groups []hintGroup
	statsUp := m.info.mode == infoStats && m.statList() != nil
	if statsUp {
		groups = append(groups, hintGroup{"statistics", statsHints})
	} else if v := p.top(); v != nil {
		groups = append(groups, v.keys()...)
	}

	// The search bar lives in the navigation group: i is how you reach it.
	groups = addHints(groups, "navigation", [2]string{"i", p.kind.barLabel()})

	// b clears the narrowings, but only earns a hint while there is one to
	// clear — otherwise it is a key that does nothing, offered next to esc.
	if m.focusNarrowed() {
		groups = addHints(groups, "navigation", [2]string{"b", "clear filter"})
	}

	// The editing keys, against the deck they change — which is routinely
	// not the list under the cursor.
	if title, keys := m.editGroup(p); len(keys) > 0 {
		groups = append(groups, hintGroup{title, keys})
	}

	// The information panel, when it holds something these keys move within.
	if keys := m.infoKeys(p); len(keys) > 0 && !statsUp {
		groups = addHints(groups, "info panel", keys...)
	}

	// The tail of navigation: between panels, out of the panel, the menu.
	var tail [][2]string
	if m.ws.count() > 1 {
		tail = append(tail,
			[2]string{"h l", "next/prev panel"},
			[2]string{"ctrl+h/l", "move panel"},
		)
	}
	if len(p.stack) > 1 {
		tail = append(tail, [2]string{"esc", "back"})
	} else {
		tail = append(tail, [2]string{"esc", "clear/close"})
	}
	tail = append(tail,
		[2]string{"space", "menu"},
		[2]string{"?", "keys"},
		[2]string{"q", "quit"},
	)
	groups = addHints(groups, "navigation", tail...)

	return groups
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

	into := target.deck.Name
	if m.ws.editing == m.ws.focused {
		into = "this deck"
	}
	return "edit · " + into, [][2]string{
		{"a A", "add / add + tag"},
		{"t x", "tag / remove"},
		{"c", "commander"},
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
	return append([][2]string{{"s", "stats"}},
		[2]string{"K J", "up/down"},
		[2]string{"ctrl+k/j", "paragraph"},
	)
}

// statsHints are the keys while the statistics have them.
var statsHints = [][2]string{
	{"j k", "category"},
	{"J K", "turn groups"},
	{"a o", "filter and/or"},
	{"x", "remove from filter"},
	{"b", "clear filter"},
	{"p P", "odds"},
	{"esc", "back"},
	{"s", "close"},
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
