package ui

import (
	"strings"
	"testing"

	"scry/internal/mtg"
	"scry/internal/rules"
)

// fakeRules is a rulebook in miniature — the real document's markers and a
// handful of its shapes — so these tests say what the view does rather than
// what the parser does, and don't need a megabyte downloaded first.
func fakeRules(t *testing.T) rules.Data {
	t.Helper()
	data := rules.Parse(strings.Join([]string{
		"1. Game Concepts",
		"100. General",
		"100.1. These Magic rules apply to any Magic game with two or more players.",
		"",
		"3. Card Types",
		"302. Creatures",
		"302.1. A player who has priority may cast a creature card from their hand.",
		"",
		"7. Additional Rules",
		"701. Keyword Actions",
		"701.13. Sacrifice",
		"701.13a To sacrifice a permanent, its controller moves it from the battlefield to that player’s graveyard.",
		"701.13b A player can’t sacrifice something that isn’t a permanent.",
		"702. Keyword Abilities",
		"702.9. Flying",
		"702.9a Flying is an evasion ability.",
		"702.9b A creature with flying can’t be blocked except by creatures with flying and/or reach.",
		"",
		"Glossary",
		"",
		"Flying",
		"A keyword ability that restricts how a creature can be blocked. See rule 702.9.",
		"",
		"Graveyard",
		"A zone. A player’s graveyard is their discard pile.",
		"",
		"Sacrifice",
		"A keyword action. To sacrifice a permanent is to move it to its owner’s graveyard.",
		"",
		"Credits",
		"",
	}, "\n"))

	if len(data.Rules) == 0 || len(data.Glossary) == 0 {
		t.Fatalf("the fixture did not parse: %d rules, %d glossary entries",
			len(data.Rules), len(data.Glossary))
	}
	return data
}

func TestSearchingTheRules(t *testing.T) {
	data := fakeRules(t)
	v := newRuleSearch(data, "sacrifice")

	if len(v.rows) == 0 {
		t.Fatal("nothing matched a word that is in there")
	}
	found := false
	for _, r := range v.rows {
		if strings.HasPrefix(r.number, "701.13") {
			found = true
		}
	}
	if !found {
		t.Errorf("the sacrifice rules are missing from %v", v.rows)
	}
}

func TestAQuotedPhraseIsOneTerm(t *testing.T) {
	// One query syntax across the program: the rules bar quotes the same
	// way the card filter does.
	data := fakeRules(t)
	loose := newRuleSearch(data, "evasion ability")
	exact := newRuleSearch(data, `"evasion ability"`)

	if len(exact.rows) > len(loose.rows) {
		t.Errorf("quoting widened the search: %d vs %d", len(exact.rows), len(loose.rows))
	}
	if len(exact.rows) == 0 {
		t.Error("the exact phrase is in the rules and matched nothing")
	}
}

func TestRelevancePutsTheRuleThatIsAboutItFirst(t *testing.T) {
	// A rule that opens with what you asked about is about it; one that
	// mentions it in passing is not.
	data := fakeRules(t)
	v := newRuleSearch(data, "flying")

	if len(v.rows) < 2 {
		t.Skip("not enough hits to rank")
	}
	first := v.rows[0]
	if !strings.Contains(strings.ToLower(first.number+first.text+first.entry.Term), "flying") {
		t.Errorf("the top hit is %v", first)
	}
}

func TestSortingByRuleNumberIsNumeric(t *testing.T) {
	// 100.2 comes before 100.10, which string comparison gets backwards.
	if !lessRuleNumber("100.2", "100.10") {
		t.Error("100.2 sorted after 100.10")
	}
	if !lessRuleNumber("100.1a", "100.2") {
		t.Error("100.1a sorted after 100.2")
	}
	if lessRuleNumber("702.9", "509.1") {
		t.Error("702.9 sorted before 509.1")
	}
}

func TestACardsRulesAreGrouped(t *testing.T) {
	// A card with an ability, an action and a zone raises three different
	// questions; running them together makes you read all of it to find the
	// one you wanted.
	data := fakeRules(t)
	card := mtg.Card{
		Name: "Test Bird", TypeLine: "Creature — Bird",
		OracleText: "Flying\nSacrifice this creature: draw a card. Put it into your graveyard.",
	}

	v := newCardRules(data, card)
	var headings []string
	for _, r := range v.rows {
		if r.kind == rowHeading {
			headings = append(headings, r.heading)
		}
	}
	if len(headings) == 0 {
		t.Fatalf("nothing was grouped; rows are %v", v.rows)
	}
	// Whatever matched, the headings must come in the fixed order.
	order := map[string]int{
		"abilities": 0, "actions": 1, "ability words": 2,
		"zones": 3, "card types": 4, "glossary": 5,
	}
	for i := 1; i < len(headings); i++ {
		if order[headings[i-1]] >= order[headings[i]] {
			t.Errorf("headings came out %v", headings)
		}
	}
}

func TestTheCursorStepsOverHeadings(t *testing.T) {
	// A heading is a label, not a row you can be on.
	v := &rulesView{rows: []ruleRow{
		{kind: rowHeading, heading: "abilities"},
		{kind: rowRule, number: "702.9"},
		{kind: rowHeading, heading: "zones"},
		{kind: rowRule, number: "400.1"},
	}}
	v.cursor.at = v.firstSelectable()
	if v.cursor.at != 1 {
		t.Fatalf("started on row %d", v.cursor.at)
	}

	v.step(1)
	if v.cursor.at != 3 {
		t.Errorf("j landed on row %d, want the rule past the heading", v.cursor.at)
	}
	v.step(1)
	if v.cursor.at != 3 {
		t.Errorf("j ran off the end to %d", v.cursor.at)
	}
	v.step(-1)
	if v.cursor.at != 1 {
		t.Errorf("k landed on row %d", v.cursor.at)
	}
}

func TestAKeywordRuleShowsSomethingWorthReading(t *testing.T) {
	// 702.9 is the word "Flying" and nothing else; everything you wanted is
	// in 702.9a. Listing "Flying  Flying" says nothing at all.
	data := fakeRules(t)
	card := mtg.Card{Name: "Bird", TypeLine: "Creature — Bird", OracleText: "Flying"}

	v := newCardRules(data, card)
	for _, r := range v.rows {
		if r.kind == rowRule && r.term == "Flying" {
			if repeatsTerm(r.text, r.term) {
				t.Errorf("the row body is just the word again: %q", r.text)
			}
			return
		}
	}
	t.Skip("flying did not match; the parser's business, not the view's")
}

func TestFilteringDropsEmptiedHeadings(t *testing.T) {
	// A heading with nothing under it is a lie.
	v := &rulesView{all: []ruleRow{
		{kind: rowHeading, heading: "abilities"},
		{kind: rowRule, number: "702.9", text: "flying"},
		{kind: rowHeading, heading: "zones"},
		{kind: rowRule, number: "400.1", text: "graveyard"},
	}, grouped: true}
	v.refresh()
	v.setFilter("flying")

	for _, r := range v.rows {
		if r.kind == rowHeading && r.heading == "zones" {
			t.Errorf("an empty heading survived: %v", v.rows)
		}
	}
}

func TestTheInfoPanelCarriesTheWholeRule(t *testing.T) {
	// The list is numbers and first lines; a rule is a paragraph, and the
	// sub-rules are usually where the behaviour actually is.
	data := fakeRules(t)
	v := newRuleSearch(data, "sacrifice")

	// The glossary entry outranks the rule for a bare term, which is
	// right; this test is about what the panel shows for a rule.
	found := false
	for i, r := range v.rows {
		if r.kind == rowRule && strings.HasPrefix(r.number, "701.13") {
			v.cursor.at, found = i, true
			break
		}
	}
	if !found {
		t.Skip("no rule matched")
	}

	info := strings.Join(v.info(60), "\n")
	if !strings.Contains(info, "701.13") {
		t.Errorf("the rule number is missing:\n%s", info)
	}
	if !strings.Contains(info, "battlefield") {
		t.Errorf("the sub-rules did not come with it:\n%s", info)
	}
}

func TestZonesAreTheirOwnGroup(t *testing.T) {
	if !rules.IsZone("graveyard") || !rules.IsZone("Battlefield") {
		t.Error("a zone was not recognised")
	}
	if rules.IsZone("flying") {
		t.Error("flying is not a zone")
	}
}

func TestOpeningTheRulesOverACard(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m.rules = fakeRules(t)

	m = drive(m, "space", "r")
	p := m.ws.current()
	if p.kind != KindRules {
		t.Fatalf("space r opened a %v panel", p.kind)
	}
	if p.searchOpen {
		t.Error("it opened on a search bar with a card under the cursor")
	}
	if _, ok := p.top().(*rulesView); !ok {
		t.Fatalf("the panel is showing %T", p.top())
	}
}

func TestOpeningTheRulesWithNoCardGivesYouTheBar(t *testing.T) {
	m := sized(140, 30)
	m.rules = fakeRules(t)

	m = drive(m, "space", "r")
	p := m.ws.current()
	if !p.searchOpen {
		t.Error("with nothing highlighted there is nothing to explain, so it should ask")
	}
}
