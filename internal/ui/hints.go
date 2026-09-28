package ui

import (
	"strings"

	"scry/internal/keymap"
)

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

// hint is one entry for the hint bar: the keys some actions are on, and
// what they do. The keys come from the keymap, so they are the ones in force.
func hint(what string, s keymap.Scope, actions ...keymap.Action) [2]string {
	return [2]string{keymap.Hint(s, actions...), what}
}

// listHint is a hint for the movement every list shares.
func listHint(what string, actions ...keymap.Action) [2]string {
	return hint(what, keymap.List, actions...)
}

// gotoHint is a g-prefixed key as it is typed: "gv".
func gotoHint(a keymap.Action) string {
	return seqHint(keymap.Hint(keymap.Global, keymap.GlobalGoto), "", keymap.Hint(keymap.Goto, a))
}

// leaderHint is a leader key as the hints write it: "space s".
func leaderHint(a keymap.Action) string {
	return seqHint(keymap.Hint(keymap.Global, keymap.GlobalLeader), " ", keymap.Hint(keymap.Leader, a))
}

// topBottomHint is the pair for the ends of a list: "gg G".
func topBottomHint() string {
	top := gotoHint(keymap.GotoTop)
	bottom := keymap.Hint(keymap.List, keymap.ListBottom)
	if top == "" || bottom == "" {
		return top + bottom
	}
	return top + " " + bottom
}

// seqHint is a prefix and the key after it, or nothing if either is unbound:
// half a sequence can't be typed.
func seqHint(prefix, sep, key string) string {
	if prefix == "" || key == "" {
		return ""
	}
	return prefix + sep + key
}

// dropUnbound takes out the hints for keys that have been unbound in
// keys.json, and any group left empty by it.
func dropUnbound(groups []hintGroup) []hintGroup {
	var out []hintGroup
	for _, g := range groups {
		var keys [][2]string
		for _, k := range g.keys {
			if k[0] != "" {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			out = append(out, hintGroup{g.title, keys})
		}
	}
	return out
}

// restingKeys are the keys the footer always shows: the way to everything.
func restingKeys() [][2]string {
	return [][2]string{
		hint("menu", keymap.Global, keymap.GlobalLeader),
		hint("keys", keymap.Global, keymap.GlobalHelp),
		hint("quit", keymap.Global, keymap.GlobalQuit),
	}
}

// hintGroups is the whole keymap for where you are: the focused panel's
// keys, the information panel's, and the footer's. The panels draw their
// shares of it; this is the union, for the tests that check them against
// each other.
func (m Model) hintGroups() []hintGroup {
	p := m.ws.current()
	if p == nil {
		return dropUnbound([]hintGroup{{"", restingKeys()}})
	}

	// While the bar has the cursor it has every key, so nothing else is
	// worth offering: a printable key types rather than acting.
	if p.searchOpen && p.search.Focused() {
		return dropUnbound([]hintGroup{{"search", m.barKeys(p)}})
	}

	groups := m.panelHintGroups(p)
	groups = append(groups, m.infoHintGroups()...)
	return dropUnbound(addHints(groups, "navigation", restingKeys()...))
}

// statsTaken is every key the statistics claim while they're up, as the
// hints write them. The list behind them still has its other keys, so its
// hints stay — less these.
func statsTaken() map[string]bool {
	taken := map[string]bool{}
	for _, e := range keymap.Current() {
		if e.Scope == keymap.Stats {
			for _, k := range e.Keys {
				taken[keymap.Display(k)] = true
			}
		}
	}
	return taken
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
		groups = addHints(groups, "navigation", hint(i, keymap.Global, keymap.GlobalBar))
	}

	// b clears the narrowings, but only earns a hint while there is one to
	// clear — otherwise it is a key that does nothing, offered next to esc.
	if m.focusNarrowed() {
		groups = addHints(groups, "navigation", hint("clear filters", keymap.Global, keymap.GlobalClearFilter))
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
			hint("left/right panel", keymap.Global, keymap.GlobalPanelPrev, keymap.GlobalPanelNext),
			hint("move panel", keymap.Global, keymap.GlobalMoveLeft, keymap.GlobalMoveRight),
		)
	}
	esc, _ := (&m).escStep(p)
	tail = append(tail, hint(esc, keymap.Global, keymap.GlobalBack))
	groups = addHints(groups, "navigation", tail...)

	if statsUp {
		groups = withoutTaken(groups)
	}
	return dropUnbound(groups)
}

// withoutTaken drops the keys the statistics have claimed, and any group
// left with nothing in it.
func withoutTaken(groups []hintGroup) []hintGroup {
	statsTaken := statsTaken()
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
		return dropUnbound([]hintGroup{{"statistics", statsHints()}})
	}
	// Untitled: the keys sit in the panel they move, so a heading naming
	// that panel says nothing.
	if keys := m.infoKeys(p); len(keys) > 0 {
		return dropUnbound([]hintGroup{{"", keys}})
	}
	// A view that reads its own rows into the panel — a rule, a change —
	// declares how to move through them.
	if v := p.top(); v != nil {
		for _, g := range v.keys() {
			if g.title == "info panel" {
				return dropUnbound([]hintGroup{{"", g.keys}})
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
	out := [][2]string{hint("another target: scryfall, decks, rules", keymap.Search, keymap.SearchNextTarget)}
	if p.kind == KindFind {
		out = append(out,
			hint("queries you've run", keymap.Search, keymap.SearchHistoryPrev, keymap.SearchHistoryNext),
			hint("order the results", keymap.Search, keymap.SearchQuerySort),
		)
	}
	return append(out,
		hint(p.kind.prompt(), keymap.Search, keymap.SearchRun),
		hint("leave the bar", keymap.Search, keymap.SearchBack),
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

	editKeys := hint("", keymap.Global, keymap.GlobalEditNext, keymap.GlobalEditPrev)[0]
	target := m.ws.editingList()
	if target == nil || target.deck == nil {
		return "edit", [][2]string{{editKeys, "choose a deck to edit"}}
	}

	if m.ws.editing == m.ws.focused {
		return "edit · this deck", [][2]string{
			hint("add a copy", keymap.Cards, keymap.CardsAdd),
			hint("add + tag latest", keymap.Cards, keymap.CardsAddTagged),
			hint("remove", keymap.Cards, keymap.CardsRemove),
			hint("commander", keymap.Cards, keymap.CardsCommander),
			hint("undo", keymap.Cards, keymap.CardsUndo),
			{editKeys, m.editingLabel()},
		}
	}
	return "edit · " + target.deck.Name, [][2]string{
		hint("add to deck", keymap.Cards, keymap.CardsAdd),
		hint("add + tag latest", keymap.Cards, keymap.CardsAddTagged),
		hint("remove from deck", keymap.Cards, keymap.CardsRemove),
		hint("commander of deck", keymap.Cards, keymap.CardsCommander),
		hint("undo", keymap.Cards, keymap.CardsUndo),
		{editKeys, m.editingLabel()},
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
		hint("stats", keymap.Global, keymap.GlobalStats),
		{leaderHint(keymap.LeaderStats), "editing deck stats"},
		hint("half page", keymap.Global, keymap.GlobalInfoUp, keymap.GlobalInfoDown),
		hint("paragraph", keymap.Global, keymap.GlobalInfoParaUp, keymap.GlobalInfoParaDn),
	}
}

// statsHints are the keys while the statistics have them.
func statsHints() [][2]string {
	return [][2]string{
		hint("category", keymap.Stats, keymap.StatsDown, keymap.StatsUp),
		hint("turn groups", keymap.Stats, keymap.StatsNextGroup, keymap.StatsPrevGroup),
		hint("filter and/or/not", keymap.Stats, keymap.StatsAnd, keymap.StatsOr, keymap.StatsNot),
		hint("remove from filter", keymap.Stats, keymap.StatsDrop),
		hint("clear stats-filter", keymap.Stats, keymap.StatsClear),
		hint("odds", keymap.Stats, keymap.StatsOddsNext, keymap.StatsOddsPrev),
		hint("close", keymap.Stats, keymap.StatsClose),
		hint("back", keymap.Stats, keymap.StatsBack),
	}
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
