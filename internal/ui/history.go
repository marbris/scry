package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/mtg"
	"scry/internal/prints"
	"scry/internal/theme"
)

// A card's printed text, through the years.
//
// gv on a card, the same key that shows a deck's versions — same question,
// two kinds of thing. Scryfall only serves a card's *current* wording, so
// this comes from MTGJSON, which publishes whole sets: a heavily reprinted
// card means one file per set it appeared in.
//
// That is the whole difficulty. Forty printings is forty downloads of a
// multi-megabyte file each, which is minutes. So what has already been
// cached is shown immediately, and the rest is offered rather than fetched:
// you get an answer at once, and a way to get a better one.

type histState int

const (
	histPrintings histState = iota // finding out where it was printed
	histWaiting                    // some sets aren't cached; asking first
	histFetching
	histReady
	histFailed
)

// cardHistory is the printed-text history of one card, and how far along it
// is.
type cardHistory struct {
	state     histState
	card      mtg.Card
	printings []prints.Printing
	// missing are the sets whose text hasn't been downloaded yet.
	missing   []string
	originals map[string]map[string]string
	revisions []prints.TextRevision
	err       error
}

type printingsMsg struct {
	card      string
	printings []prints.Printing
	err       error
}

type setTextMsg struct {
	card  string
	set   string
	cards map[string]string
	err   error
}

// openHistory is gv on a card.
func (m *Model) openHistory(c mtg.Card) tea.Cmd {
	if c.OracleID == "" || c.PrintsSearchURI == "" {
		m.notice = "no printing history for this card"
		return nil
	}

	m.info.mode = infoVersions
	m.info.offset = 0
	if h, ok := m.histories[c.OracleID]; ok && h.state != histFailed {
		return nil // already have it, or already asking
	}

	m.histories[c.OracleID] = &cardHistory{
		state: histPrintings, card: c,
		originals: map[string]map[string]string{},
	}
	oracle, uri := c.OracleID, c.PrintsSearchURI
	return func() tea.Msg {
		list, err := prints.Printings(uri)
		return printingsMsg{card: oracle, printings: list, err: err}
	}
}

func (m Model) handlePrintings(msg printingsMsg) (tea.Model, tea.Cmd) {
	h, ok := m.histories[msg.card]
	if !ok {
		return m, nil
	}
	if msg.err != nil {
		h.state, h.err = histFailed, msg.err
		return m, nil
	}

	h.printings = msg.printings
	// Everything already on disk costs nothing, so take it now and see what
	// is left to ask about.
	var missing []string
	var cmds []tea.Cmd
	for _, p := range msg.printings {
		if prints.IsCached(p.Set) {
			cmds = append(cmds, fetchSetText(msg.card, p.Set))
			continue
		}
		missing = append(missing, p.Set)
	}
	h.missing = missing

	switch {
	case len(missing) == 0:
		h.state = histFetching
	default:
		h.state = histWaiting
	}
	if len(cmds) == 0 {
		m.finishHistory(h)
	}
	return m, tea.Batch(cmds...)
}

// fetchAllSets is what y answers: go and get the rest.
func (m *Model) fetchAllSets(h *cardHistory) tea.Cmd {
	h.state = histFetching
	var cmds []tea.Cmd
	for _, set := range h.missing {
		cmds = append(cmds, fetchSetText(h.card.OracleID, set))
	}
	h.missing = nil
	return tea.Batch(cmds...)
}

func fetchSetText(oracle, set string) tea.Cmd {
	return func() tea.Msg {
		cards, err := prints.SetOriginals(set)
		return setTextMsg{card: oracle, set: set, cards: cards, err: err}
	}
}

func (m Model) handleSetText(msg setTextMsg) (tea.Model, tea.Cmd) {
	h, ok := m.histories[msg.card]
	if !ok {
		return m, nil
	}
	if msg.err == nil {
		h.originals[msg.set] = msg.cards
	}

	// Done when every set that is going to arrive has.
	want := 0
	for _, p := range h.printings {
		if _, have := h.originals[p.Set]; have {
			continue
		}
		if contains(h.missing, p.Set) {
			continue
		}
		want++
	}
	if want == 0 {
		m.finishHistory(h)
	}
	return m, nil
}

func (m *Model) finishHistory(h *cardHistory) {
	h.revisions = prints.BuildRevisions(h.card, h.printings, h.originals)
	if len(h.missing) > 0 {
		h.state = histWaiting
	} else {
		h.state = histReady
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// renderHistory draws what is known so far, and says what isn't.
func (m Model) renderHistory(c mtg.Card, width int) []string {
	h, ok := m.histories[c.OracleID]
	if !ok {
		return []string{mutedLine("gv for how its text has changed", width)}
	}

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	muted := lipgloss.NewStyle().Foreground(theme.TextMuted)

	switch h.state {
	case histPrintings:
		return []string{muted.Render(fit("finding the printings…", width))}
	case histFailed:
		return wrapStyled(errorText(h.err), width, lipgloss.NewStyle().Foreground(theme.Error))
	}

	var out []string
	for i, rev := range h.revisions {
		if i > 0 {
			out = append(out, "")
		}
		label := rev.SetName
		if label == "" {
			label = "current oracle text"
		}
		if year := year(rev.Released); year != "" {
			label += " · " + year
		}
		if rev.Printings > 1 {
			label += " · " + itoa(rev.Printings) + " printings"
		}
		if rev.Current {
			label += " · current"
		}
		out = append(out, head.Render(fit(label, width)))
		out = append(out, highlightOracle(rev.Text, c, width, m.rules)...)
	}

	if len(h.revisions) == 0 && h.state != histWaiting {
		out = append(out, muted.Render(fit("no printed text on record", width)))
	}

	switch h.state {
	case histFetching:
		out = append(out, "", muted.Render(fit("fetching…", width)))
	case histWaiting:
		out = append(out, "")
		out = append(out, dim.Render(fit(itoa(len(h.missing))+" more "+
			plural("set", len(h.missing))+" not downloaded", width)))
		out = append(out, muted.Render(fit("y to fetch them", width)))
	}
	return out
}

func year(released string) string {
	if i := strings.Index(released, "-"); i > 0 {
		return released[:i]
	}
	return released
}
