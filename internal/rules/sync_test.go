package rules

import (
	"strings"
	"testing"
)

func TestVersionFromURL(t *testing.T) {
	got := versionFromURL("https://media.wizards.com/2026/downloads/MagicCompRules%2020260819.txt")
	if got != "20260819" {
		t.Errorf("got %q, want the date stamp", got)
	}
	if got := versionFromURL("no date here"); got != "" {
		t.Errorf("got %q, want empty for a URL with no date", got)
	}
}

func TestLinkReFindsTheDownload(t *testing.T) {
	// The href turns up both with the space escaped and, on some renders, left
	// literal — both have to match, and the version comes out either way.
	for _, page := range []string{
		`<a href="https://media.wizards.com/2026/downloads/MagicCompRules%2020260819.txt">Download</a>`,
		`<a href="https://media.wizards.com/2026/downloads/MagicCompRules 20260819.txt">Download</a>`,
	} {
		m := linkRe.FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("no match in %q", page)
		}
		if m[1] != "20260819" {
			t.Errorf("version %q, want 20260819", m[1])
		}
	}
}

// framed wraps rule lines in the minimal structure Parse recognises: the
// section header it scans for, then the rules.
func framed(rules ...string) string {
	return "1. Game Concepts\n\n" + strings.Join(rules, "\n") + "\n"
}

func TestDiffRulesReportsWhatChanged(t *testing.T) {
	old := framed(
		"702.3. Deathtouch is a keyword.",
		"702.4. Defender is a keyword.",
		"702.5. Gone in the new one.",
	)
	nw := framed(
		"702.3. Deathtouch is a keyword.",
		"702.4. Defender is a static ability.",
		"702.6. Brand new keyword.",
	)

	changes := DiffRules(old, nw)
	byNum := map[string]RuleChange{}
	for _, c := range changes {
		byNum[c.Number] = c
	}

	// Unchanged rules are left out entirely.
	if _, ok := byNum["702.3"]; ok {
		t.Errorf("an unchanged rule appeared in the diff: %+v", byNum["702.3"])
	}
	if c, ok := byNum["702.4"]; !ok || c.Kind != Changed {
		t.Errorf("702.4 should read as changed, got %+v", c)
	}
	if c, ok := byNum["702.5"]; !ok || c.Kind != Removed {
		t.Errorf("702.5 should read as removed, got %+v", c)
	}
	if c, ok := byNum["702.6"]; !ok || c.Kind != Added {
		t.Errorf("702.6 should read as added, got %+v", c)
	}
}

func TestDiffRulesIsOrderedByNumber(t *testing.T) {
	old := framed("100.1. A.", "100.2. B.", "100.10. C.")
	nw := framed("100.1. A changed.", "100.2. B changed.", "100.10. C changed.")

	changes := DiffRules(old, nw)
	want := []string{"100.1", "100.2", "100.10"}
	if len(changes) != len(want) {
		t.Fatalf("got %d changes, want %d: %+v", len(changes), len(want), changes)
	}
	for i, n := range want {
		if changes[i].Number != n {
			t.Errorf("change %d is %s, want %s (10 must not sort before 2)", i, changes[i].Number, n)
		}
	}
}
