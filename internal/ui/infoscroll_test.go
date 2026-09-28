package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/mtg"
)

// longInfo is a search whose highlighted card has far more to read than the
// information panel has room for: a dozen rulings, each its own paragraph.
func longInfo(t *testing.T) Model {
	t.Helper()
	m := withCards(sized(120, 20), "f", sample(), sortArrival)
	l := m.ws.current().cardsView()
	c, ok := l.current()
	if !ok {
		t.Fatal("no card under the cursor")
	}
	if l.rulings == nil {
		l.rulings = map[string][]mtg.Ruling{}
	}
	var rs []mtg.Ruling
	for i := 0; i < 12; i++ {
		rs = append(rs, mtg.Ruling{Source: "wotc", Comment: "Ruling number " + itoa(i) +
			" says something long enough to wrap onto a second line of the panel."})
	}
	l.rulings[c.Card.ID] = rs
	if _, _, most, _ := m.infoSpan(); most == 0 {
		t.Fatal("the fixture fits the panel; nothing to scroll")
	}
	return m
}

func ctrl(m Model, down bool) Model {
	msg := tea.KeyMsg{Type: tea.KeyCtrlK}
	if down {
		msg = tea.KeyMsg{Type: tea.KeyCtrlJ}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestShiftJScrollsHalfTheInfoPanel(t *testing.T) {
	m := longInfo(t)
	_, room, most, _ := m.infoSpan()

	m = drive(m, "J")
	if got, want := m.info.offset, minInt(room/2, most); got != want {
		t.Errorf("J scrolled to %d, want half of %d rows = %d", got, room, want)
	}
	m = drive(m, "K")
	if m.info.offset != 0 {
		t.Errorf("K came back to %d, want 0", m.info.offset)
	}
}

func TestShiftJStopsAtTheBottom(t *testing.T) {
	m := longInfo(t)
	_, _, most, _ := m.infoSpan()
	for i := 0; i < 40; i++ {
		m = drive(m, "J")
	}
	if m.info.offset != most {
		t.Fatalf("offset %d, want the bottom %d", m.info.offset, most)
	}
	// One K is one move up — nothing to unwind first.
	m = drive(m, "K")
	if m.info.offset >= most {
		t.Errorf("K at the bottom didn't move: %d", m.info.offset)
	}
}

func TestCtrlJNeverScrollsPastTheBottom(t *testing.T) {
	// ctrl+j used to go on to the last paragraph's start however far down
	// that was, while the drawing stopped at the bottom — so ctrl+k then had
	// to unwind presses nobody could see before anything moved.
	m := longInfo(t)
	_, _, most, _ := m.infoSpan()
	for i := 0; i < 40; i++ {
		m = ctrl(m, true)
	}
	if m.info.offset != most {
		t.Fatalf("offset %d, want the bottom %d", m.info.offset, most)
	}
	before := stripANSI(m.View())
	m = ctrl(m, false)
	if m.info.offset >= most {
		t.Errorf("ctrl+k at the bottom didn't move: %d", m.info.offset)
	}
	if stripANSI(m.View()) == before {
		t.Error("ctrl+k at the bottom changed nothing on screen")
	}
}

func TestTheInfoPanelHintsHalfAPage(t *testing.T) {
	m := drive(longInfo(t), "?")
	if !strings.Contains(stripANSI(m.View()), "half page") {
		t.Error("K J aren't offered as half a page")
	}
}
