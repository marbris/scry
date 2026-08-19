package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// splitLines and visibleWidth measure a rendered frame the way a terminal
// would: by what it shows, not by the escape sequences that colour it.

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func visibleWidth(s string) int {
	return textWidth(stripANSI(s))
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// msgOf runs a command and hands back what it produced, for tests that want
// the answer without a running program.
func msgOf(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func sizeOf(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

// focusOn moves focus to a panel by index, without going through the keys.
func focusOn(m Model, at int) Model {
	m.ws.focus(at)
	return m
}

func keyMsg(k string) tea.KeyMsg {
	if k == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func mkReader(s string) *strings.Reader { return strings.NewReader(s) }
