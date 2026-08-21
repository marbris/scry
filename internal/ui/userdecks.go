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
	out += " " + plural("deck", len(l.decks))
	if l.filter != "" {
		out += " · /" + l.filter
	}
	return out
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
	// The same columns the decks list uses, sized once for the whole list so
	// they line up: legality, colours, size and age.
	cols := measureUserCols(l.decks, width)
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.decks) && len(lines) < height; i++ {
		lines = append(lines, renderUserDeck(l.decks[i], cols, width, focused && i == l.cursor.at))
	}
	return fillTo(lines, width, height)
}

// clear has nothing transient to drop for esc; the filter is cleared with b.
func (l *userDeckList) clear() bool { return false }

// userCols mirrors deckCols for someone else's decks: the widths of the
// colours, size and age columns, held fixed across the list so they align. The
// legality mark is a fixed single character, so it needs no measuring.
type userCols struct {
	pips  int
	count int
	age   int
}

func measureUserCols(decks []moxfield.UserDeck, width int) userCols {
	var c userCols
	for _, d := range decks {
		c.pips = maxInt(c.pips, textWidth(manaPips(d.Colors)))
		c.count = maxInt(c.count, textWidth(itoa(d.Cards)))
		c.age = maxInt(c.age, textWidth(shortAge(d.UpdatedAt())))
	}

	const minName = 12
	for userTailWidth(c)+minName+1 > width {
		switch {
		case c.age > 0:
			c.age = 0
		case c.count > 0:
			c.count = 0
		case c.pips > 0:
			c.pips = 0
		default:
			return c
		}
	}
	return c
}

func userTailWidth(c userCols) int {
	w := 1 // the legality mark
	for _, col := range []int{c.pips, c.count, c.age} {
		if col > 0 {
			w += 1 + col
		}
	}
	return w
}

// renderUserDeck draws one of someone's decks in the same shape as the decks
// list: the name, then legality — by Moxfield's own reckoning — colours, size
// and an abbreviated age.
func renderUserDeck(d moxfield.UserDeck, cols userCols, width int, under bool) string {
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	slots := []string{userLegalMark(d.Legal)}
	if cols.pips > 0 {
		slots = append(slots, paintMana(padLeft(manaPips(d.Colors), cols.pips)))
	}
	if cols.count > 0 {
		slots = append(slots, dim.Render(padLeft(itoa(d.Cards), cols.count)))
	}
	if cols.age > 0 {
		slots = append(slots, dim.Render(padLeft(shortAge(d.UpdatedAt()), cols.age)))
	}
	right := strings.Join(slots, " ")
	tailWidth := textWidth(stripStyles(right))

	name := fit(d.Name, maxInt(width-tailWidth-1, 0))
	style := lipgloss.NewStyle().Foreground(theme.Text)
	if under {
		style = style.Foreground(theme.SelectionFg).Bold(true)
	}

	line := style.Render(name) + " " + right
	if under {
		return highlightLine(line, width, theme.SelectionBg)
	}
	return line
}

// userLegalMark is Moxfield's own legality verdict, in the one character the
// row has room for: the same * / ! the decks list uses. Moxfield always tells
// us, so unlike a local deck there is no unknown state.
func userLegalMark(legal bool) string {
	if legal {
		return lipgloss.NewStyle().Foreground(theme.Success).Render("*")
	}
	return lipgloss.NewStyle().Foreground(theme.Error).Render("!")
}

func (l *userDeckList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if l.cursor.navKey(k, len(l.decks)) {
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

	case "C":
		// Take the main deck and the author's Considering list, as two decks.
		if l.cursor.at < len(l.decks) {
			d := l.decks[l.cursor.at]
			return true, copyEntryBoth(deckEntry{kind: entryRemote, name: d.Name, id: d.PublicID})
		}

	case "r":
		// Follow it, so it lands in your decks list without a copy.
		if l.cursor.at < len(l.decks) {
			d := l.decks[l.cursor.at]
			return true, followRemote(d.Name, d.PublicID, d.PublicURL)
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
	out = append(out, "",
		mutedLine("enter to open · r to follow", width),
		mutedLine("c to copy · C for the considering list too", width))
	return out
}

func (l *userDeckList) keys() []hintGroup {
	return []hintGroup{
		{"navigation", [][2]string{
			{"j k", "up/down"},
			{"/", "filter"},
		}},
		{"decks", [][2]string{
			{"enter", "open"},
			{"L", "beside"},
			{"r", "follow"},
			{"c", "copy"},
			{"C", "copy + considering"},
		}},
	}
}
