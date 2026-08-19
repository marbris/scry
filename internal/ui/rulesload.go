package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/mtg"
	"scry/internal/rules"
)

// Getting hold of the rulebook.
//
// It is a megabyte of text and parsing it takes long enough to notice, so it
// happens once, off the main thread, and only when something asks. Everything
// that wants it goes through here and waits if it has to — a panel opened
// before the parse finishes is filled when it lands, rather than refusing.

type rulesLoadedMsg struct {
	data rules.Data
	err  error
}

func loadRules() tea.Msg {
	data, err := rules.Load()
	return rulesLoadedMsg{data: data, err: err}
}

// wantRules is what a rules panel is waiting for: a query to run, or a card
// to explain, once the rulebook has arrived.
type wantRules struct {
	panel int
	query string
	card  *mtg.Card
}

// openRules opens a rules panel — over the card under the cursor if there is
// one, and empty with the bar focused if there isn't. That is the whole
// difference between looking a rule up and asking why a card doesn't work.
func (m *Model) openRules() tea.Cmd {
	card := m.focusedCard()

	p := m.ws.open(KindRules)
	if card == nil {
		return m.ensureRules(nil)
	}

	p.searchOpen = false
	p.search.Blur()
	p.title = card.Name
	m.pending = append(m.pending, wantRules{panel: p.id, card: card})
	return m.ensureRules(p)
}

// focusedCard is the card under the cursor in the panel you were on, which
// is what a rules panel opens about.
func (m Model) focusedCard() *mtg.Card {
	p := m.ws.current()
	if p == nil {
		return nil
	}
	l := p.cardsView()
	if l == nil {
		return nil
	}
	c, ok := l.current()
	if !ok {
		return nil
	}
	card := c.Card
	return &card
}

// focusedCardValue is focusedCard without the pointer receiver, for the
// drawing side, which only ever reads.
func (m Model) focusedCardValue() *mtg.Card { return m.focusedCard() }

// ensureRules starts the parse if it hasn't happened, and marks the panel as
// waiting for it.
func (m *Model) ensureRules(p *panel) tea.Cmd {
	if m.rules.Loaded() {
		m.fillPending()
		return nil
	}
	if p != nil {
		p.loading = true
	}
	if m.rulesLoading {
		return nil
	}
	m.rulesLoading = true
	return loadRules
}

func (m Model) handleRulesLoaded(msg rulesLoadedMsg) (tea.Model, tea.Cmd) {
	m.rulesLoading = false
	if msg.err != nil {
		m.rulesErr = msg.err
		for _, w := range m.pending {
			if p := m.ws.byID(w.panel); p != nil {
				p.loading = false
				p.err = msg.err
			}
		}
		m.pending = nil
		return m, nil
	}

	m.rules = msg.data
	m.fillPending()
	return m, nil
}

// fillPending gives the rulebook to every panel that was waiting for it.
func (m *Model) fillPending() {
	for _, w := range m.pending {
		p := m.ws.byID(w.panel)
		if p == nil {
			continue
		}
		p.loading = false
		switch {
		case w.card != nil:
			p.show(newCardRules(m.rules, *w.card))
		case w.query != "":
			p.show(newRuleSearch(m.rules, w.query))
		}
	}
	m.pending = nil
}

// searchRules is what enter in a rules panel's bar does.
func (m *Model) searchRules(p *panel, query string) tea.Cmd {
	p.searchOpen = false
	p.search.Blur()
	p.title = "rules: " + query

	if m.rules.Loaded() {
		p.show(newRuleSearch(m.rules, query))
		return nil
	}
	m.pending = append(m.pending, wantRules{panel: p.id, query: query})
	return m.ensureRules(p)
}
