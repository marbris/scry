package rules

import (
	"strings"
	"testing"

	"scry/internal/mtg"
)

// loadTestRules skips rather than fails when the rules haven't been
// downloaded: these assert the parser against the real rulebook, and an
// absent download is a missing fixture, not a broken parser.
func loadTestRules(t *testing.T) Data {
	t.Helper()
	data, err := Load()
	if err != nil {
		t.Skipf("comprehensive rules unavailable: %v", err)
	}
	return data
}

// ── Rules index ─────────────────────────────────────────────────

func TestRulesIndex(t *testing.T) {
	data := loadTestRules(t)

	if len(data.Rules) < 3000 || len(data.Glossary) < 500 {
		t.Errorf("parsed %d rules and %d glossary entries, expected far more",
			len(data.Rules), len(data.Glossary))
	}

	// Keyword abilities, keyword actions and ability words all come out of
	// the rules text rather than a hardcoded list.
	want := map[string]string{
		"flying":        "702.9",
		"double strike": "702.4",
		"scry":          "701.22",
		"landfall":      "207.2c",
	}
	for name, rule := range want {
		kw, ok := data.keywords[name]
		if !ok {
			t.Errorf("keyword %q missing from the index", name)
			continue
		}
		if kw.Rule != rule {
			t.Errorf("keyword %q -> rule %s, want %s", name, kw.Rule, rule)
		}
	}

	// Every card type must resolve to a category rule, including the
	// irregular plurals ("Sorceries", "Phenomena").
	for cardType := range cardTypeRules {
		if _, ok := data.typeRules[cardType]; !ok {
			t.Errorf("card type %q did not resolve to a rule", cardType)
		}
	}
}

func TestMatchCard(t *testing.T) {
	data := loadTestRules(t)
	card := mtg.Card{
		Name:       "Test Dragon",
		TypeLine:   "Legendary Creature — Dragon",
		OracleText: "Flying, hexproof\nWhenever Test Dragon attacks, scry 1.",
	}

	var terms []string
	for _, match := range data.MatchCard(card) {
		terms = append(terms, match.Term)
	}
	joined := strings.Join(terms, ",")

	for _, want := range []string{"Flying", "Hexproof", "Creature"} {
		if !strings.Contains(joined, want) {
			t.Errorf("MatchCard did not find %q; got %s", want, joined)
		}
	}
}
