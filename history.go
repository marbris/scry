package main

// Printed-text history.
//
// Scryfall only ever serves a card's *current* oracle text — its
// printed_text field is populated for non-English cards only, and there is
// no oracle revision history in the API at all. MTGJSON does carry the
// wording as printed, per printing, in its originalText field, so that's
// where this comes from.
//
// MTGJSON only publishes whole sets, so a heavily reprinted card means one
// file per set it appeared in. Each set is distilled down to a name -> text
// map and cached on disk, and nothing is fetched until the user asks for it.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/prints"
)

// fetchPrintings and fetchSetOriginals run the prints package off the main
// thread; the work itself lives there.

func fetchPrintings(oracleID, uri string) tea.Cmd {
	return func() tea.Msg {
		list, err := prints.Printings(uri)
		return printingsMsg{oracleID: oracleID, printings: list, err: err}
	}
}

func fetchSetOriginals(set string) tea.Cmd {
	return func() tea.Msg {
		cards, err := prints.SetOriginals(set)
		return setOriginalsMsg{set: set, cards: cards, err: err}
	}
}

type histState int

const (
	histPrintings histState = iota // looking up where the card was printed
	histNeedSets                   // waiting for the go-ahead to download sets
	histFetching                   // downloading set data
	histReady
	histFailed
)

type cardHistory struct {
	state     histState
	printings []prints.Printing
	sets      []string // every set the card was printed in
	missing   []string // the ones not cached yet
	revisions []prints.TextRevision
	err       error
}

type printingsMsg struct {
	oracleID  string
	printings []prints.Printing
	err       error
}

type setOriginalsMsg struct {
	set   string
	cards map[string]string
	err   error
}

// ── Model wiring ────────────────────────────────────────────────

// toggleHistory opens the printed-text panel. Because a heavily reprinted
// card can mean a lot of set downloads, this only ever fetches when the
// user presses the key — never as a side effect of moving the cursor.
func (m model) toggleHistory() (tea.Model, tea.Cmd) {
	card, ok := m.active().selected()
	if !ok {
		return m, nil
	}
	h := m.histories[card.Card.OracleID]

	// Already showing a finished history: the key toggles back to the card.
	if m.panel == panelHistory && h != nil && (h.state == histReady || h.state == histFailed) {
		m.panel = panelCard
		return m, nil
	}

	m.panel = panelHistory
	m.historyScroll = 0

	switch {
	case card.Card.OracleID == "":
		return m, nil

	case h == nil:
		// First look at this card: find out where it was printed.
		m.histories[card.Card.OracleID] = &cardHistory{state: histPrintings}
		return m, fetchPrintings(card.Card.OracleID, card.Card.PrintsSearchURI)

	case h.state == histNeedSets:
		// The user has seen the download count and pressed the key again.
		// Every set still needs asking for, not just the uncached ones —
		// fetchSetOriginals serves the cached ones straight off disk.
		h.state = histFetching
		cmds := make([]tea.Cmd, 0, len(h.sets))
		for _, set := range h.sets {
			if _, inMemory := m.originals[set]; !inMemory {
				cmds = append(cmds, fetchSetOriginals(set))
			}
		}
		if len(cmds) == 0 {
			m.finishHistory(card.Card.OracleID)
			return m, nil
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m model) handlePrintings(msg printingsMsg) (tea.Model, tea.Cmd) {
	h := m.histories[msg.oracleID]
	if h == nil {
		return m, nil
	}

	if msg.err != nil {
		h.state, h.err = histFailed, msg.err
		return m, nil
	}

	h.printings = msg.printings
	h.sets = h.sets[:0]
	h.missing = h.missing[:0]
	for _, p := range msg.printings {
		h.sets = append(h.sets, p.Set)
		if _, inMemory := m.originals[p.Set]; !inMemory && !prints.IsCached(p.Set) {
			h.missing = append(h.missing, p.Set)
		}
	}

	// Everything is on disk already, so there's nothing to ask about.
	if len(h.missing) == 0 {
		cmds := make([]tea.Cmd, 0, len(h.sets))
		for _, set := range h.sets {
			if _, inMemory := m.originals[set]; !inMemory {
				cmds = append(cmds, fetchSetOriginals(set))
			}
		}
		if len(cmds) == 0 {
			m.finishHistory(msg.oracleID)
			return m, nil
		}
		h.state = histFetching
		return m, tea.Batch(cmds...)
	}

	h.state = histNeedSets
	return m, nil
}

func (m model) handleSetOriginals(msg setOriginalsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Remember the failure as an empty set so one bad set doesn't
		// wedge the whole history.
		m.originals[msg.set] = map[string]string{}
	} else {
		m.originals[msg.set] = msg.cards
	}

	// Any card waiting on this set may now be complete.
	for oracleID, h := range m.histories {
		if h.state != histFetching {
			continue
		}
		ready := true
		for _, set := range h.sets {
			if _, ok := m.originals[set]; !ok {
				ready = false
				break
			}
		}
		if ready {
			m.finishHistory(oracleID)
		}
	}
	return m, nil
}

// finishHistory assembles the revisions once every set is in.
func (m model) finishHistory(oracleID string) {
	h := m.histories[oracleID]
	if h == nil {
		return
	}
	card := m.cardByOracleID(oracleID)
	h.revisions = prints.BuildRevisions(card, h.printings, m.originals)
	h.state = histReady
}

func (m model) cardByOracleID(oracleID string) ScryfallCard {
	for _, c := range m.cards {
		if c.OracleID == oracleID {
			return c
		}
	}
	return ScryfallCard{}
}

// pendingSets counts the sets a history is still waiting on.
func (m model) pendingSets(h *cardHistory) int {
	n := 0
	for _, set := range h.sets {
		if _, ok := m.originals[set]; !ok {
			n++
		}
	}
	return n
}

// ── Panel ───────────────────────────────────────────────────────

func (m model) renderTextHistory(c ScryfallCard, width int) string {
	if width < 20 {
		width = 20
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvOrange)
	setStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)
	warnStyle := lipgloss.NewStyle().Foreground(gruvYellow)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Text History") + " " +
		dimStyle.Render("· "+truncate(c.Name, width-16)) + "\n\n")

	if c.Name == "" {
		return b.String() + dimStyle.Render("No card selected")
	}
	if c.OracleID == "" {
		return b.String() + dimStyle.Render("This card has no oracle id to trace.")
	}

	h := m.histories[c.OracleID]
	if h == nil {
		return b.String() + dimStyle.Render("t: load printed text history")
	}

	switch h.state {
	case histPrintings:
		return b.String() + dimStyle.Render("Looking up printings…")

	case histNeedSets:
		b.WriteString(dimStyle.Render(fmt.Sprintf(
			"%d printings across %d sets.", len(h.printings), len(h.sets))) + "\n\n")
		b.WriteString(warnStyle.Width(width).Render(fmt.Sprintf(
			"%d sets aren't cached yet. MTGJSON only publishes whole sets, so reading this card's old wording means downloading them.",
			len(h.missing))) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(gruvOrange).Render("t: download and show"))
		return b.String()

	case histFetching:
		done := len(h.sets) - m.pendingSets(h)
		return b.String() + dimStyle.Render(fmt.Sprintf("Fetching set data… %d/%d", done, len(h.sets)))

	case histFailed:
		return b.String() + lipgloss.NewStyle().Foreground(gruvRed).
			Render(fmt.Sprintf("unavailable: %v", h.err))
	}

	if len(h.revisions) == 0 {
		return b.String() + dimStyle.Render("No printed text on record for this card.")
	}

	wordings := "1 wording"
	if len(h.revisions) > 1 {
		wordings = fmt.Sprintf("%d wordings", len(h.revisions))
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("%s · %d printings",
		wordings, len(h.printings))) + "\n\n")

	for i, rev := range h.revisions {
		if i > 0 {
			b.WriteString("\n")
		}

		switch {
		case rev.SetName == "":
			b.WriteString(setStyle.Render("Current oracle text") + "\n")
		default:
			head := setStyle.Render(truncate(rev.SetName, width-14))
			if rev.Released != "" {
				head += dimStyle.Render(" · " + year(rev.Released))
			}
			if rev.Current {
				head += lipgloss.NewStyle().Foreground(gruvGreen).Render(" → current")
			}
			b.WriteString(head + "\n")
		}

		b.WriteString(highlightRuleText(rev.Text, width, m.rules) + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func year(released string) string {
	if len(released) >= 4 {
		return released[:4]
	}
	return released
}
