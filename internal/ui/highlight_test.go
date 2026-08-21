package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestHighlightLineCoversTheWholeRow guards the fix for a selection that only
// reached the first column: a row is built from styled spans, each ending in a
// reset that clears the background too, so setting the background once left the
// fill stopping at the first reset. highlightLine re-sets it after every reset.
//
// The check needs colour actually on, which the test terminal doesn't have, so
// it forces a profile for the duration and puts it back.
func TestHighlightLineCoversTheWholeRow(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)

	bg := lipgloss.Color("4")
	line := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("MK") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("Name") + "  tail"

	out := highlightLine(line, 30, bg)
	seq := bgStart(bg)
	if seq == "" {
		t.Fatal("no background sequence under a colour profile")
	}

	// The background must be set again after every reset, so no reset is left
	// standing without the background following it — which is what let the fill
	// lapse mid-row.
	for _, part := range strings.SplitAfter(out, ansiReset) {
		part = strings.TrimSuffix(part, ansiReset)
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, seq) {
			t.Errorf("a span after a reset lost the background: %q in %q", part, out)
		}
	}

	// And the whole row is filled to the width, not just the text.
	if got := textWidth(stripANSI(out)); got != 30 {
		t.Errorf("highlighted row is %d wide, want 30: %q", got, stripANSI(out))
	}
}
