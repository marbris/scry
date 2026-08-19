package ui

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/theme"
)

// The decks panel: everything you can open, in one list.
//
// Three kinds of thing live here and the design is deliberate about keeping
// them together rather than in three panes. They answer the same question —
// what can I look at? — and the differences between them are one letter
// wide:
//
//	L  a local deck, a file you can edit
//	R  a remote deck on Moxfield, which you can take a copy of
//	U  somebody whose decks you can look through
//
// The row is name, kind, legality, size and age. In a narrow panel the tail
// is given up from the right, because a name with no age beside it is still
// a name, and an age with no name is nothing.

type entryKind int

const (
	entryLocal entryKind = iota
	entryRemote
	entryUser
)

func (k entryKind) letter() string {
	switch k {
	case entryRemote:
		return "R"
	case entryUser:
		return "U"
	}
	return "L"
}

// deckEntry is one row.
type deckEntry struct {
	kind entryKind
	name string
	// slug identifies a local deck; id identifies a remote one; user is the
	// Moxfield name. Exactly one is set.
	slug string
	id   string
	user string

	format   string
	count    int // cards for a deck, decks for a user
	modified time.Time
	// legal is nil when nothing has worked it out yet, which is every local
	// deck until the legality engine lands.
	legal  *bool
	broken bool
}

type deckListSort int

const (
	byModified deckListSort = iota
	byName
	bySize
	byKind
)

func (s deckListSort) String() string {
	switch s {
	case byName:
		return "name"
	case bySize:
		return "size"
	case byKind:
		return "kind"
	}
	return "last touched"
}

type deckList struct {
	cursor
	all  []deckEntry
	rows []deckEntry

	order  deckListSort
	filter string

	// confirming is a deletion waiting for a yes, and holds the row it
	// would delete so that moving the cursor can't redirect it.
	confirming *deckEntry
}

// newDeckList reads what's on disk and what's bookmarked.
func newDeckList() *deckList {
	l := &deckList{order: byModified}
	l.reload()
	return l
}

// reload rebuilds from disk. Every change goes through it, so the list can
// never disagree with the files behind it.
func (l *deckList) reload() {
	var all []deckEntry

	summaries, _ := deck.Summaries()
	for _, s := range summaries {
		all = append(all, deckEntry{
			kind: entryLocal, name: s.Name, slug: s.Slug, format: s.Format,
			count: s.Total, modified: s.Modified, broken: s.Broken,
		})
	}

	b := deck.LoadBookmarks()
	for _, r := range b.Remotes {
		all = append(all, deckEntry{kind: entryRemote, name: r.Name, id: r.ID})
	}
	for _, u := range b.Users {
		all = append(all, deckEntry{kind: entryUser, name: u, user: u})
	}

	l.all = all
	l.refresh()
}

func (l *deckList) refresh() {
	rows := l.all
	if terms := filterTerms(l.filter); len(terms) > 0 {
		kept := rows[:0:0]
		for _, e := range rows {
			if matchesEntry(e, terms) {
				kept = append(kept, e)
			}
		}
		rows = kept
	}

	sorted := append([]deckEntry(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return lessEntry(sorted[i], sorted[j], l.order) })
	l.rows = sorted
	l.cursor.clamp(len(l.rows))
}

func matchesEntry(e deckEntry, terms []string) bool {
	hay := strings.ToLower(e.name + " " + e.slug + " " + e.format + " " + e.kind.letter())
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func lessEntry(a, b deckEntry, s deckListSort) bool {
	switch s {
	case byName:
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	case bySize:
		return a.count > b.count
	case byKind:
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	}
	// Last touched, newest first. Things with no date — remotes and users —
	// sort to the bottom rather than claiming 1970.
	if a.modified.IsZero() != b.modified.IsZero() {
		return !a.modified.IsZero()
	}
	if !a.modified.Equal(b.modified) {
		return a.modified.After(b.modified)
	}
	return strings.ToLower(a.name) < strings.ToLower(b.name)
}

func (l *deckList) current() (deckEntry, bool) {
	if l.cursor.at < 0 || l.cursor.at >= len(l.rows) {
		return deckEntry{}, false
	}
	return l.rows[l.cursor.at], true
}

func (l *deckList) setFilter(s string) {
	l.filter = s
	l.refresh()
}

func (l *deckList) cycleSort(delta int) {
	const n = 4
	l.order = deckListSort(((int(l.order)+delta)%n + n) % n)
	l.refresh()
}

// ── As a view ───────────────────────────────────────────────────

func (l *deckList) title() string { return "decks" }

func (l *deckList) subtitle() string {
	out := itoa(len(l.rows))
	if len(l.rows) != len(l.all) {
		out += "/" + itoa(len(l.all))
	}
	return out + " · " + l.order.String()
}

func (l *deckList) lines(width, height int, focused bool, m *Model) []string {
	if l.confirming != nil {
		return fillTo([]string{
			lipgloss.NewStyle().Foreground(theme.Error).Bold(true).
				Render(fit("delete "+l.confirming.name+"?", width)),
			mutedLine("y to confirm · any other key cancels", width),
		}, width, height)
	}
	if len(l.rows) == 0 {
		what := "no decks yet — n to make one"
		if l.filter != "" {
			what = "nothing matches"
		}
		return fillTo([]string{mutedLine(what, width)}, width, height)
	}

	l.cursor.scrollInto(height, len(l.rows))
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.rows) && len(lines) < height; i++ {
		lines = append(lines, renderEntry(l.rows[i], width, focused && i == l.cursor.at))
	}
	return fillTo(lines, width, height)
}

func (l *deckList) clear() bool {
	if l.filter != "" {
		l.setFilter("")
		return true
	}
	return false
}

// ── Drawing a row ───────────────────────────────────────────────

// renderEntry draws one row: the name, then a tail of kind, legality, size
// and age. The tail is given up from the right as the panel narrows.
func renderEntry(e deckEntry, width int, under bool) string {
	if width < 1 {
		return ""
	}

	// The kind and its legality mark travel together: they are two
	// characters saying what this is and whether it's playable.
	kind := e.kind.letter()
	legal := " "
	if e.legal != nil && *e.legal {
		legal = "*"
	}

	tail := []string{kind + legal}
	if e.count > 0 {
		tail = append(tail, itoa(e.count))
	}
	if age := shortAge(e.modified); age != "" {
		tail = append(tail, age)
	}

	// Drop from the right until the name has somewhere worth living. A
	// name with no age beside it is still a name; an age with no name is
	// nothing, so the tail yields first and keeps yielding.
	name := e.name
	if e.broken {
		name += " (unreadable)"
	}
	const minName = 12
	for len(tail) > 1 && textWidth(strings.Join(tail, " "))+minName+1 > width {
		tail = tail[:len(tail)-1]
	}

	right := strings.Join(tail, " ")
	space := width - textWidth(right)
	left := fit(name, maxInt(space-1, 0))

	nameStyle := lipgloss.NewStyle().Foreground(theme.Text)
	switch {
	case e.broken:
		nameStyle = nameStyle.Foreground(theme.Error)
	case e.kind == entryUser:
		nameStyle = nameStyle.Foreground(theme.Info)
	case e.kind == entryRemote:
		nameStyle = nameStyle.Foreground(theme.TextDim)
	}
	if under {
		nameStyle = nameStyle.Foreground(theme.SelectionFg).Bold(true)
	}

	line := nameStyle.Render(left) + " " +
		lipgloss.NewStyle().Foreground(theme.TextDim).Render(right)

	if under {
		return lipgloss.NewStyle().Background(theme.SelectionBg).Width(width).Render(line)
	}
	return line
}

// shortAge is how long ago, in one or two characters plus a unit. Empty for
// things that have no date, rather than a dash that reads as a value.
func shortAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return itoa(maxInt(int(d.Minutes()), 1)) + "m"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h"
	case d < 365*24*time.Hour:
		return itoa(int(d.Hours()/24)) + "d"
	}
	return itoa(int(d.Hours()/24/365)) + "y"
}

// key is the decks panel's own keymap. The mutations arrive in the next
// commit; this is navigation, ordering and opening.
func (l *deckList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	// A pending deletion swallows the next key, whatever it is: a
	// confirmation you can answer by accident isn't one.
	if l.confirming != nil {
		target := *l.confirming
		l.confirming = nil
		if k == "y" {
			return true, m.deleteEntry(l, target)
		}
		return true, nil
	}

	if l.cursor.navKey(k, len(l.rows), m.pageStep()) {
		return true, nil
	}

	switch k {
	case "o":
		l.cycleSort(1)
	case "O":
		l.cycleSort(-1)
	case "/":
		p.openFilter(l.filter)
	case "enter":
		return true, m.openEntry(l, p, false)
	case "L":
		return true, m.openEntry(l, p, true)

	case "n":
		p.ask(askNewDeck, "name", "")

	case "r":
		if e, ok := l.current(); ok && e.kind != entryUser {
			p.ask(askRename, "rename", e.name)
		}

	case "c":
		if e, ok := l.current(); ok {
			return true, copyEntry(e)
		}

	case "x":
		e, ok := l.current()
		if !ok {
			return true, nil
		}
		if e.kind == entryLocal {
			// A deck file is the only one of the three that loses work.
			// Following and unfollowing are free, so they just happen.
			l.confirming = &e
			return true, nil
		}
		return true, m.deleteEntry(l, e)

	default:
		return false, nil
	}
	return true, nil
}
