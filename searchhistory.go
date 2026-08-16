package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The queries you've run, kept between sessions so the search bar behaves
// like a shell prompt: up walks back through them, down walks forward again,
// and what you'd half-typed before you started walking is still there when
// you get back to the end.

const (
	queryHistoryFile = "queries.json"

	// Enough to reach back through a session or two of searching. The file
	// is a few kilobytes at that size.
	queryHistoryMax = 200
)

func queryHistoryPath() string {
	return filepath.Join(dataDir(), queryHistoryFile)
}

// loadQueryHistory reads the queries, oldest first. A missing or unreadable
// file just means no history — it's a convenience, not something to report.
func loadQueryHistory() []string {
	body, err := os.ReadFile(queryHistoryPath())
	if err != nil {
		return nil
	}
	var out []string
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}

func saveQueryHistory(h []string) error {
	body, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return os.WriteFile(queryHistoryPath(), body, 0644)
}

// rememberQuery adds a query to the history, most recent last. Running the
// same search twice doesn't fill the history with it: the earlier copy is
// removed rather than a second one added, so walking back never steps
// through the same query repeatedly.
func rememberQuery(h []string, query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return h
	}

	out := make([]string, 0, len(h)+1)
	for _, q := range h {
		if q != query {
			out = append(out, q)
		}
	}
	out = append(out, query)

	if len(out) > queryHistoryMax {
		out = out[len(out)-queryHistoryMax:]
	}
	return out
}

// ── Walking it ──────────────────────────────────────────────────

// historyIdle is historyAt when you aren't walking the history — the search
// bar is showing whatever you typed rather than something recalled.
const historyIdle = -1

// recallQuery steps through the history and returns what the search bar
// should show. delta is -1 for the previous query and +1 for the next.
//
// Stepping forward past the newest query brings back the draft — whatever
// was in the bar when you started walking — so glancing at an old search
// doesn't cost you the one you were composing.
func (m model) recallQuery(delta int) model {
	if len(m.queryHistory) == 0 {
		return m
	}

	// Starting to walk: remember what's in the bar to come back to.
	if m.historyAt == historyIdle {
		if delta > 0 {
			return m // already at the end of it
		}
		m.historyDraft = m.searchInput.Value()
		m.historyAt = len(m.queryHistory)
	}

	at := m.historyAt + delta
	if at < 0 {
		at = 0
	}
	if at >= len(m.queryHistory) {
		// Back to what you were typing.
		m.historyAt = historyIdle
		m.searchInput.SetValue(m.historyDraft)
		m.searchInput.CursorEnd()
		return m
	}

	m.historyAt = at
	m.searchInput.SetValue(m.queryHistory[at])
	m.searchInput.CursorEnd()
	return m
}

// leaveHistory stops walking, so typing after a recall is editing that
// query rather than still being somewhere in the list.
func (m *model) leaveHistory() {
	m.historyAt = historyIdle
	m.historyDraft = ""
}
