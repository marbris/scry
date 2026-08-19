package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"scry/internal/paths"
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
	return filepath.Join(paths.State(), queryHistoryFile)
}

// LoadQueryHistory reads the queries, oldest first. A missing or unreadable
// file just means no history — it's a convenience, not something to report.
func LoadQueryHistory() []string {
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

func SaveQueryHistory(h []string) error {
	body, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return os.WriteFile(queryHistoryPath(), body, 0644)
}

// RememberQuery adds a query to the history, most recent last. Running the
// same search twice doesn't fill the history with it: the earlier copy is
// removed rather than a second one added, so walking back never steps
// through the same query repeatedly.
func RememberQuery(h []string, query string) []string {
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

// historyIdle is where the walk sits when you aren't walking: the bar is
// showing what you typed rather than something recalled.
const historyIdle = -1

// recall moves through the history and returns what the bar should show.
// Walking back from the end keeps your draft, so pressing up out of
// curiosity doesn't cost you the query you were in the middle of writing.
func (p *panel) recall(delta int) {
	if len(p.history) == 0 {
		return
	}

	if p.historyAt == historyIdle {
		if delta > 0 {
			return // already at the end; there is nothing newer
		}
		p.draft = p.search.Value()
		p.historyAt = len(p.history)
	}

	at := p.historyAt + delta
	switch {
	case at < 0:
		at = 0
	case at >= len(p.history):
		// Off the newest end: back to whatever you were writing.
		p.historyAt = historyIdle
		p.search.SetValue(p.draft)
		p.search.CursorEnd()
		return
	}

	p.historyAt = at
	p.search.SetValue(p.history[at])
	p.search.CursorEnd()
}

// leaveHistory ends the walk, which typing does: what's in the bar is yours
// again, not a recalled query you're standing on.
func (p *panel) leaveHistory() {
	p.historyAt = historyIdle
	p.draft = ""
}
