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
// keymap is how one of them comes to lie. There is one now.

// contextKeys is every key that does something where you are, in the order
// it reads: moving about the list, then what you can do to it, then the deck
// those edits land in, then the panel itself.
func (m Model) contextKeys() [][2]string {
	p := m.ws.current()
	if p == nil {
		return [][2]string{
			{"space", "the menu"},
			{"?", "these keys"},
			{"q", "quit"},
		}
	}

	// While the bar has the cursor it has every key, so nothing else is
	// worth offering: a printable key types rather than acting.
	if p.searchOpen && p.search.Focused() {
		return m.barKeys(p)
	}

	var out [][2]string
	if v := p.top(); v != nil {
		out = append(out, v.keys()...)
	}
	out = append(out, m.editKeys(p)...)
	out = append(out, m.panelKeys(p)...)
	return out
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

// editKeys are the keys that change a deck. They are listed against the deck
// they change, which is not the list you are looking at: a, x, t, T, c and u
// all act on the *editing* deck from whatever panel you are in. Reading
// "add" while looking at somebody else's deck and watching the card land
// somewhere else is the confusion this names away.
func (m Model) editKeys(p *panel) [][2]string {
	l := p.cardsView()
	if l == nil {
		return nil // a decks panel, the rules, a list of versions
	}

	out := [][2]string{
		{"s", "statistics"},
		{"y", "yank the selection"},
	}

	// Where the edits go. Named, because "the deck you're editing" is only
	// useful if you can see which deck that is — and with no deck chosen,
	// none of these keys does anything but explain itself, so they are not
	// offered at all. What is offered then is the way to choose one.
	target := m.ws.editingList()
	if target == nil || target.deck == nil {
		return append(out,
			[2]string{"c", "start a deck with this commander"},
			[2]string{"e E", "choose a deck to edit"},
		)
	}

	into := target.deck.Name
	if m.ws.editing == m.ws.focused {
		into = "this deck"
	}
	out = append(out,
		[2]string{"a x", "add, remove a copy — " + into},
		[2]string{"t T", "tag, tag again — " + into},
		[2]string{"c", "commander — " + into},
		[2]string{"u", "undo — " + into},
	)
	// p is the exception in this group: it puts into the list in front of
	// you, which is why it needs that list to be one of yours.
	if l.deck != nil && l.deck.Local() {
		out = append(out, [2]string{"p", "put them here"})
	}
	return append(out, [2]string{"e E", m.editingLabel()})
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
		return "edit the next, previous deck"
	}
	return "choose the deck to edit"
}

// panelKeys are the panel and the workspace: the bar, the way out, and the
// keys that are the same wherever you are.
func (m Model) panelKeys(p *panel) [][2]string {
	out := [][2]string{{"i", p.kind.barLabel()}}

	// The information panel is only worth naming when it holds something
	// those keys move within.
	switch {
	case m.info.mode == infoStats:
		out = append(out,
			[2]string{"K J", "previous, next category"},
			[2]string{"ctrl+k/j", "previous, next group"},
		)
	case p.cardsView() != nil:
		out = append(out,
			[2]string{"K J", "move in the info panel"},
			[2]string{"ctrl+k/j", "scroll it"},
		)
	}

	if m.ws.count() > 1 {
		out = append(out, [2]string{"h l", "previous, next panel"})
	}
	return append(out,
		[2]string{"esc", "clear, then close"},
		[2]string{"space", "the menu"},
		[2]string{"?", "these keys"},
		[2]string{"q", "quit"},
	)
}
