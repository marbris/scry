package main

// Printed-text history.
//
// Scryfall only ever serves a card's *current* oracle text — its
// printed_text field is populated for non-English cards only, and there is
// no oracle revision history in the API at all. MTGJSON does carry the
// wording as printed, per printing, in its originalText field, so that's
// where this comes from.
//
// MTGJSON only publishes whole sets, so a heavily reprinted card means one
// file per set it appeared in. Each set is distilled down to a name -> text
// map and cached on disk, and nothing is fetched until the user asks for it.

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// mtgjsonSem keeps set downloads to a civilised number at a time.
var mtgjsonSem = make(chan struct{}, 4)

type printing struct {
	Name     string
	Set      string // upper-case, as MTGJSON names them
	SetName  string
	Released string // YYYY-MM-DD
}

// TextRevision is one distinct wording, and the printing it first appeared on.
type TextRevision struct {
	Text      string
	SetCode   string
	SetName   string
	Released  string
	Printings int  // how many printings shared this wording
	Current   bool // matches the card's current oracle text
}

type histState int

const (
	histPrintings histState = iota // looking up where the card was printed
	histNeedSets                   // waiting for the go-ahead to download sets
	histFetching                   // downloading set data
	histReady
	histFailed
)

type cardHistory struct {
	state     histState
	printings []printing
	sets      []string // every set the card was printed in
	missing   []string // the ones not cached yet
	revisions []TextRevision
	err       error
}

// ── Messages ────────────────────────────────────────────────────

type printingsMsg struct {
	oracleID  string
	printings []printing
	err       error
}

type setOriginalsMsg struct {
	set   string
	cards map[string]string
	err   error
}

// ── Commands ────────────────────────────────────────────────────

// fetchPrintings lists every paper English printing of a card, oldest first.
func fetchPrintings(oracleID, uri string) tea.Cmd {
	return func() tea.Msg {
		if uri == "" {
			return printingsMsg{oracleID: oracleID, err: fmt.Errorf("no printings link for this card")}
		}

		var out []printing
		seen := make(map[string]bool)

		for page, next := 0, uri; next != "" && page < 6; page++ {
			body, err := doGet(next)
			if err != nil {
				if _, ok := err.(notFoundError); ok {
					break
				}
				return printingsMsg{oracleID: oracleID, err: err}
			}

			var sr ScryfallResponse
			if err := json.Unmarshal(body, &sr); err != nil {
				return printingsMsg{oracleID: oracleID, err: err}
			}

			for _, c := range sr.Data {
				// Digital-only and translated printings aren't part of the
				// paper wording history.
				if c.Digital || (c.Lang != "" && c.Lang != "en") || c.Set == "" {
					continue
				}
				set := strings.ToUpper(c.Set)
				if seen[set] {
					continue // one printing per set is enough to read its text
				}
				seen[set] = true
				out = append(out, printing{
					Name:     c.Name,
					Set:      set,
					SetName:  c.SetName,
					Released: c.ReleasedAt,
				})
			}

			if !sr.HasMore {
				break
			}
			next = sr.NextPage
		}

		sort.SliceStable(out, func(i, j int) bool { return out[i].Released < out[j].Released })
		return printingsMsg{oracleID: oracleID, printings: out}
	}
}

// fetchSetOriginals pulls one set's printed text, from disk if it's already
// been distilled, otherwise from MTGJSON.
func fetchSetOriginals(set string) tea.Cmd {
	return func() tea.Msg {
		if cards, ok := readSetCache(set); ok {
			return setOriginalsMsg{set: set, cards: cards}
		}

		mtgjsonSem <- struct{}{}
		defer func() { <-mtgjsonSem }()

		// Another card may have fetched it while we waited for a slot.
		if cards, ok := readSetCache(set); ok {
			return setOriginalsMsg{set: set, cards: cards}
		}

		cards, err := downloadSetOriginals(set)
		if err != nil {
			return setOriginalsMsg{set: set, err: err}
		}
		writeSetCache(set, cards)
		return setOriginalsMsg{set: set, cards: cards}
	}
}

func downloadSetOriginals(set string) (map[string]string, error) {
	u := fmt.Sprintf("https://mtgjson.com/api/v5/%s.json.gz", set)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "scry/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Sets Scryfall knows and MTGJSON doesn't are cached as empty so we
	// don't ask again.
	if resp.StatusCode == 404 {
		return map[string]string{}, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("mtgjson (%d) for %s", resp.StatusCode, set)
	}

	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var payload struct {
		Data struct {
			Cards []struct {
				Name         string `json:"name"`
				OriginalText string `json:"originalText"`
			} `json:"cards"`
		} `json:"data"`
	}
	if err := json.NewDecoder(zr).Decode(&payload); err != nil {
		return nil, err
	}

	cards := make(map[string]string)
	for _, c := range payload.Data.Cards {
		if c.OriginalText == "" {
			continue
		}
		key := strings.ToLower(c.Name)
		if _, dup := cards[key]; !dup {
			cards[key] = c.OriginalText
		}
	}
	return cards, nil
}

// ── Disk cache ──────────────────────────────────────────────────

func originalsDir() string {
	dir := filepath.Join(dataDir(), "originals")
	os.MkdirAll(dir, 0755)
	return dir
}

func setCachePath(set string) string {
	return filepath.Join(originalsDir(), set+".json")
}

func readSetCache(set string) (map[string]string, bool) {
	body, err := os.ReadFile(setCachePath(set))
	if err != nil {
		return nil, false
	}
	var cards map[string]string
	if err := json.Unmarshal(body, &cards); err != nil {
		return nil, false
	}
	return cards, true
}

func writeSetCache(set string, cards map[string]string) {
	body, err := json.Marshal(cards)
	if err != nil {
		return
	}
	os.WriteFile(setCachePath(set), body, 0644)
}

func setIsCached(set string) bool {
	_, err := os.Stat(setCachePath(set))
	return err == nil
}

// ── Building the history ────────────────────────────────────────

// buildRevisions collapses the printings into one entry per distinct
// wording — a card reprinted twenty times with the same text is one entry.
func buildRevisions(c ScryfallCard, printings []printing, originals map[string]map[string]string) []TextRevision {
	var revs []TextRevision

	for _, p := range printings {
		text := cleanOriginal(lookupOriginal(originals[p.Set], p.Name))
		if text == "" {
			continue
		}
		if n := len(revs); n > 0 && sameText(revs[n-1].Text, text) {
			revs[n-1].Printings++
			continue
		}
		revs = append(revs, TextRevision{
			Text:      text,
			SetCode:   p.Set,
			SetName:   p.SetName,
			Released:  p.Released,
			Printings: 1,
		})
	}

	// The current oracle wording is the last revision, whether or not any
	// printing carries it verbatim.
	current := c.CombinedOracle()
	if n := len(revs); n > 0 && sameText(revs[n-1].Text, current) {
		revs[n-1].Current = true
	} else if current != "" {
		revs = append(revs, TextRevision{Text: current, Current: true})
	}

	return revs
}

// cleanOriginal converts MTGJSON's Gatherer-derived encoding into the same
// shape as oracle text. "ocT" is how the old tap symbol comes through, and
// "//" stands in for a line break. Normalising before comparing keeps a
// mere change of encoding from looking like a change of wording.
func cleanOriginal(s string) string {
	s = strings.ReplaceAll(s, "ocT", "{T}")
	s = originalBreakRe.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}

// "//" stands in for a line break, with or without spaces around it.
var originalBreakRe = regexp.MustCompile(`[ \t]*//[ \t]*`)

func lookupOriginal(cards map[string]string, name string) string {
	if cards == nil {
		return ""
	}
	if text, ok := cards[strings.ToLower(name)]; ok {
		return text
	}
	// Split and double-faced cards are sometimes filed under one face.
	if i := strings.Index(name, " // "); i > 0 {
		if text, ok := cards[strings.ToLower(name[:i])]; ok {
			return text
		}
	}
	return ""
}

// sameText compares wording while ignoring how it was laid out.
func sameText(a, b string) bool {
	return normalizeText(a) == normalizeText(b)
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "//", "\n")
	return strings.Join(strings.Fields(s), " ")
}

// ── Model wiring ────────────────────────────────────────────────

// toggleHistory opens the printed-text panel. Because a heavily reprinted
// card can mean a lot of set downloads, this only ever fetches when the
// user presses the key — never as a side effect of moving the cursor.
func (m model) toggleHistory() (tea.Model, tea.Cmd) {
	card, ok := m.active().selected()
	if !ok {
		return m, nil
	}
	h := m.histories[card.card.OracleID]

	// Already showing a finished history: the key toggles back to the card.
	if m.panel == panelHistory && h != nil && (h.state == histReady || h.state == histFailed) {
		m.panel = panelCard
		return m, nil
	}

	m.panel = panelHistory
	m.historyScroll = 0

	switch {
	case card.card.OracleID == "":
		return m, nil

	case h == nil:
		// First look at this card: find out where it was printed.
		m.histories[card.card.OracleID] = &cardHistory{state: histPrintings}
		return m, fetchPrintings(card.card.OracleID, card.card.PrintsSearchURI)

	case h.state == histNeedSets:
		// The user has seen the download count and pressed the key again.
		// Every set still needs asking for, not just the uncached ones —
		// fetchSetOriginals serves the cached ones straight off disk.
		h.state = histFetching
		cmds := make([]tea.Cmd, 0, len(h.sets))
		for _, set := range h.sets {
			if _, inMemory := m.originals[set]; !inMemory {
				cmds = append(cmds, fetchSetOriginals(set))
			}
		}
		if len(cmds) == 0 {
			m.finishHistory(card.card.OracleID)
			return m, nil
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m model) handlePrintings(msg printingsMsg) (tea.Model, tea.Cmd) {
	h := m.histories[msg.oracleID]
	if h == nil {
		return m, nil
	}

	if msg.err != nil {
		h.state, h.err = histFailed, msg.err
		return m, nil
	}

	h.printings = msg.printings
	h.sets = h.sets[:0]
	h.missing = h.missing[:0]
	for _, p := range msg.printings {
		h.sets = append(h.sets, p.Set)
		if _, inMemory := m.originals[p.Set]; !inMemory && !setIsCached(p.Set) {
			h.missing = append(h.missing, p.Set)
		}
	}

	// Everything is on disk already, so there's nothing to ask about.
	if len(h.missing) == 0 {
		cmds := make([]tea.Cmd, 0, len(h.sets))
		for _, set := range h.sets {
			if _, inMemory := m.originals[set]; !inMemory {
				cmds = append(cmds, fetchSetOriginals(set))
			}
		}
		if len(cmds) == 0 {
			m.finishHistory(msg.oracleID)
			return m, nil
		}
		h.state = histFetching
		return m, tea.Batch(cmds...)
	}

	h.state = histNeedSets
	return m, nil
}

func (m model) handleSetOriginals(msg setOriginalsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Remember the failure as an empty set so one bad set doesn't
		// wedge the whole history.
		m.originals[msg.set] = map[string]string{}
	} else {
		m.originals[msg.set] = msg.cards
	}

	// Any card waiting on this set may now be complete.
	for oracleID, h := range m.histories {
		if h.state != histFetching {
			continue
		}
		ready := true
		for _, set := range h.sets {
			if _, ok := m.originals[set]; !ok {
				ready = false
				break
			}
		}
		if ready {
			m.finishHistory(oracleID)
		}
	}
	return m, nil
}

// finishHistory assembles the revisions once every set is in.
func (m model) finishHistory(oracleID string) {
	h := m.histories[oracleID]
	if h == nil {
		return
	}
	card := m.cardByOracleID(oracleID)
	h.revisions = buildRevisions(card, h.printings, m.originals)
	h.state = histReady
}

func (m model) cardByOracleID(oracleID string) ScryfallCard {
	for _, c := range m.cards {
		if c.OracleID == oracleID {
			return c
		}
	}
	return ScryfallCard{}
}

// pendingSets counts the sets a history is still waiting on.
func (m model) pendingSets(h *cardHistory) int {
	n := 0
	for _, set := range h.sets {
		if _, ok := m.originals[set]; !ok {
			n++
		}
	}
	return n
}

// ── Panel ───────────────────────────────────────────────────────

func (m model) renderTextHistory(c ScryfallCard, width int) string {
	if width < 20 {
		width = 20
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvOrange)
	setStyle := lipgloss.NewStyle().Bold(true).Foreground(gruvAqua)
	dimStyle := lipgloss.NewStyle().Foreground(gruvGray)
	warnStyle := lipgloss.NewStyle().Foreground(gruvYellow)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Text History") + " " +
		dimStyle.Render("· "+truncate(c.Name, width-16)) + "\n\n")

	if c.Name == "" {
		return b.String() + dimStyle.Render("No card selected")
	}
	if c.OracleID == "" {
		return b.String() + dimStyle.Render("This card has no oracle id to trace.")
	}

	h := m.histories[c.OracleID]
	if h == nil {
		return b.String() + dimStyle.Render("t: load printed text history")
	}

	switch h.state {
	case histPrintings:
		return b.String() + dimStyle.Render("Looking up printings…")

	case histNeedSets:
		b.WriteString(dimStyle.Render(fmt.Sprintf(
			"%d printings across %d sets.", len(h.printings), len(h.sets))) + "\n\n")
		b.WriteString(warnStyle.Width(width).Render(fmt.Sprintf(
			"%d sets aren't cached yet. MTGJSON only publishes whole sets, so reading this card's old wording means downloading them.",
			len(h.missing))) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(gruvOrange).Render("t: download and show"))
		return b.String()

	case histFetching:
		done := len(h.sets) - m.pendingSets(h)
		return b.String() + dimStyle.Render(fmt.Sprintf("Fetching set data… %d/%d", done, len(h.sets)))

	case histFailed:
		return b.String() + lipgloss.NewStyle().Foreground(gruvRed).
			Render(fmt.Sprintf("unavailable: %v", h.err))
	}

	if len(h.revisions) == 0 {
		return b.String() + dimStyle.Render("No printed text on record for this card.")
	}

	wordings := "1 wording"
	if len(h.revisions) > 1 {
		wordings = fmt.Sprintf("%d wordings", len(h.revisions))
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("%s · %d printings",
		wordings, len(h.printings))) + "\n\n")

	for i, rev := range h.revisions {
		if i > 0 {
			b.WriteString("\n")
		}

		switch {
		case rev.SetName == "":
			b.WriteString(setStyle.Render("Current oracle text") + "\n")
		default:
			head := setStyle.Render(truncate(rev.SetName, width-14))
			if rev.Released != "" {
				head += dimStyle.Render(" · " + year(rev.Released))
			}
			if rev.Current {
				head += lipgloss.NewStyle().Foreground(gruvGreen).Render(" → current")
			}
			b.WriteString(head + "\n")
		}

		b.WriteString(highlightRuleText(rev.Text, width, m.rules) + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func year(released string) string {
	if len(released) >= 4 {
		return released[:4]
	}
	return released
}
