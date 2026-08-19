package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/theme"
)

// A panel is one vertical strip of the workspace: a search bar over whatever
// that search turned up.
//
// The bar and the header are the same line. You type into it to fill the
// panel, and once something is in there the line stops being an input and
// starts describing what you're looking at — pressing i turns it back. A
// panel that permanently spent a row on an empty search box would be paying
// for it in the one direction there is never enough of.

// Kind is what a panel searches. It decides the panel's keymap and what its
// search bar means; the design doc's ctrl+f / ctrl+l / ctrl+r are the space
// prefix's f, d and r.
type Kind int

const (
	// KindNew is a panel that hasn't been told what it is yet. Tab in its
	// search bar cycles through the others.
	KindNew Kind = iota
	KindFind
	KindDecks
	KindRules
	KindCards
)

// kinds is the cycle tab walks, leaving out the two that aren't a search
// target: an untyped panel, and a card list, which is something a search
// produces rather than something you ask for.
var kinds = []Kind{KindFind, KindDecks, KindRules}

func (k Kind) String() string {
	switch k {
	case KindFind:
		return "find"
	case KindDecks:
		return "decks"
	case KindRules:
		return "rules"
	case KindCards:
		return "cards"
	}
	return "new"
}

// prompt is what the search bar offers to do, which is the only thing
// distinguishing an empty panel of one kind from another.
func (k Kind) prompt() string {
	switch k {
	case KindFind:
		return "scryfall"
	case KindDecks:
		return "moxfield user or deck url"
	case KindRules:
		return "rules"
	}
	return "tab to choose"
}

func (k Kind) placeholder() string {
	switch k {
	case KindFind:
		return "t:creature c:R cmc<=3 otag:removal"
	case KindDecks:
		return "a moxfield username, or a deck url"
	case KindRules:
		return "flying, 702.9, sacrifice"
	}
	return "tab: scryfall · decks · rules"
}

// next moves a panel to the following search target, wrapping. An untyped
// panel lands on the first rather than the second.
func (k Kind) next(delta int) Kind {
	at := 0
	for i, c := range kinds {
		if c == k {
			at = i + delta
			break
		}
	}
	n := len(kinds)
	return kinds[((at%n)+n)%n]
}

type panel struct {
	kind Kind

	// search is the bar, and searchOpen says whether it's showing. It starts
	// open on a new panel because an empty panel is a question.
	search     textinput.Model
	searchOpen bool

	// title is what the header says once the bar has closed.
	title string
}

func newPanel(kind Kind) *panel {
	in := textinput.New()
	in.Prompt = "⌕ "
	in.Placeholder = kind.placeholder()
	in.Focus()

	p := &panel{kind: kind, search: in, searchOpen: true}
	p.restyle()
	return p
}

// restyle repaints the search bar. Colours are read at render time rather
// than at construction, so switching theme doesn't need the panels rebuilt.
func (p *panel) restyle() {
	p.search.PromptStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	p.search.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	p.search.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.TextMuted)
}

// setKind retargets a panel, which only means anything while it's empty.
func (p *panel) setKind(k Kind) {
	p.kind = k
	p.search.Placeholder = k.placeholder()
}

// header is the line at the top of a panel: the search bar while it's open,
// otherwise a description of what's below it.
func (p *panel) header(width int) string {
	if p.searchOpen {
		p.search.Width = maxInt(width-4, 4)
		return p.search.View()
	}
	name := p.title
	if name == "" {
		name = p.kind.String()
	}
	return truncate(name, width)
}

// empty reports whether a panel has nothing in it yet, which is what makes
// esc close it rather than clear something.
func (p *panel) empty() bool { return p.title == "" }
