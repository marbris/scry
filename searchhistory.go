package main

// The pure half of this — reading, writing and adding to the file — moved to
// internal/ui, where the search bar that uses it now lives. What is left is
// the old model's way of walking it.

// The queries you've run, kept between sessions so the search bar behaves
// like a shell prompt: up walks back through them, down walks forward again,
// and what you'd half-typed before you started walking is still there when
// you get back to the end.

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
