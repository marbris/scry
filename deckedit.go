package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Editing the open deck. Every change is made against m.deckCards — the deck
// as a deck, which is what the file is written from — and the list beside it
// is rebuilt afterwards. Nothing here touches the disk directly: edits mark
// the deck dirty and a save follows a moment later, so tagging twelve cards
// in a row is one commit rather than twelve.

// deckSaveDelay is how long a deck sits unsaved before it's written. Long
// enough to collect a burst of edits into one commit, short enough that you
// won't get to the end of a thought before it lands.
const deckSaveDelay = 2 * time.Second

type deckSaveTickMsg struct{ seq int }

type deckSavedMsg struct {
	subject string
	warning string
	err     error
}

// ── Editing ─────────────────────────────────────────────────────

// editable reports whether the open deck can be changed, and says why not
// when it can't. A deck being browsed off Moxfield isn't yours to edit.
func (m model) editable() (bool, string) {
	switch {
	case m.deck == nil:
		return false, "no deck open — ,d to pick one, ,n for a new one"
	case !m.deck.local():
		return false, "this deck is Moxfield's — ,i to make it yours first"
	}
	return true, ""
}

// deckIndexOf finds a card in the open deck by name, or -1.
func (m model) deckIndexOf(name string) int {
	name = strings.ToLower(name)
	for i, dc := range m.deckCards {
		if strings.ToLower(dc.card.Name) == name {
			return i
		}
	}
	return -1
}

// selectedCard is the card under the cursor in whichever list has focus.
func (m model) selectedCard() (ScryfallCard, bool) {
	it, ok := m.active().selected()
	return it.card, ok
}

// addToDeck puts the selected card in the deck. A card already there isn't
// quietly doubled: nearly every deck this is built for is singleton, so a
// second copy has to be asked for with +.
func (m model) addToDeck() (tea.Model, tea.Cmd) {
	if ok, why := m.editable(); !ok {
		m.notice = why
		return m, nil
	}
	card, ok := m.selectedCard()
	if !ok {
		return m, nil
	}

	if i := m.deckIndexOf(card.Name); i >= 0 {
		m.notice = fmt.Sprintf("%s is already in the deck — + for another copy", card.Name)
		return m, nil
	}

	m.pushUndo("+1 " + card.Name)
	m.deckCards = append(m.deckCards, deckCard{card: card, qty: 1})
	m.notice = "+1 " + card.Name
	return m.deckChanged(card.Name)
}

// removeFromDeck takes the selected card out, however many copies. It works
// from either list, so a card you've just found in a search can be pulled
// back out without hunting for it in the deck.
func (m model) removeFromDeck() (tea.Model, tea.Cmd) {
	if ok, why := m.editable(); !ok {
		m.notice = why
		return m, nil
	}
	card, ok := m.selectedCard()
	if !ok {
		return m, nil
	}

	i := m.deckIndexOf(card.Name)
	if i < 0 {
		m.notice = card.Name + " isn't in the deck"
		return m, nil
	}

	// The cursor should land somewhere sensible once the row it was on is
	// gone, so aim at whatever follows.
	next := ""
	if m.focus == focusDeck {
		if items := m.deckPane.list.VisibleItems(); len(items) > 1 {
			at := m.deckPane.list.Index()
			if at+1 < len(items) {
				if ci, ok := items[at+1].(cardItem); ok {
					next = ci.card.Name
				}
			} else if ci, ok := items[at-1].(cardItem); ok {
				next = ci.card.Name
			}
		}
	}

	m.pushUndo("-" + card.Name)
	m.deckCards = append(m.deckCards[:i:i], m.deckCards[i+1:]...)
	m.notice = "-" + card.Name
	return m.deckChanged(next)
}

// changeQty adds or removes copies of the selected card. Dropping to zero
// takes it out of the deck, so - is a removal you can walk into gently.
func (m model) changeQty(delta int) (tea.Model, tea.Cmd) {
	if ok, why := m.editable(); !ok {
		m.notice = why
		return m, nil
	}
	card, ok := m.selectedCard()
	if !ok {
		return m, nil
	}

	i := m.deckIndexOf(card.Name)
	if i < 0 {
		if delta < 0 {
			m.notice = card.Name + " isn't in the deck"
			return m, nil
		}
		return m.addToDeck()
	}

	qty := m.deckCards[i].qty + delta
	if qty < 1 {
		return m.removeFromDeck()
	}

	m.pushUndo(fmt.Sprintf("%dx %s", m.deckCards[i].qty, card.Name))
	m.deckCards = append([]deckCard(nil), m.deckCards...)
	m.deckCards[i].qty = qty
	m.notice = fmt.Sprintf("%dx %s", qty, card.Name)
	return m.deckChanged(card.Name)
}

// toggleCommander marks the selected card as a commander, or unmarks it. A
// card that isn't in the deck is added as one, which is how a new deck
// starts: search for your commander and press c.
//
// Nothing here knows the rules. How many commanders a deck may have, and
// what may be one, is between you and your playgroup — this only records
// what you said, so several commanders and none are both fine.
func (m model) toggleCommander() (tea.Model, tea.Cmd) {
	if ok, why := m.editable(); !ok {
		m.notice = why
		return m, nil
	}
	card, ok := m.selectedCard()
	if !ok {
		return m, nil
	}

	i := m.deckIndexOf(card.Name)
	if i < 0 {
		m.pushUndo("+1 " + card.Name + " · commander")
		m.deckCards = append(m.deckCards, deckCard{card: card, qty: 1, commander: true})
		m.notice = "+1 " + card.Name + " · commander"
		return m.deckChanged(card.Name)
	}

	m.pushUndo(card.Name + " · commander")
	m.deckCards = append([]deckCard(nil), m.deckCards...)
	m.deckCards[i].commander = !m.deckCards[i].commander
	if m.deckCards[i].commander {
		m.notice = card.Name + " · commander"
	} else {
		m.notice = card.Name + " · no longer a commander"
	}
	return m.deckChanged(card.Name)
}

// setTags replaces one card's tags. Mass tagging in phase 7 works through
// this, a card at a time.
func (m *model) setTags(index int, tags []string) {
	m.deckCards = append([]deckCard(nil), m.deckCards...)
	m.deckCards[index].tags = tags
}

// ── Undo ────────────────────────────────────────────────────────

// undoDepth is how far back u goes. Deep enough to walk out of a mistake,
// shallow enough that a session's worth of tagging doesn't accumulate.
const undoDepth = 50

// undoStep is the deck as it was before one edit, and what that edit was.
type undoStep struct {
	cards []deckCard
	what  string
}

// pushUndo records the deck as it stands, before an edit changes it.
func (m *model) pushUndo(what string) {
	m.undo = append(m.undo, undoStep{
		cards: append([]deckCard(nil), m.deckCards...),
		what:  what,
	})
	if len(m.undo) > undoDepth {
		m.undo = m.undo[len(m.undo)-undoDepth:]
	}
}

// undoLast puts the deck back the way it was before the last edit. It goes
// through the same path as any other change, so the undo is written and
// committed like one — nothing is lost and the history says what happened.
func (m model) undoLast() (tea.Model, tea.Cmd) {
	if ok, why := m.editable(); !ok {
		m.notice = why
		return m, nil
	}
	if len(m.undo) == 0 {
		m.notice = "nothing to undo"
		return m, nil
	}

	step := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]

	// Undoing is not itself undoable — otherwise u alternates between two
	// states instead of walking back.
	m.deckCards = step.cards
	next, cmd := m.deckChanged("")
	out := next.(model)
	out.notice = "undo · " + step.what
	return out, cmd
}

// ── After an edit ───────────────────────────────────────────────

// keepOn is the card to leave the cursor on, so editing in the deck doesn't
// send you back to the top of it every time.
func (m model) deckChanged(keepOn string) (tea.Model, tea.Cmd) {
	total, unique := 0, 0
	for _, dc := range m.deckCards {
		total += dc.qty
		unique++
	}
	info := *m.deck
	info.total, info.unique = total, unique
	m.deck = &info

	// Remember the typed filter: rebuilding the list drops it, and losing
	// it on every keystroke would make editing a filtered deck unusable.
	filter := m.deckPane.list.FilterValue()
	m.deckPane.setItems(deckItems(m.deckCards))
	if filter != "" {
		m.deckPane.list.SetFilterText(filter)
	}
	if keepOn != "" {
		selectCard(&m.deckPane, keepOn)
	}

	m.deckDirty = true
	m.deckSeq++
	seq := m.deckSeq

	next, hoverCmd := m.syncHover()
	return next, tea.Batch(hoverCmd, tea.Tick(deckSaveDelay, func(time.Time) tea.Msg {
		return deckSaveTickMsg{seq: seq}
	}))
}

// selectCard puts the cursor on a named card, if it's still in the list.
func selectCard(p *pane, name string) {
	for i, it := range p.list.VisibleItems() {
		if ci, ok := it.(cardItem); ok && ci.card.Name == name {
			p.list.Select(i)
			return
		}
	}
}

// ── Saving ──────────────────────────────────────────────────────

// saveDeckNow writes the deck and commits it. Used both by the debounce and
// by anything that can't wait for it — quitting, most importantly.
func (m model) saveDeckNow() (model, tea.Cmd) {
	if m.deck == nil || !m.deck.local() {
		ok, why := m.editable()
		if !ok {
			m.notice = why
		}
		return m, nil
	}
	if !m.deckDirty {
		m.notice = "nothing to write — " + m.deck.slug + " is up to date"
		return m, nil
	}
	m.deckDirty = false

	slug := m.deck.slug
	file := deckFileFrom(*m.deck, m.deckCards)
	return m, func() tea.Msg {
		subject, warning, err := saveDeckVersioned(slug, file)
		return deckSavedMsg{subject: subject, warning: warning, err: err}
	}
}

// flushDeck writes a pending edit before the program exits. Bubbletea won't
// run a command after tea.Quit, so this one happens inline: an edit made two
// seconds before quitting is still an edit.
func (m *model) flushDeck() {
	if !m.deckDirty || m.deck == nil || !m.deck.local() {
		return
	}
	m.deckDirty = false
	_, _, _ = saveDeckVersioned(m.deck.slug, deckFileFrom(*m.deck, m.deckCards))
}

// quitAfterSaving is tea.Quit with any unsaved edit written first.
func (m model) quitAfterSaving() (tea.Model, tea.Cmd) {
	m.flushDeck()
	return m, tea.Quit
}

func (m model) handleDeckSaveTick(msg deckSaveTickMsg) (tea.Model, tea.Cmd) {
	// A later edit has restarted the clock, so this tick is stale. Nothing
	// pending means it's already been written, by w or by quitting.
	if msg.seq != m.deckSeq || !m.deckDirty {
		return m, nil
	}
	return m.saveDeckNow()
}

func (m model) handleDeckSaved(msg deckSavedMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.notice = "could not save: " + msg.err.Error()
	case msg.warning != "":
		m.notice = msg.warning
	}
	return m, nil
}
