package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tagging in bulk, which is the point of tags: filter the deck to what you
// mean — "flying", "draw", anything the fuzzy filter matches on name or
// oracle text — mark that lot, and give them all a tag in one go.
//
// Marks are held by card name rather than by list position, so filtering,
// sorting or editing between marking and tagging doesn't move them onto the
// wrong cards.

// ── Marks ───────────────────────────────────────────────────────

// markKey is what a mark is filed under. Names, so a mark survives the list
// being rebuilt beneath it.
func markKey(c ScryfallCard) string { return strings.ToLower(c.Name) }

func (m model) marked(c ScryfallCard) bool { return m.marks[markKey(c)] }

func (m model) markCount() int { return len(m.marks) }

// toggleMark marks or unmarks the card under the cursor and steps down, so
// marking a run of cards is one key held down.
func (m model) toggleMark() (tea.Model, tea.Cmd) {
	card, ok := m.selectedCard()
	if !ok {
		return m, nil
	}

	if m.marks == nil {
		m.marks = map[string]bool{}
	} else {
		m.marks = cloneMarks(m.marks)
	}

	key := markKey(card)
	if m.marks[key] {
		delete(m.marks, key)
	} else {
		m.marks[key] = true
	}

	p := m.active()
	if at := p.list.Index(); at+1 < len(p.list.VisibleItems()) {
		p.list.Select(at + 1)
	}
	next, cmd := m.syncHover()
	return next, cmd
}

// markVisible marks every card the list is currently showing — which is
// what a typed filter leaves behind, and the reason this exists: filter to
// flying, press v, tag the lot.
func (m model) markVisible() (tea.Model, tea.Cmd) {
	items := m.active().list.VisibleItems()
	if len(items) == 0 {
		return m, nil
	}

	m.marks = cloneMarks(m.marks)
	added := 0
	for _, it := range items {
		ci, ok := it.(cardItem)
		if !ok {
			continue
		}
		if !m.marks[markKey(ci.Card)] {
			m.marks[markKey(ci.Card)] = true
			added++
		}
	}
	m.notice = fmt.Sprintf("marked %d (%d in all)", added, len(m.marks))
	return m, nil
}

// clearMarks drops every mark.
func (m model) clearMarks() (tea.Model, tea.Cmd) {
	if len(m.marks) == 0 {
		return m, nil
	}
	m.notice = fmt.Sprintf("cleared %d marks", len(m.marks))
	m.marks = nil
	return m, nil
}

func cloneMarks(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ── The tag prompt ──────────────────────────────────────────────

// targets is what a tag will be applied to: the marked cards, or the one
// under the cursor when nothing is marked. Deliberately not "everything the
// filter is showing" — that's what v is for, and tagging a hundred cards
// because a filter was still on would be a nasty surprise.
func (m model) tagTargets() []string {
	if len(m.marks) > 0 {
		out := make([]string, 0, len(m.marks))
		for k := range m.marks {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	if card, ok := m.selectedCard(); ok {
		return []string{markKey(card)}
	}
	return nil
}

func (m model) openTagPrompt() (tea.Model, tea.Cmd) {
	if ok, why := m.editable(); !ok {
		m.notice = why
		return m, nil
	}
	if len(m.tagTargets()) == 0 {
		m.notice = "nothing to tag"
		return m, nil
	}

	m.tagging = true
	m.tagInput = newTagInput(len(m.tagTargets()))
	return m, textinput.Blink
}

func newTagInput(n int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "tag, or -tag to remove"
	ti.Prompt = fmt.Sprintf("Tag %s: ", plural(fmt.Sprintf("%d card", n), n))
	ti.CharLimit = 60
	ti.Width = 40
	ti.Focus()
	ti.PromptStyle = lipgloss.NewStyle().Foreground(gruvAqua).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(gruvFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(gruvGray)
	return ti
}

func (m model) updateTagging(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tagging = false
		return m, nil

	case "enter":
		m.tagging = false
		return m.applyTags(m.tagInput.Value())
	}

	var cmd tea.Cmd
	m.tagInput, cmd = m.tagInput.Update(msg)
	return m, cmd
}

// applyTags adds or removes tags across the targets. A leading "-" removes,
// so one key and one prompt do both — and t stays printed-text history.
// Several tags at once are comma-separated, the way they read in the file.
func (m model) applyTags(input string) (tea.Model, tea.Cmd) {
	targets := m.tagTargets()
	if len(targets) == 0 {
		return m, nil
	}

	var add, remove []string
	for _, raw := range strings.Split(input, ",") {
		t := strings.ToLower(strings.TrimSpace(raw))
		if strings.HasPrefix(t, "-") {
			if t = strings.TrimSpace(t[1:]); t != "" {
				remove = append(remove, t)
			}
			continue
		}
		if t != "" {
			add = append(add, t)
		}
	}
	if len(add) == 0 && len(remove) == 0 {
		return m, nil
	}

	inTargets := map[string]bool{}
	for _, t := range targets {
		inTargets[t] = true
	}

	// One snapshot for the batch, so u undoes the whole tagging rather than
	// one card of it.
	m.pushUndo(tagUndoLabel(add, remove, len(targets)))

	changed, missing := 0, 0
	for i := range m.deckCards {
		if !inTargets[markKey(m.deckCards[i].Card)] {
			continue
		}
		before := strings.Join(m.deckCards[i].Tags, ",")
		tags := applyTagEdits(m.deckCards[i].Tags, add, remove)
		if strings.Join(tags, ",") != before {
			m.setTags(i, tags)
			changed++
		}
	}
	// A marked card that isn't in the deck can't carry a tag: tags live in
	// the deck file, so there's nowhere to put one.
	for _, t := range targets {
		if m.deckIndexOfKey(t) < 0 {
			missing++
		}
	}

	m.notice = tagNotice(changed, missing, add, remove)
	if changed == 0 {
		// Nothing happened, so there is nothing to undo.
		m.undo = m.undo[:len(m.undo)-1]
		return m, nil
	}

	// Keep the cursor where it is: tagging doesn't reorder anything, but
	// deckChanged rebuilds the list underneath it.
	on := ""
	if card, ok := m.selectedCard(); ok {
		on = card.Name
	}
	return m.deckChanged(on)
}

func (m model) deckIndexOfKey(key string) int {
	for i, dc := range m.deckCards {
		if markKey(dc.Card) == key {
			return i
		}
	}
	return -1
}

func tagUndoLabel(add, remove []string, n int) string {
	var what []string
	for _, t := range add {
		what = append(what, "+"+t)
	}
	for _, t := range remove {
		what = append(what, "-"+t)
	}
	return fmt.Sprintf("%s on %d %s", strings.Join(what, " "), n, plural("card", n))
}

func tagNotice(changed, missing int, add, remove []string) string {
	var what []string
	for _, t := range add {
		what = append(what, "+"+t)
	}
	for _, t := range remove {
		what = append(what, "-"+t)
	}
	label := strings.Join(what, " ")

	switch {
	case changed == 0 && missing > 0:
		return fmt.Sprintf("%s: nothing changed (%d not in the deck)", label, missing)
	case changed == 0:
		return label + ": nothing changed"
	case missing > 0:
		return fmt.Sprintf("%s on %d %s (%d not in the deck)",
			label, changed, plural("card", changed), missing)
	}
	return fmt.Sprintf("%s on %d %s", label, changed, plural("card", changed))
}
