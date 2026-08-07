package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// ── Helpers ─────────────────────────────────────────────────────

// drive feeds messages through Update like the runtime would, running the
// resulting commands (including batches) so async work actually lands.
func drive(m model, msgs ...tea.Msg) model {
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next.(model)
		m = runCmd(m, cmd, 0)
	}
	return m
}

func runCmd(m model, cmd tea.Cmd, depth int) model {
	if cmd == nil || depth > 4 {
		return m
	}
	out := cmd()
	if out == nil {
		return m
	}
	if batch, ok := out.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runCmd(m, c, depth+1)
		}
		return m
	}
	if _, ok := out.(tea.QuitMsg); ok {
		return m
	}
	next, nextCmd := m.Update(out)
	return runCmd(next.(model), nextCmd, depth+1)
}

func visibleLen(s string) int {
	n, inEsc := 0, false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		default:
			n++
		}
	}
	return n
}

// frameCheck asserts a rendered frame fits the terminal exactly.
func frameCheck(t *testing.T, name, frame string, w, h int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) > h {
		t.Errorf("%s: %d lines > height %d", name, len(lines), h)
	}
	for i, l := range lines {
		if n := visibleLen(l); n > w {
			t.Errorf("%s: line %d is %d cols > width %d: %q", name, i, n, w, l)
		}
	}
}

func testCards() []ScryfallCard {
	return []ScryfallCard{
		{
			ID: "a", Name: "Dragonlord Ojutai", ManaCost: "{3}{W}{U}",
			TypeLine: "Legendary Creature — Elder Dragon", Colors: []string{"W", "U"},
			Power: "5", Toughness: "4", SetName: "Dragons of Tarkir", Rarity: "mythic", CMC: 5,
			RulingsURI: "http://example.invalid/a",
			OracleText: "Flying\nDragonlord Ojutai has hexproof as long as it's untapped.\n" +
				"Whenever Dragonlord Ojutai deals combat damage to a player, look at the top three cards of your library.",
			Legalities: map[string]string{"commander": "legal", "modern": "legal", "standard": "not_legal"},
		},
		{
			ID: "b", Name: "Goldspan Dragon", ManaCost: "{3}{R}{R}",
			TypeLine: "Legendary Creature — Dragon", Colors: []string{"R"},
			Power: "4", Toughness: "4", SetName: "Kaldheim", Rarity: "mythic", CMC: 5,
			RulingsURI: "http://example.invalid/b",
			OracleText: "Flying, haste\nWhenever this creature attacks or becomes the target of a spell, create a Treasure token.\n" +
				"Treasures you control have \"{T}, Sacrifice this artifact: Add two mana of any one color.\"",
			Legalities: map[string]string{"commander": "legal", "modern": "legal"},
		},
		{
			ID: "c", Name: "Test Walker", ManaCost: "{2}{R}",
			TypeLine: "Legendary Planeswalker — Test", Colors: []string{"R"},
			Loyalty: "4", SetName: "Testing", Rarity: "rare", CMC: 3,
			RulingsURI: "http://example.invalid/c",
			OracleText: "+1: Test Walker deals 2 damage to any target.\n" +
				"−3: Destroy target creature. It can't be regenerated. (This is reminder text.)\n" +
				"Target creature gets +2/+2 and gains trample until end of turn.",
			Legalities: map[string]string{"commander": "legal"},
		},
	}
}

func resultsModel(t *testing.T, w, h int, data RulesData) model {
	t.Helper()
	m := initialModel()
	m.rules = data
	m.searchInput.SetValue("t:dragon f:commander")
	m = drive(m, tea.WindowSizeMsg{Width: w, Height: h})
	return drive(m, searchResultMsg{cards: testCards(), totalCards: 190})
}

func loadTestRules(t *testing.T) RulesData {
	t.Helper()
	data, err := loadRules()
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
	card := testCards()[0]

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

// ── Highlighting ────────────────────────────────────────────────

func TestHighlightWrapsToWidth(t *testing.T) {
	data := loadTestRules(t)

	for _, c := range testCards() {
		for _, w := range []int{20, 33, 48, 70} {
			out := highlightOracle(c, w, data)
			for _, line := range strings.Split(out, "\n") {
				if n := visibleLen(line); n > w {
					t.Errorf("%s at width %d: %d visible cols: %q", c.Name, w, n, line)
				}
			}
			if stripANSI(out) == "" {
				t.Errorf("%s at width %d: highlighting dropped the text", c.Name, w)
			}
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func TestHighlightPreservesText(t *testing.T) {
	data := loadTestRules(t)
	c := testCards()[1]

	got := stripANSI(highlightOracle(c, 200, data))
	want := strings.ReplaceAll(c.OracleText, "\n", "\n")
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Errorf("highlighting changed the text:\n got %q\nwant %q", got, want)
	}
}

// ── Views ───────────────────────────────────────────────────────

func TestResultsFrameFitsTerminal(t *testing.T) {
	data := loadTestRules(t)

	// One size per layout branch: stacked (narrow) and side by side.
	for _, size := range [][2]int{{100, 30}, {140, 40}, {200, 50}} {
		w, h := size[0], size[1]
		m := resultsModel(t, w, h, data)

		if m.state != stateResults {
			t.Fatalf("%dx%d: expected results state, got %v", w, h, m.state)
		}
		frameCheck(t, "results", m.View(), w, h)

		// Every panel mode has to hold the same frame.
		for _, keys := range []string{"r", "s", "t", "rs", "sr", "rt", "JJ", "R"} {
			mk := m
			for _, r := range keys {
				mk = drive(mk, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			frameCheck(t, "results+"+keys, mk.View(), w, h)
		}

		// Rules browser, scoped to the selected card.
		mb := drive(m, tea.KeyMsg{Type: tea.KeyEnter})
		if mb.state != stateRules {
			t.Fatalf("%dx%d: enter did not open the rules browser", w, h)
		}
		frameCheck(t, "browser", mb.View(), w, h)

		mg := drive(mb, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
		frameCheck(t, "glossary", mg.View(), w, h)

		if back := drive(mg, tea.KeyMsg{Type: tea.KeyEsc}); back.state != stateResults {
			t.Errorf("%dx%d: esc did not return to the results", w, h)
		}
	}
}

// Starting up drops you straight on the results screen with the search
// bar focused — there is no separate search page to pass through.
func TestStartsOnResultsScreen(t *testing.T) {
	m := initialModel()
	if m.state != stateResults {
		t.Errorf("initial state is %v, want the results screen", m.state)
	}
	if !m.searchFocused {
		t.Error("the search bar does not have focus at startup")
	}

	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m.View()
	frameCheck(t, "startup", view, 120, 40)

	plain := stripANSI(view)
	if strings.Contains(plain, "Max") {
		t.Error("the header still offers a max-results setting")
	}
	if strings.Contains(plain, "scryfall-tui") || strings.Contains(plain, "scry-tui") {
		t.Error("the app name is still in the top left corner")
	}
	if !strings.Contains(plain, "⌕") {
		t.Error("no search bar in the header")
	}
}

// A query passed on the command line goes straight to results, showing
// its progress in the header rather than on another screen.
func TestInitialQuerySkipsTheSearchScreen(t *testing.T) {
	m := initialModel()
	m.searchInput.SetValue("t:creature")
	m.initialQuery = "t:creature"
	m.searching = true
	m = drive(m, tea.WindowSizeMsg{Width: 140, Height: 40})

	if m.state != stateResults {
		t.Fatalf("state is %v, want the results screen", m.state)
	}
	first := stripANSI(m.View())
	if !strings.Contains(first, "t:creature") {
		t.Error("the query is not shown in the search bar while it runs")
	}
	if !strings.Contains(first, "searching") {
		t.Error("no indication that a search is in flight")
	}

	// When the results land, focus moves to the list.
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 190})
	if m.searching {
		t.Error("still marked as searching after the results arrived")
	}
	if m.searchFocused {
		t.Error("focus stayed in the search bar after the results arrived")
	}
}

// Focus moves between the search bar and the list without changing screens.
func TestSearchBarFocus(t *testing.T) {
	data := loadTestRules(t)
	m := resultsModel(t, 140, 40, data)

	if m.searchFocused {
		t.Fatal("focus should be on the list once results are in")
	}

	// Typing a letter that is also a panel key must reach the list, not the bar.
	rules := drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if rules.panel != panelRules {
		t.Error("r was swallowed by the search bar instead of switching the panel")
	}

	// i (or esc) puts focus back in the search bar, where the same letter types.
	editing := drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !editing.searchFocused {
		t.Fatal("i did not focus the search bar")
	}
	typed := drive(editing, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !strings.HasSuffix(typed.searchInput.Value(), "r") {
		t.Errorf("typing did not reach the search bar: %q", typed.searchInput.Value())
	}

	// Arrows still move the list while typing.
	moved := drive(typed, tea.KeyMsg{Type: tea.KeyDown})
	if moved.resultList.Index() != 1 {
		t.Error("arrow keys do not move the list while the search bar has focus")
	}

	// esc hands focus back to the list.
	if back := drive(typed, tea.KeyMsg{Type: tea.KeyEsc}); back.searchFocused {
		t.Error("esc did not return focus to the list")
	}
}

func TestResultsHeaderLines(t *testing.T) {
	data := loadTestRules(t)
	m := resultsModel(t, 140, 40, data)

	header := stripANSI(m.resultsHeader())
	for _, want := range []string{"⌕", "Sort", "Results"} {
		if !strings.Contains(header, want) {
			t.Errorf("header missing %q:\n%s", want, header)
		}
	}
	if strings.Contains(header, "max") {
		t.Errorf("header still shows a max: %s", header)
	}
	if got := strings.Count(m.resultsHeader(), "\n") + 1; got != headerLines {
		t.Errorf("header is %d lines, layout assumes %d", got, headerLines)
	}
}

// ── Rulings ─────────────────────────────────────────────────────

func TestRulingsAreDebounced(t *testing.T) {
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})

	cards := testCards()
	m.cards = cards
	m.state = stateResults
	m.resultList.SetItems([]list.Item{
		cardItem{card: cards[0]}, cardItem{card: cards[1]},
	})
	m.resultList.ResetSelected()

	m, _ = m.syncHover() // hovering the first card; a tick is pending
	staleSeq := m.hoverSeq
	if staleSeq == 0 {
		t.Fatal("hovering a card did not schedule a rulings fetch")
	}

	// Move on before the tick fires.
	moved := drive(m, tea.KeyMsg{Type: tea.KeyDown})
	if moved.hoverKey != "b" {
		t.Fatalf("hover key = %q, want b", moved.hoverKey)
	}

	// The stale tick arrives late and must be ignored.
	late, _ := moved.Update(rulingsTickMsg{key: "a", uri: cards[0].RulingsURI, seq: staleSeq})
	if late.(model).inflight["a"] {
		t.Error("fetched rulings for a card that was already scrolled past")
	}

	// A tick that still matches the hover does fetch.
	current, _ := moved.Update(rulingsTickMsg{key: "b", uri: cards[1].RulingsURI, seq: moved.hoverSeq})
	if !current.(model).inflight["b"] {
		t.Error("did not fetch rulings for the card under the cursor")
	}
}

func TestRulingsCacheIsReused(t *testing.T) {
	data := loadTestRules(t)
	m := resultsModel(t, 160, 40, data)

	key := cardKey(testCards()[0])
	m.rulings[key] = []Ruling{{Comment: "Cached ruling."}}

	// Re-hovering a cached card must not schedule another fetch.
	m.hoverKey = ""
	next, cmd := m.syncHover()
	if cmd != nil {
		t.Error("re-hovering a cached card scheduled another request")
	}
	if !strings.Contains(stripANSI(next.renderPreview(testCards()[0], 60)), "Cached ruling.") {
		t.Error("cached rulings are not shown in the card panel")
	}
}

// The panel shows one thing at a time: r and s swap it, and pressing the
// same key again brings the card back.
func TestPanelModes(t *testing.T) {
	data := loadTestRules(t)
	m := resultsModel(t, 160, 40, data)

	if m.panel != panelCard {
		t.Fatalf("panel starts as %v, want the card", m.panel)
	}
	if strings.Contains(stripANSI(m.View()), "Rules ·") {
		t.Error("the rules panel is visible before r is pressed")
	}

	press := func(m model, r rune) model {
		return drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	rules := press(m, 'r')
	if rules.panel != panelRules {
		t.Errorf("r left the panel on %v", rules.panel)
	}
	view := stripANSI(rules.View())
	if !strings.Contains(view, "Rules ·") {
		t.Error("r did not show the rules")
	}
	if strings.Contains(view, "Oracle Text") {
		t.Error("the card view is still drawn alongside the rules")
	}

	if back := press(rules, 'r'); back.panel != panelCard {
		t.Errorf("r again left the panel on %v", back.panel)
	}

	stats := press(rules, 's')
	if stats.panel != panelStats {
		t.Errorf("s from the rules left the panel on %v", stats.panel)
	}
	if !strings.Contains(stripANSI(stats.View()), "Statistics") {
		t.Error("s did not show the statistics")
	}
	if back := press(stats, 's'); back.panel != panelCard {
		t.Errorf("s again left the panel on %v", back.panel)
	}
}

// Each panel keeps its own scroll position.
func TestPanelScrolling(t *testing.T) {
	data := loadTestRules(t)

	// A short terminal, so the card panel has more to show than it can fit.
	m := resultsModel(t, 160, 14, data)

	scrollDown := func(m model) model {
		return drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	}
	press := func(m model, r rune) model {
		return drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	m = scrollDown(scrollDown(m))
	if m.previewScroll != 2 || m.rulesScroll != 0 {
		t.Fatalf("card scroll = %d, rules scroll = %d; want 2 and 0", m.previewScroll, m.rulesScroll)
	}

	m = scrollDown(press(m, 'r'))
	if m.rulesScroll != 1 || m.previewScroll != 2 {
		t.Errorf("rules scroll = %d, card scroll = %d; want 1 and 2", m.rulesScroll, m.previewScroll)
	}

	// Moving to another card resets both.
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.previewScroll != 0 || m.rulesScroll != 0 {
		t.Errorf("moving cards left scroll at %d/%d", m.previewScroll, m.rulesScroll)
	}

	// Scrolling stops at the end of the content rather than running on
	// into blank space.
	m = press(m, 'r')
	for i := 0; i < 200; i++ {
		m = scrollDown(m)
	}
	lines := strings.Count(m.panelContent(m.resultsLayout().panelW-4), "\n") + 1
	if m.rulesScroll >= lines {
		t.Errorf("scrolled to line %d of %d lines of content", m.rulesScroll, lines)
	}
	atEnd := m.rulesScroll
	m = scrollDown(m)
	if m.rulesScroll != atEnd {
		t.Errorf("scroll moved past the end: %d then %d", atEnd, m.rulesScroll)
	}

	// A panel that fits doesn't scroll at all.
	tall := resultsModel(t, 160, 60, data)
	if got := scrollDown(scrollDown(tall)).previewScroll; got != 0 {
		t.Errorf("a panel with room to spare scrolled to %d", got)
	}
}

// A card with no rulings has to come back as an empty slice, not an error,
// so the one-shot printer and the panel can both say "none" plainly.
func TestGetRulingsWithoutURI(t *testing.T) {
	rulings, err := getRulings("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rulings == nil {
		t.Error("got a nil slice, want an empty one")
	}
	if len(rulings) != 0 {
		t.Errorf("got %d rulings for an empty URI", len(rulings))
	}
}

func TestStdoutWidthIsSane(t *testing.T) {
	w := stdoutWidth()
	if w < 30 || w > 100 {
		t.Errorf("stdout width %d is outside the 30–100 range", w)
	}
}

// ── Printed-text history ────────────────────────────────────────

func TestCleanOriginal(t *testing.T) {
	cases := map[string]string{
		"ocT: Add {G} to your mana pool.":       "{T}: Add {G} to your mana pool.",
		"Flying// Tap to add one mana.":         "Flying\nTap to add one mana.",
		"Assault deals 2 damage. // Create it.": "Assault deals 2 damage.\nCreate it.",
		"  Flying  ":                            "Flying",
	}
	for in, want := range cases {
		if got := cleanOriginal(in); got != want {
			t.Errorf("cleanOriginal(%q) = %q, want %q", in, got, want)
		}
	}
}

// The history lists wordings, not printings: twenty reprints with the same
// text are one entry, but a wording that comes back later is its own.
func TestBuildRevisionsCollapsesIdenticalPrintings(t *testing.T) {
	card := ScryfallCard{Name: "Test Bird", OracleText: "Flying\n{T}: Add one mana of any color."}
	printings := []printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha Set", Released: "1993-08-05"},
		{Name: "Test Bird", Set: "BBB", SetName: "Beta Set", Released: "1993-10-04"},
		{Name: "Test Bird", Set: "CCC", SetName: "Third Set", Released: "1994-04-11"},
		{Name: "Test Bird", Set: "DDD", SetName: "Fourth Set", Released: "1995-04-01"},
		{Name: "Test Bird", Set: "EEE", SetName: "Fifth Set", Released: "2019-01-25"},
	}
	originals := map[string]map[string]string{
		"AAA": {"test bird": "Flying\nocT: Add one mana of any color to your mana pool."},
		"BBB": {"test bird": "Flying\n{T}: Add one mana of any color to your mana pool."}, // same but for the symbol encoding
		"CCC": {"test bird": "Flying (Reminder.)\n{T}: Add one mana of any color to your mana pool."},
		"DDD": {"test bird": "Flying\n{T}: Add one mana of any color to your mana pool."}, // back to the earlier wording
		"EEE": {"test bird": "Flying\n{T}: Add one mana of any color."},
	}

	revs := buildRevisions(card, printings, originals)
	if len(revs) != 4 {
		for _, r := range revs {
			t.Logf("  %s (%d printings): %q", r.SetCode, r.Printings, r.Text)
		}
		t.Fatalf("got %d wordings, want 4", len(revs))
	}

	if revs[0].SetCode != "AAA" || revs[0].Printings != 2 {
		t.Errorf("first wording = %s covering %d printings, want AAA covering 2",
			revs[0].SetCode, revs[0].Printings)
	}
	if revs[1].SetCode != "CCC" {
		t.Errorf("second wording = %s, want CCC", revs[1].SetCode)
	}
	if revs[2].SetCode != "DDD" {
		t.Errorf("third wording = %s, want DDD (the wording returned)", revs[2].SetCode)
	}
	if !revs[3].Current || revs[3].SetCode != "EEE" {
		t.Errorf("last wording = %s current=%v, want EEE marked current",
			revs[3].SetCode, revs[3].Current)
	}
}

// When no printing carries the current oracle wording it's still shown last.
func TestBuildRevisionsAppendsCurrentOracle(t *testing.T) {
	card := ScryfallCard{Name: "Test Bird", OracleText: "Flying"}
	printings := []printing{{Name: "Test Bird", Set: "AAA", SetName: "Alpha Set", Released: "1993-08-05"}}
	originals := map[string]map[string]string{"AAA": {"test bird": "Does not tap when attacking."}}

	revs := buildRevisions(card, printings, originals)
	if len(revs) != 2 {
		t.Fatalf("got %d wordings, want 2", len(revs))
	}
	if !revs[1].Current || revs[1].Text != "Flying" {
		t.Errorf("last entry = %+v, want the current oracle text", revs[1])
	}
}

// Nothing is downloaded just by moving the cursor — only the key does that.
func TestHistoryNeverFetchesOnHover(t *testing.T) {
	data := loadTestRules(t)
	m := resultsModel(t, 160, 40, data)

	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyDown})
	if len(m.histories) != 0 {
		t.Errorf("moving the cursor started %d history lookups", len(m.histories))
	}
}

func TestHistoryPanelWithoutOracleID(t *testing.T) {
	data := loadTestRules(t)
	m := resultsModel(t, 160, 40, data)

	// The test cards carry no oracle id, so this must not reach the network.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if m.panel != panelHistory {
		t.Fatalf("t left the panel on %v", m.panel)
	}
	if len(m.histories) != 0 {
		t.Error("a card with no oracle id started a lookup anyway")
	}
	if !strings.Contains(stripANSI(m.renderTextHistory(testCards()[0], 60)), "oracle id") {
		t.Error("the panel does not explain why there's no history")
	}
}

// The set data assembles into a history once every set has reported in.
func TestHistoryAssemblesFromCachedSets(t *testing.T) {
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})

	card := ScryfallCard{
		ID: "x", OracleID: "oid", Name: "Test Bird",
		OracleText: "Flying", TypeLine: "Creature — Bird",
	}
	m.cards = []ScryfallCard{card}
	m.resultList.SetItems([]list.Item{cardItem{card: card}})
	m.histories["oid"] = &cardHistory{state: histPrintings}

	// Both sets are already in memory, so this resolves without any fetch.
	m.originals["AAA"] = map[string]string{"test bird": "Does not tap when attacking."}
	m.originals["BBB"] = map[string]string{"test bird": "Flying"}

	next, cmd := m.Update(printingsMsg{oracleID: "oid", printings: []printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha Set", Released: "1993-08-05"},
		{Name: "Test Bird", Set: "BBB", SetName: "Beta Set", Released: "1994-04-11"},
	}})
	m = next.(model)
	if cmd != nil {
		t.Error("issued a fetch even though every set was cached")
	}

	h := m.histories["oid"]
	if h.state != histReady {
		t.Fatalf("history state = %v, want ready", h.state)
	}
	if len(h.revisions) != 2 {
		t.Fatalf("got %d wordings, want 2", len(h.revisions))
	}

	m.panel = panelHistory
	panel := stripANSI(m.renderTextHistory(card, 60))
	for _, want := range []string{"Text History", "2 wordings", "Alpha Set", "Does not tap"} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel missing %q:\n%s", want, panel)
		}
	}
}

// ── Moxfield decks ──────────────────────────────────────────────

func TestDeckReferenceParsing(t *testing.T) {
	urls := map[string]string{
		"https://moxfield.com/decks/j-0aJlxuOUm9FnKRvJcfZw": "j-0aJlxuOUm9FnKRvJcfZw",
		"https://www.moxfield.com/decks/Y8dZ7":              "Y8dZ7",
		"http://moxfield.com/decks/Y8dZ7/primer":            "Y8dZ7",
		"moxfield.com/decks/Y8dZ7?utm_source=x":             "Y8dZ7",
		"  https://moxfield.com/decks/Y8dZ7  ":              "Y8dZ7",
	}
	for in, want := range urls {
		got, ok := moxfieldURLID(in)
		if !ok || got != want {
			t.Errorf("moxfieldURLID(%q) = %q, %t; want %q, true", in, got, ok, want)
		}
	}

	// A bare word is a Scryfall query, not a deck — otherwise typing a card
	// name in the search bar would try to load a deck.
	for _, in := range []string{"ghen", "t:dragon c:R", "", "archidekt.com/decks/123"} {
		if id, ok := moxfieldURLID(in); ok {
			t.Errorf("moxfieldURLID(%q) = %q, true; want no match", in, id)
		}
	}

	// `scry deck` is explicit, so it takes the bare id too.
	if id, ok := deckRef("Y8dZ7"); !ok || id != "Y8dZ7" {
		t.Errorf("deckRef(bare id) = %q, %t; want Y8dZ7, true", id, ok)
	}
	if id, ok := deckRef("https://moxfield.com/decks/Y8dZ7"); !ok || id != "Y8dZ7" {
		t.Errorf("deckRef(url) = %q, %t; want Y8dZ7, true", id, ok)
	}
	if _, ok := deckRef("t:dragon c:R"); ok {
		t.Error("deckRef accepted a query with spaces")
	}
}

func TestPrimaryTypePicksOneSection(t *testing.T) {
	cases := map[string]string{
		"Legendary Creature — Human Warrior": "Creature",
		"Artifact Creature — Golem":          "Creature",
		"Enchantment Creature — Nightmare":   "Creature",
		"Artifact Land":                      "Land",
		"Land Creature — Forest Dryad":       "Creature",
		"Legendary Enchantment Land":         "Land",
		"Instant":                            "Instant",
		"Legendary Planeswalker — Teferi":    "Planeswalker",
		"Sorcery // Land":                    "Sorcery",
		"Basic Land — Mountain":              "Land",
		"Dungeon":                            "Other",
	}
	for line, want := range cases {
		if got := primaryType(line); got != want {
			t.Errorf("primaryType(%q) = %q, want %q", line, got, want)
		}
	}
}

// deckFixture is a small deck covering the things grouping has to get right:
// a commander, repeat copies, and more than one section.
func deckFixture() []deckCard {
	cards := testCards()
	return []deckCard{
		{card: cards[1], qty: 1, commander: true}, // Goldspan Dragon
		{card: cards[0], qty: 1},                  // Dragonlord Ojutai — creature
		{card: cards[2], qty: 1},                  // Test Walker — planeswalker
		{card: ScryfallCard{ID: "m", Name: "Mountain", TypeLine: "Basic Land — Mountain"}, qty: 30},
		{card: ScryfallCard{ID: "z", Name: "Ancient Tomb", TypeLine: "Land"}, qty: 1},
	}
}

func TestDeckItemsOrderByType(t *testing.T) {
	items := deckItems(deckFixture())

	var layout []string
	for _, it := range items {
		if ci, ok := it.(cardItem); ok {
			layout = append(layout, ci.card.Name)
		}
	}

	// Commander first, then creatures, spells and lands — alphabetically
	// inside each group, and with no heading rows between them.
	want := []string{
		"Goldspan Dragon",
		"Dragonlord Ojutai",
		"Test Walker",
		"Ancient Tomb", "Mountain",
	}
	if strings.Join(layout, "|") != strings.Join(want, "|") {
		t.Errorf("deck order =\n  %v\nwant\n  %v", layout, want)
	}
	if len(items) != len(want) {
		t.Errorf("deck produced %d rows for %d cards — headings are back", len(items), len(want))
	}
}

func TestDeckStatsCountCopies(t *testing.T) {
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Test Deck", total: 34, unique: 5},
		cards: deckFixture(),
	})

	stats := stripANSI(m.renderStats(60))
	if !strings.Contains(stats, "Statistics (34 cards)") {
		t.Errorf("statistics did not count copies:\n%s", stats)
	}

	// 30 Mountains have to show up as 30 lands, not one.
	for _, line := range strings.Split(stats, "\n") {
		if strings.Contains(line, "Land") && strings.HasSuffix(strings.TrimSpace(line), "31") {
			return
		}
	}
	t.Errorf("Land count is not 31:\n%s", stats)
}

func TestDeckHeaderShowsDeckNotSort(t *testing.T) {
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Winota: Snowball Stax", author: "ComedIan", format: "commander", total: 100, unique: 98},
		cards: deckFixture(),
	})

	header := stripANSI(m.resultsHeader())
	for _, want := range []string{"Deck", "Winota: Snowball Stax", "by ComedIan", "commander", "100 cards", "98 unique"} {
		if !strings.Contains(header, want) {
			t.Errorf("deck header missing %q:\n%s", want, header)
		}
	}
	// The layout budget is fixed, so the deck header has to be the same
	// height as the search one.
	if got := strings.Count(m.resultsHeader(), "\n") + 1; got != headerLines {
		t.Errorf("deck header is %d lines, layout assumes %d", got, headerLines)
	}
}

func TestSearchAfterDeckClearsIt(t *testing.T) {
	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{info: deckInfo{name: "Test Deck", total: 34}, cards: deckFixture()})
	if m.deck == nil {
		t.Fatal("deck did not load")
	}

	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	if m.deck != nil {
		t.Error("a new search left the deck header in place")
	}
	if !strings.Contains(stripANSI(m.resultsHeader()), "Sort") {
		t.Error("header did not go back to showing the sort order")
	}
}

// ── Layout ──────────────────────────────────────────────────────

func TestListColumnsFitTheirWidth(t *testing.T) {
	// 2 for the cursor and 2 between each pair of columns.
	const chrome = 6
	for _, w := range []int{40, 60, 75, 80, 100, 120, 160, 240} {
		nameW, manaW, typeW := listColumns(w)
		if got := nameW + manaW + typeW + chrome; got > w {
			t.Errorf("at width %d the row needs %d columns (%d/%d/%d)", w, got, nameW, manaW, typeW)
		}
		if nameW < 10 || typeW < 8 || manaW < 1 {
			t.Errorf("at width %d the columns collapsed: %d/%d/%d", w, nameW, manaW, typeW)
		}
	}

	// Wider terminals give the columns more room, not the same fixed 30.
	narrow, _, _ := listColumns(80)
	wide, _, _ := listColumns(200)
	if wide <= narrow {
		t.Errorf("name column did not grow with the terminal: %d at 80, %d at 200", narrow, wide)
	}
}

func TestManaNeverOverflowsItsColumn(t *testing.T) {
	// A four-symbol hybrid cost is 12 characters wide unabbreviated.
	rendered, width := renderManaWidth("{R/W}{R/W}{R/W}{R/W}", 10)
	if width > 10 {
		t.Errorf("hybrid cost rendered %d wide, column is 10", width)
	}
	if !strings.Contains(stripANSI(rendered), "…") {
		t.Errorf("a truncated cost should say so: %q", stripANSI(rendered))
	}
	// Cutting mid-symbol would turn R/W into a different cost.
	if strings.HasSuffix(strings.TrimSuffix(stripANSI(rendered), "…"), "/") {
		t.Errorf("cost was cut through a symbol: %q", stripANSI(rendered))
	}

	if _, width := renderManaWidth("{3}{W}{U}", 10); width != 3 {
		t.Errorf("a cost that fits should render whole, got width %d", width)
	}
}

// ── Double-faced cards ──────────────────────────────────────────

func dfcFixture() ScryfallCard {
	return ScryfallCard{
		ID: "dfc", Name: "Slicer, Hired Muscle // Slicer, High-Speed Antagonist",
		TypeLine: "Legendary Artifact Creature — Robot // Legendary Artifact — Vehicle",
		SetName:  "Transformers", Rarity: "mythic", CMC: 5,
		CardFaces: []CardFace{
			{
				Name: "Slicer, Hired Muscle", ManaCost: "{4}{R}",
				TypeLine: "Legendary Artifact Creature — Robot", Colors: []string{"R"},
				Power: "3", Toughness: "4",
				OracleText: "Double strike, haste",
			},
			{
				Name: "Slicer, High-Speed Antagonist", ManaCost: "",
				TypeLine: "Legendary Artifact — Vehicle", Colors: []string{"R"},
				Power: "3", Toughness: "2",
				OracleText: "Living metal\nFirst strike, haste",
			},
		},
	}
}

func TestFacesStandInForTheCard(t *testing.T) {
	c := dfcFixture()

	faces := c.faces()
	if len(faces) != 2 {
		t.Fatalf("got %d faces, want 2", len(faces))
	}
	if faces[0].Name != "Slicer, Hired Muscle" || faces[0].Power != "3" || faces[0].ManaCost != "{4}{R}" {
		t.Errorf("front face did not take the face's own details: %+v", faces[0])
	}
	// Details that belong to the card, not the face, stay put.
	if faces[1].SetName != "Transformers" || faces[1].CMC != 5 {
		t.Errorf("back face lost the card's printing details: %+v", faces[1])
	}

	// A single-faced card is its own only face.
	if got := len(testCards()[0].faces()); got != 1 {
		t.Errorf("single-faced card produced %d faces", got)
	}

	if !strings.Contains(c.combinedOracle(), "Double strike") ||
		!strings.Contains(c.combinedOracle(), "Living metal") {
		t.Errorf("combined oracle text missed a face: %q", c.combinedOracle())
	}

	// The top level carries no cost or colors for a transforming card, so
	// the list has to fall back to the front face.
	if c.displayManaCost() != "{4}{R}" {
		t.Errorf("display mana cost = %q, want the front face's", c.displayManaCost())
	}
	if len(c.displayColors()) != 1 || c.displayColors()[0] != "R" {
		t.Errorf("display colors = %v, want the front face's", c.displayColors())
	}
}

func TestPanelShowsBothFaces(t *testing.T) {
	data := loadTestRules(t)
	m := resultsModel(t, 160, 40, data)

	panel := stripANSI(m.renderPreview(dfcFixture(), 70))
	for _, want := range []string{
		"Slicer, Hired Muscle",
		"Legendary Artifact Creature — Robot",
		"P/T: 3/4",
		"Double strike, haste",
		// The back face needs its own heading, not just its text.
		"Slicer, High-Speed Antagonist",
		"Legendary Artifact — Vehicle",
		"P/T: 3/2",
		"Living metal",
		"First strike, haste",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel missing %q:\n%s", want, panel)
		}
	}
}

func TestBackFaceKeywordsMatchRules(t *testing.T) {
	data := loadTestRules(t)

	var terms []string
	for _, match := range data.MatchCard(dfcFixture()) {
		terms = append(terms, strings.ToLower(match.Term))
	}
	joined := strings.Join(terms, " ")
	// "first strike" is only on the back face.
	if !strings.Contains(joined, "first strike") {
		t.Errorf("back face keywords did not reach the rules panel: %v", terms)
	}
	if !strings.Contains(joined, "double strike") {
		t.Errorf("front face keywords went missing: %v", terms)
	}
}

// ── Author tags ─────────────────────────────────────────────────

func TestAuthorTagsInStatistics(t *testing.T) {
	cards := deckFixture()
	cards[0].tags = []string{"Ramp", "Own"}
	cards[1].tags = []string{"Ramp"}
	cards[3].tags = []string{"Land"} // the 30 Mountains

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{info: deckInfo{name: "Tagged", total: 34}, cards: cards})

	stats := stripANSI(m.renderStats(70))
	if !strings.Contains(stats, "Tags") {
		t.Fatalf("statistics has no tag breakdown:\n%s", stats)
	}

	tagPart := stats[strings.Index(stats, "Tags"):]
	// Tags count copies like everything else, so 30 Mountains is 30 lands,
	// and the commonest tag leads.
	if !strings.Contains(tagPart, "Land") || !strings.Contains(tagPart, "30") {
		t.Errorf("tag counts do not follow quantities:\n%s", tagPart)
	}
	if idxLand, idxRamp := strings.Index(tagPart, "Land"), strings.Index(tagPart, "Ramp"); idxLand > idxRamp {
		t.Errorf("tags are not ordered by count:\n%s", tagPart)
	}

	// Search results have no tags, so the section shouldn't appear at all.
	plain := drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	if strings.Contains(stripANSI(plain.renderStats(70)), "Tags") {
		t.Error("search results grew a tag section")
	}
}

// ── Saved decks ─────────────────────────────────────────────────

func TestSavedDeckRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if decks, err := loadSavedDecks(); err != nil || len(decks) != 0 {
		t.Fatalf("a fresh install should have no saved decks: %v, %v", decks, err)
	}

	err := saveDeck("ghen", savedDeck{
		Name: "Ghen reanimator", ID: "pdxwlk", URL: "https://moxfield.com/decks/pdxwlk",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, ok := lookupSavedDeck("ghen")
	if !ok || got.ID != "pdxwlk" || got.Name != "Ghen reanimator" {
		t.Fatalf("lookup returned %+v, %t", got, ok)
	}
	if got.Saved == "" {
		t.Error("saved deck has no timestamp")
	}

	// Saving again under the same name replaces it rather than duplicating.
	if err := saveDeck("ghen", savedDeck{Name: "Ghen v2", ID: "other"}); err != nil {
		t.Fatal(err)
	}
	decks, _ := loadSavedDecks()
	if len(decks) != 1 || decks["ghen"].ID != "other" {
		t.Errorf("re-saving did not replace the entry: %+v", decks)
	}

	found, err := forgetDeck("ghen")
	if err != nil || !found {
		t.Fatalf("forget returned %t, %v", found, err)
	}
	if _, ok := lookupSavedDeck("ghen"); ok {
		t.Error("deck survived being forgotten")
	}
	if found, _ := forgetDeck("ghen"); found {
		t.Error("forgetting an unknown deck reported success")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Winota: Snowball Stax": "winota-snowball-stax",
		"Ghen reanimator":       "ghen-reanimator",
		"  Trailing  ":          "trailing",
		"Ω":                     "ω",
		"!!!":                   "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveDeckFromTheApp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// On search results w has nothing to save, and must say so rather than
	// filing an empty deck. (With no results at all the search bar has
	// focus, so w is just typing.)
	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if !strings.Contains(m.notice, "nothing to save") {
		t.Errorf("notice on search results = %q", m.notice)
	}
	if !strings.Contains(stripANSI(m.resultsHeader()), "nothing to save") {
		t.Error("the notice never reached the header on search results")
	}

	m = drive(m, deckLoadedMsg{
		info:  deckInfo{name: "Winota: Snowball Stax", id: "Y8dZ7", url: "https://moxfield.com/decks/Y8dZ7", total: 34},
		cards: deckFixture(),
	})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})

	saved, ok := lookupSavedDeck("winota-snowball-stax")
	if !ok {
		t.Fatalf("w did not save the deck; notice was %q", m.notice)
	}
	if saved.ID != "Y8dZ7" {
		t.Errorf("saved the wrong deck: %+v", saved)
	}
	if !strings.Contains(m.notice, "winota-snowball-stax") {
		t.Errorf("notice does not say how to reopen it: %q", m.notice)
	}
	if !strings.Contains(stripANSI(m.resultsHeader()), "winota-snowball-stax") {
		t.Error("the confirmation never reached the header")
	}

	// The next keypress clears it.
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.notice != "" {
		t.Errorf("notice outlived the next keypress: %q", m.notice)
	}
}

// ── Statistics as a filter ──────────────────────────────────────

// taggedDeckModel is a loaded deck with author tags, sitting on the
// statistics panel.
func taggedDeckModel(t *testing.T) model {
	t.Helper()

	cards := deckFixture()
	cards[0].tags = []string{"Ramp", "Own"} // Goldspan Dragon, commander
	cards[1].tags = []string{"Ramp"}        // Dragonlord Ojutai
	cards[3].tags = []string{"Land"}        // 30x Mountain

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = drive(m, deckLoadedMsg{info: deckInfo{name: "Tagged", total: 34, unique: 5}, cards: cards})
	return drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
}

func TestStatCountsMatchWhatFilteringGives(t *testing.T) {
	m := taggedDeckModel(t)
	entries := toEntries(m.baseItems)

	// Every row's number has to be exactly what you get when you filter to
	// it — that's the whole point of the rows carrying their own test.
	for _, row := range flatRows(statGroups(entries, entries)) {
		got := 0
		for _, e := range entries {
			if row.match(e) {
				got += e.qty
			}
		}
		if got != row.count {
			t.Errorf("%s/%s counts %d but filtering gives %d", row.group, row.label, row.count, got)
		}
	}
}

func TestStatNavigationFiltersTheList(t *testing.T) {
	m := taggedDeckModel(t)

	if m.statFilter != nil {
		t.Fatal("opening the statistics panel filtered the list on its own")
	}
	full := len(m.resultList.Items())

	// The first press enters the category list rather than stepping past it.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if m.statIndex != 0 || m.statFilter == nil {
		t.Fatalf("first J did not select the first category (index %d)", m.statIndex)
	}

	rows := flatRows(m.statPanel())
	for i, want := range rows {
		// Walk to row i.
		for m.statIndex < i {
			m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
		}
		if !want.same(m.statFilter) {
			t.Fatalf("at index %d the filter is %+v, want %s/%s", i, m.statFilter, want.group, want.label)
		}

		onScreen := toEntries(m.resultList.Items())
		total := 0
		for _, e := range onScreen {
			total += e.qty
		}

		// The panel now describes the cards the category left on screen,
		// while keeping every row in the place it started in.
		panel := flatRows(m.statPanel())
		if len(panel) != len(rows) {
			t.Fatalf("at %s/%s the panel has %d rows, want the original %d",
				want.group, want.label, len(panel), len(rows))
		}
		for j, got := range panel {
			if !got.same(&rows[j]) {
				t.Fatalf("at %s/%s row %d became %s/%s, want %s/%s",
					want.group, want.label, j, got.group, got.label, rows[j].group, rows[j].label)
			}
			expect := 0
			for _, e := range onScreen {
				if got.match(e) {
					expect += e.qty
				}
			}
			if got.count != expect {
				t.Errorf("filtered to %s/%s, row %s/%s reads %d but %d cards on screen match",
					want.group, want.label, got.group, got.label, got.count, expect)
			}
		}

		// The category you are on accounts for everything on screen.
		if panel[i].count != total {
			t.Errorf("%s/%s reads %d but the list holds %d cards",
				want.group, want.label, panel[i].count, total)
		}
	}

	// The categories hold their places while filtering, so there is always
	// something to move on to.
	if got := len(flatRows(m.statPanel())); got != len(rows) {
		t.Errorf("categories collapsed to %d while filtering, want %d", got, len(rows))
	}

	// Walking off either end stays put rather than wrapping.
	for i := 0; i < len(rows)+5; i++ {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	}
	if m.statIndex != 0 {
		t.Errorf("K past the top left index at %d", m.statIndex)
	}

	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.statFilter != nil || m.statIndex != -1 {
		t.Error("esc did not clear the category")
	}
	if len(m.resultList.Items()) != full {
		t.Errorf("clearing left %d cards, want all %d", len(m.resultList.Items()), full)
	}
}

func TestStatLineMatchesRender(t *testing.T) {
	m := taggedDeckModel(t)
	groups := m.statPanel()
	m.statFilter = &flatRows(groups)[0] // so the panel counts the whole deck

	for i := range flatRows(groups) {
		m.statIndex = i
		lines := strings.Split(stripANSI(m.renderStats(60)), "\n")

		got := -1
		for n, line := range lines {
			if strings.HasPrefix(strings.TrimLeft(line, " "), "▸") {
				got = n
				break
			}
		}
		if want := statLine(groups, i); got != want {
			t.Fatalf("row %d renders on line %d, statLine says %d — scrolling will be off", i, got, want)
		}
	}
}

func TestStatFilterSurvivesLeavingThePanel(t *testing.T) {
	m := taggedDeckModel(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	narrowed := len(m.resultList.Items())

	// Back to the card view: the filter stays, and the header says so.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.panel != panelCard {
		t.Fatal("s did not return to the card view")
	}
	if len(m.resultList.Items()) != narrowed {
		t.Error("leaving the statistics panel dropped the filter")
	}
	if !strings.Contains(stripANSI(m.resultsHeader()), m.statFilterLabel()) {
		t.Errorf("header does not name the active category: %s", stripANSI(m.resultsHeader()))
	}
}

func TestNewResultsForgetTheCategory(t *testing.T) {
	m := taggedDeckModel(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if m.statFilter == nil {
		t.Fatal("no category selected")
	}

	m = drive(m, searchResultMsg{cards: testCards(), totalCards: 3})
	if m.statFilter != nil || m.statIndex != -1 {
		t.Error("a new search kept the old category filter")
	}
	if len(m.resultList.Items()) != len(testCards()) {
		t.Errorf("search results were narrowed to %d", len(m.resultList.Items()))
	}
}

// ── esc and i ───────────────────────────────────────────────────

// quits reports whether a command asks the program to quit.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestEscPeelsOffOneLayerAtATime(t *testing.T) {
	m := taggedDeckModel(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}) // back to the card view

	// A typed filter goes first.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Mountain")})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.resultList.FilterState() == list.Unfiltered {
		t.Fatal("the typed filter never applied")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.resultList.FilterState() != list.Unfiltered {
		t.Error("esc did not clear the typed filter")
	}
	if quits(cmd) {
		t.Fatal("esc quit while a filter was still on")
	}

	// Then the category filter.
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.statFilter != nil {
		t.Error("esc did not clear the category filter")
	}
	if quits(cmd) {
		t.Fatal("esc quit while a category was still selected")
	}

	// Only then does it quit.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}); !quits(cmd) {
		t.Error("esc with nothing to clear did not quit")
	}

	// And it no longer walks back to the search bar — i does that.
	if m.searchFocused {
		t.Fatal("focus should still be on the list")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !next.(model).searchFocused {
		t.Error("i did not move focus to the search bar")
	}
}

func TestEmptiedCategoriesKeepTheirPlace(t *testing.T) {
	m := taggedDeckModel(t)
	before := flatRows(m.statPanel())

	// Land: the 30 Mountains and nothing else, so most other categories
	// are emptied by it.
	target := -1
	for i, r := range before {
		if r.group == "Type" && r.label == "Land" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("fixture has no Land row")
	}
	for i := 0; i <= target; i++ {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	}
	if m.statFilterLabel() != "Type: Land" {
		t.Fatalf("landed on %q", m.statFilterLabel())
	}

	after := flatRows(m.statPanel())
	zeros := 0
	for i, r := range after {
		if !r.same(&before[i]) {
			t.Fatalf("row %d moved from %s/%s to %s/%s", i,
				before[i].group, before[i].label, r.group, r.label)
		}
		if r.count == 0 {
			zeros++
		}
	}
	if zeros == 0 {
		t.Fatal("filtering to Land emptied nothing — the fixture is not exercising this")
	}

	// The emptied rows are still drawn, reading zero.
	panel := stripANSI(m.renderStats(60))
	if !strings.Contains(panel, "Creature") {
		t.Errorf("an emptied category vanished from the panel:\n%s", panel)
	}
	for _, line := range strings.Split(panel, "\n") {
		if strings.Contains(line, "Creature") && !strings.HasSuffix(strings.TrimRight(line, " "), "0") {
			t.Errorf("Creature should read zero under a Land filter: %q", line)
		}
	}

	// And the histograms describe the filtered cards, not the whole deck.
	for _, r := range after {
		if r.group == "Type" && r.label == "Land" && r.count != 31 {
			t.Errorf("Land reads %d, want 31 (30 Mountains + Ancient Tomb)", r.count)
		}
	}
}

// selectedBar is the length of the bar on the highlighted row.
func selectedBar(t *testing.T, m model) int {
	t.Helper()
	for _, line := range strings.Split(stripANSI(m.renderStats(60)), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "▸") {
			return strings.Count(line, "█")
		}
	}
	t.Fatal("no row is selected")
	return 0
}

// walkTo moves the statistics cursor onto a named category.
func walkTo(t *testing.T, m model, group, label string) model {
	t.Helper()
	rows := flatRows(m.statPanel())
	for i, r := range rows {
		if r.group == group && r.label == label {
			for j := 0; j <= i; j++ {
				m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
			}
			if m.statFilterLabel() != group+": "+label {
				t.Fatalf("walked to %q, want %s: %s", m.statFilterLabel(), group, label)
			}
			return m
		}
	}
	t.Fatalf("no %s/%s row", group, label)
	return m
}

func TestBarsScaleToTheUnfilteredSet(t *testing.T) {
	// The fixture's types are Creature 2, Planeswalker 1, Land 31.
	base := taggedDeckModel(t)

	land := selectedBar(t, walkTo(t, base, "Type", "Land"))
	creature := selectedBar(t, walkTo(t, base, "Type", "Creature"))

	// Land is the group's largest, so it fills the bar; a category a
	// fifteenth its size must not look the same.
	if creature >= land {
		t.Errorf("Creature (2 cards) drew %d blocks against Land's (31 cards) %d — "+
			"the scale is following the filtered set", creature, land)
	}
	if land < 10 {
		t.Errorf("the group's largest category drew only %d blocks", land)
	}

	// And two small categories stay in proportion to each other rather
	// than both filling the bar: CMC 5 has two cards, CMC 3 has one.
	cmc3 := selectedBar(t, walkTo(t, base, "CMC", "3"))
	cmc5 := selectedBar(t, walkTo(t, base, "CMC", "5"))
	if cmc5 <= cmc3 {
		t.Errorf("CMC 5 (2 cards) drew %d blocks and CMC 3 (1 card) drew %d", cmc5, cmc3)
	}
}

func TestStatsScrollLeavesTheSelectionAlone(t *testing.T) {
	// A short terminal, so the category list runs past the panel.
	cards := deckFixture()
	cards[0].tags = []string{"Ramp", "Own"}
	cards[1].tags = []string{"Ramp"}
	cards[3].tags = []string{"Land"}

	m := initialModel()
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 16})
	m = drive(m, deckLoadedMsg{info: deckInfo{name: "Tagged", total: 34}, cards: cards})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = walkTo(t, m, "Type", "Creature")

	was, wasFilter := m.statIndex, m.statFilterLabel()
	narrowed := len(m.resultList.Items())

	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.previewScroll == 0 {
		t.Fatal("ctrl+d did not scroll the statistics panel")
	}
	if m.statIndex != was || m.statFilterLabel() != wasFilter {
		t.Errorf("scrolling moved the selection from %d/%s to %d/%s",
			was, wasFilter, m.statIndex, m.statFilterLabel())
	}
	if len(m.resultList.Items()) != narrowed {
		t.Error("scrolling changed which cards are listed")
	}

	scrolled := m.previewScroll
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.previewScroll >= scrolled {
		t.Errorf("ctrl+u did not scroll back: %d then %d", scrolled, m.previewScroll)
	}
	if m.statIndex != was {
		t.Error("scrolling back moved the selection")
	}

	// Moving the selection again brings it back into view.
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	line := statLine(m.statPanel(), m.statIndex)
	height := m.resultsLayout().panelH - 1
	if line < m.previewScroll || line >= m.previewScroll+height {
		t.Errorf("selected row on line %d is outside the visible %d..%d",
			line, m.previewScroll, m.previewScroll+height-1)
	}
}
