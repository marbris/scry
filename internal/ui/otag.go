package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
	"ttr/internal/fetch"
	"ttr/internal/scryfall"
)

// tab in the i bar: tag the deck in front of you from Scryfall's oracle tags.
//
// Type "removal" and every card in the list that Scryfall tags
// otag:removal gets the tag otag-removal. It asks Scryfall one question per
// tag: otag:removal (!"Sol Ring" or !"Rancor" or …), over the list's own
// names, and tags whatever comes back. Names go in batches, so no one query
// grows longer than a URL should. A batch that matches more than a page's
// worth is paged through, which only happens when the first page came back
// full.

// otagQueryLen is how long one query is allowed to grow. Scryfall reads
// only the first 1024 characters of a query and drops the rest, which cuts
// off the closing parenthesis: "Your search contains unclosed parentheses."
// Lengths here are bytes, which are never fewer than characters.
const otagQueryLen = 1000

// otagPrefix is put in front of an oracle tag to make it one of yours.
const otagPrefix = "otag-"

// otagMsg carries the answer back: for each tag, the names in the list
// Scryfall gave it.
type otagMsg struct {
	panel int
	tags  []string
	hits  map[string][]string
	err   error
}

// otagNames is what was typed, as oracle tags: one per word, lowercased,
// with any otag: or otag- someone typed out of habit taken off.
func otagNames(input string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(input)) {
		w = strings.TrimPrefix(strings.TrimPrefix(w, "otag:"), otagPrefix)
		if w != "" && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// otagQueries is the queries that ask which of names Scryfall gives tag,
// each short enough to send.
func otagQueries(tag string, names []string) []string {
	var out []string
	head := "otag:" + tag + " ("
	var b strings.Builder
	for _, n := range names {
		term := fmt.Sprintf("!%q", n)
		if b.Len() > 0 && len(head)+b.Len()+len(" or ")+len(term)+1 > otagQueryLen {
			out = append(out, head+b.String()+")")
			b.Reset()
		}
		if b.Len() > 0 {
			b.WriteString(" or ")
		}
		b.WriteString(term)
	}
	if b.Len() > 0 {
		out = append(out, head+b.String()+")")
	}
	return out
}

// uniqueNames is each card in the list once, in the order they're kept.
func uniqueNames(cards []deck.Card) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range cards {
		k := strings.ToLower(c.Card.Name)
		if !seen[k] {
			seen[k] = true
			out = append(out, c.Card.Name)
		}
	}
	return out
}

// runOtag asks Scryfall, off the main thread, pausing between requests.
func runOtag(panelID int, tags, names []string) tea.Cmd {
	return func() tea.Msg {
		msg := otagMsg{panel: panelID, tags: tags, hits: map[string][]string{}}
		first := true
		for _, tag := range tags {
			for _, q := range otagQueries(tag, names) {
				if !first {
					time.Sleep(scryfall.PageDelay)
				}
				first = false
				cards, _, err := scryfall.Search(q, "name", "auto", 1<<20)
				if err != nil {
					msg.err = err
					return msg
				}
				for _, c := range cards {
					msg.hits[tag] = append(msg.hits[tag], c.Name)
				}
			}
		}
		return msg
	}
}

// sameCard reports whether a name Scryfall gave back is this card. A card
// with two faces may be kept by its front face alone.
func sameCard(kept, found string) bool {
	if strings.EqualFold(kept, found) {
		return true
	}
	front, _, ok := strings.Cut(found, " // ")
	return ok && strings.EqualFold(kept, front)
}

// handleOtag tags the list that asked, if it is still there and still yours.
func (m Model) handleOtag(msg otagMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	l := p.cardsView()
	if l == nil || l.deck == nil || !l.deck.Local() {
		return m, nil
	}
	if msg.err != nil {
		if _, ok := msg.err.(fetch.NotFound); !ok {
			m.notice = "error: " + msg.err.Error()
			return m, nil
		}
	}

	l.pushUndo("tag by otag")
	var said []string
	changed := false
	for _, tag := range msg.tags {
		mine := otagPrefix + tag
		n := 0
		for i := range l.all {
			for _, found := range msg.hits[tag] {
				if sameCard(l.all[i].Card.Name, found) {
					before := len(l.all[i].Tags)
					l.all[i].Tags = deck.ApplyTagEdits(l.all[i].Tags, []string{mine}, nil)
					if len(l.all[i].Tags) != before {
						changed = true
					}
					n++
					break
				}
			}
		}
		said = append(said, mine+": "+itoa(n)+" of "+itoa(len(l.all)))
	}
	if !changed {
		l.undoLast()
	}
	l.refresh()
	m.notice = strings.Join(said, " · ")
	return m, m.autosave()
}
