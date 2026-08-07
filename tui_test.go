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
	m := resultsModel(t, 160, 40, data)

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
