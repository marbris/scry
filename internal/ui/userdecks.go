package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"strings"

	"scry/internal/moxfield"
	"scry/internal/theme"
)

// Somebody's decks on Moxfield.
//
// This is a sub-view, reached by pressing enter on a person in the decks
// list, so esc goes back to that list rather than closing the panel. It's
// the one place in the program showing decks that aren't yours and aren't
// followed — a shelf you're browsing rather than a shelf you own.

type userDeckList struct {
	cursor
	user string
	all  []moxfield.UserDeck
	// decks is what's on screen: all, narrowed by the filter. Somebody with
	// two hundred decks is exactly who you need / for.
	decks  []moxfield.UserDeck
	filter string
}

func newUserDeckList(user string, decks []moxfield.UserDeck) *userDeckList {
	l := &userDeckList{user: user, all: decks}
	l.refresh()
	return l
}

func (l *userDeckList) refresh() {
	l.decks = l.all
	if terms := filterTerms(l.filter); len(terms) > 0 {
		kept := make([]moxfield.UserDeck, 0, len(l.all))
		for _, d := range l.all {
			hay := strings.ToLower(d.Name + " " + d.Format)
			keep := true
			for _, t := range terms {
				if !strings.Contains(hay, t) {
					keep = false
					break
				}
			}
			if keep {
				kept = append(kept, d)
			}
		}
		l.decks = kept
	}
	l.cursor.clamp(len(l.decks))
}

func (l *userDeckList) setFilter(s string) {
	l.filter = s
	l.refresh()
}

func (l *userDeckList) title() string { return l.user }

func (l *userDeckList) subtitle() string {
	out := itoa(len(l.decks))
	if len(l.decks) != len(l.all) {
		out += "/" + itoa(len(l.all))
	}
	return out + " " + plural("deck", len(l.decks))
}

func (l *userDeckList) lines(width, height int, focused bool, m *Model) []string {
	if len(l.decks) == 0 {
		what := "no public decks"
		if l.filter != "" {
			what = "nothing matches"
		}
		return fillTo([]string{mutedLine(what, width)}, width, height)
	}

	l.cursor.scrollInto(height, len(l.decks))
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.decks) && len(lines) < height; i++ {
		lines = append(lines, renderUserDeck(l.decks[i], width, focused && i == l.cursor.at))
	}
	return fillTo(lines, width, height)
}

func (l *userDeckList) clear() bool {
	if l.filter != "" {
		l.setFilter("")
		return true
	}
	return false
}

func renderUserDeck(d moxfield.UserDeck, width int, under bool) string {
	tail := []string{itoa(d.Cards)}
	if age := d.Age(); age != "" {
		tail = append(tail, age)
	}
	right := tail[0]
	if len(tail) > 1 && textWidth(d.Name)+textWidth(tail[0])+textWidth(tail[1])+3 <= width {
		right = tail[0] + " " + tail[1]
	}

	name := fit(d.Name, maxInt(width-textWidth(right)-1, 0))
	style := lipgloss.NewStyle().Foreground(theme.Text)
	if under {
		style = style.Foreground(theme.SelectionFg).Bold(true)
	}

	line := style.Render(name) + " " +
		lipgloss.NewStyle().Foreground(theme.TextDim).Render(right)
	if under {
		return lipgloss.NewStyle().Background(theme.SelectionBg).Width(width).Render(line)
	}
	return line
}

func (l *userDeckList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if l.cursor.navKey(k, len(l.decks), m.pageStep()) {
		return true, nil
	}

	switch k {
	case "/":
		p.openFilter(l.filter)
		return true, nil

	case "enter", "L":
		if l.cursor.at >= len(l.decks) {
			return true, nil
		}
		d := l.decks[l.cursor.at]

		target := p
		if k == "L" {
			target = m.ws.open(p.kind)
			m.ws.focus(m.ws.indexOf(p))
		}
		target.loading = true
		target.err = nil
		target.title = d.Name
		return true, openRemoteDeck(target.id, k == "L", d.PublicID)

	case "c":
		// Take a copy of somebody else's deck, which is the point of
		// looking through them.
		if l.cursor.at < len(l.decks) {
			d := l.decks[l.cursor.at]
			return true, copyEntry(deckEntry{kind: entryRemote, name: d.Name, id: d.PublicID})
		}
	}
	return false, nil
}

// plural is the same six lines as in deck and moxfield; see the note there.
func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func (l *userDeckList) info(width int) []string {
	if l.cursor.at >= len(l.decks) {
		return nil
	}
	d := l.decks[l.cursor.at]

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	out := []string{head.Render(fit(d.Name, width)), ""}
	if d.Format != "" {
		out = append(out, dim.Render(fit(d.Format, width)))
	}
	out = append(out, dim.Render(fit(itoa(d.Cards)+" cards", width)))
	if age := d.Age(); age != "" {
		out = append(out, dim.Render(fit("updated "+age, width)))
	}
	return append(out, "", mutedLine("enter to open · c to take a copy", width))
}
