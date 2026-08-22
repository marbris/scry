package ui

import (
	"strings"
	"testing"

	"scry/internal/rules"
)

func changeSet() []rules.RuleChange {
	return []rules.RuleChange{
		{Number: "100.1", Kind: rules.Changed, Old: "old text", New: "new text"},
		{Number: "702.4", Kind: rules.Added, New: "a new keyword"},
		{Number: "800.1", Kind: rules.Removed, Old: "a departed rule"},
	}
}

func TestDiffOpensOnTheHighlightedRule(t *testing.T) {
	// gv from the rules panel opens on the rule you were reading, or the next
	// changed one after it.
	v := newRulesDiff(changeSet(), "800.1")
	if got := v.changes[v.cursor.at].Number; got != "800.1" {
		t.Errorf("opened on %q, want 800.1", got)
	}

	// A rule with no exact change lands on the next changed one.
	v = newRulesDiff(changeSet(), "500")
	if got := v.changes[v.cursor.at].Number; got != "702.4" {
		t.Errorf("opened on %q, want the next change 702.4", got)
	}

	// No focus opens at the top.
	v = newRulesDiff(changeSet(), "")
	if v.cursor.at != 0 {
		t.Errorf("opened at row %d, want the top", v.cursor.at)
	}
}

func TestDiffRowsStayWithinWidth(t *testing.T) {
	// Fitting an already-styled string measures the escape codes as width, which
	// truncates mid-escape and blows up the whole panel. Every rendered row must
	// have a visible width no greater than the panel's.
	changes := []rules.RuleChange{
		{Number: "702.4", Kind: rules.Changed, Old: strings.Repeat("old ", 40), New: strings.Repeat("new wording that is long ", 40)},
		{Number: "205.3j", Kind: rules.Added, New: strings.Repeat("a long added rule ", 40)},
	}
	const width = 40
	v := newRulesDiff(changes, "")
	rendered := v.lines(width, 10, true, nil)
	for _, line := range rendered {
		if got := textWidth(stripStyles(line)); got > width {
			t.Errorf("row visible width %d exceeds %d: %q", got, width, stripStyles(line))
		}
	}
	// The number must survive rendering — fitting the styled string used to
	// truncate inside the escape codes and cut the number out entirely.
	joined := stripStyles(strings.Join(rendered, "\n"))
	for _, want := range []string{"702.4", "205.3j"} {
		if !strings.Contains(joined, want) {
			t.Errorf("rendered rows dropped %q:\n%s", want, joined)
		}
	}
}

func TestDiffInfoShowsBeforeAndAfter(t *testing.T) {
	v := newRulesDiff(changeSet(), "100.1") // the Changed one
	info := stripANSI(strings.Join(v.info(60), "\n"))
	for _, want := range []string{"100.1", "changed", "before", "old text", "after", "new text"} {
		if !strings.Contains(info, want) {
			t.Errorf("info is missing %q:\n%s", want, info)
		}
	}
}
