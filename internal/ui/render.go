package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/theme"
)

// Drawing the workspace: the row of panels, the information panel beside
// them, and the line along the bottom that says what the keys do here.

// viewWorkspace lays the whole frame out.
func (m Model) viewWorkspace() string {
	ws := m.ws // a copy: layout records the scroll position, and View is a
	l := ws.layoutWithFooter(m.footerHeight())

	var columns []string
	for i, width := range l.panels {
		at := l.first + i
		columns = append(columns, m.viewPanel(ws.panels[at], at, width, l.height))
	}
	if l.info > 0 {
		columns = append(columns, m.viewInfo(l.info, l.height))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	return lipgloss.JoinVertical(lipgloss.Left, row, m.viewFooter(l))
}

// viewPanel draws one panel: a border, its header, and its contents.
func (m Model) viewPanel(p *panel, index, width, height int) string {
	focused := index == m.ws.focused
	editing := index == m.ws.editing

	// The border says which panel has the keys, and which one a/x/t are
	// going to write to — the two things you need to know without looking.
	colour := theme.Border
	switch {
	case focused:
		colour = theme.BorderFocus
	case editing:
		colour = theme.BorderEditing
	}

	inner := maxInt(width-2, 1)
	p.restyle()

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	if !focused {
		head = lipgloss.NewStyle().Foreground(theme.TextDim)
	}

	text, styled := p.header(inner)
	headLine := text
	if !styled {
		headLine = head.Render(fit(text, inner))
	}
	lines := []string{headLine}
	if sub := p.subtitleWithState(); sub != "" {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(theme.TextMuted).Render(fit(sub, inner)))
	}
	lines = append(lines, lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", inner)))
	lines = append(lines, m.viewPanelBody(p, inner, maxInt(height-2-len(lines), 1))...)

	body := lipgloss.NewStyle().
		Width(inner).
		Height(maxInt(height-2, 1)).
		MaxWidth(inner).
		Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colour).
		Render(body)
}

// viewPanelBody is what a panel holds: its cards, or — before anything has
// filled it — what it's waiting for.
func (m Model) viewPanelBody(p *panel, width, height int) []string {
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	if p.loading {
		return fillTo([]string{dim.Render(fit("searching…", width))}, width, height)
	}
	if p.err != nil {
		return fillTo([]string{
			lipgloss.NewStyle().Foreground(theme.Error).Render(fit(errorText(p.err), width)),
		}, width, height)
	}

	if v := p.top(); v != nil {
		focused := m.ws.panels[m.ws.focused] == p
		return v.lines(width, height, focused, &m)
	}

	return fillTo([]string{
		dim.Render(fit("nothing here yet", width)),
		"",
		dim.Render(fit("type a "+p.kind.prompt(), width)),
	}, width, height)
}

// fillTo pads a block out to the height it has to occupy, so the panel below
// it doesn't collapse around short content.
func fillTo(lines []string, width, height int) []string {
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// membersFor is what a panel should flag as living somewhere else too.
//
// The relation runs one way and out from the editing deck: every list marks
// the cards that are already in the deck you're building, and the deck marks
// the cards that any list on screen has turned up. With five panels open,
// "in my deck" is the only comparison that means the same thing in all of
// them.
func (m Model) membersFor(l *cardList) map[string]bool {
	editing := m.ws.editingList()
	if editing == nil {
		return nil
	}

	if l == editing {
		// The deck itself: flag what the other panels are showing.
		out := map[string]bool{}
		for _, other := range m.ws.panels {
			l := other.cardsView()
			if l == nil || l == editing {
				continue
			}
			for name := range l.names() {
				out[name] = true
			}
		}
		return out
	}
	return editing.names()
}

// viewInfo draws the information panel.
func (m Model) viewInfo(width, height int) string {
	inner := maxInt(width-2, 1)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fit(m.infoTitle(), inner))

	lines := []string{
		title,
		lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", inner)),
	}

	body := m.infoContent(inner)
	offset := m.info.offset
	if m.info.mode == infoStats {
		// The statistics scroll to wherever the highlighted category is,
		// rather than remembering a position: the category *is* the
		// position, so deriving it can't drift out of step with it. J past
		// the bottom used to move a cursor you could no longer see.
		offset = scrollTo(statLine(m.statGroups(), m.stats.row),
			offset, maxInt(height-4, 1), len(body))
	}
	// Scrolled with ctrl+j and ctrl+k, from wherever you are — the panel is
	// read, never focused.
	room := maxInt(height-2-len(lines), 1)
	if offset > maxInt(len(body)-room, 0) {
		offset = maxInt(len(body)-room, 0)
	}
	if offset < len(body) {
		body = body[offset:]
	} else {
		body = nil
	}
	lines = append(lines, body...)

	if len(body) == 0 {
		lines = append(lines, dim.Render(fit("nothing highlighted", inner)))
	}
	for len(lines) < height-2 {
		lines = append(lines, strings.Repeat(" ", inner))
	}
	if len(lines) > height-2 {
		lines = lines[:maxInt(height-2, 1)]
	}

	block := lipgloss.NewStyle().Width(inner).Height(maxInt(height-2, 1)).
		MaxWidth(inner).Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Render(block)
}

// infoTitle names what the information panel is describing. The mode when
// there is one, and otherwise whatever is under the cursor — a panel headed
// "card" while showing a git diff is a small lie told constantly.
func (m Model) infoTitle() string {
	switch m.info.mode {
	case infoStats:
		if m.stats.global {
			return "statistics · everything"
		}
		return "statistics"
	case infoVersions:
		return "printed text"
	}

	p := m.ws.current()
	if p == nil {
		return "card"
	}
	switch p.top().(type) {
	case *deckList:
		return "deck"
	case *rulesView:
		return "rule"
	case *versionList:
		return "version"
	case *userDeckList:
		return "deck"
	}
	return "card"
}

// infoContent is the whole information-panel body for the current mode,
// before it is scrolled — the lines paragraph scrolling counts and the view
// draws from the same place, so they can't disagree about where a paragraph
// begins.
func (m Model) infoContent(inner int) []string {
	switch m.info.mode {
	case infoStats:
		return m.renderStats(inner)
	case infoVersions:
		return m.infoVersions(inner)
	default:
		return m.infoBody(inner)
	}
}

// scrollInfoParagraph moves the information panel by a paragraph rather than
// a line — the same idea as ctrl+k / ctrl+j jumping a whole group in
// statistics, one level coarser than K and J. A card's rulings run to
// several paragraphs, and a line at a time is a lot of pressing to walk them.
func (m *Model) scrollInfoParagraph(delta int) {
	l := m.ws.layout()
	if l.info == 0 {
		return // no information panel on a terminal this narrow
	}
	starts := paragraphStarts(m.infoContent(maxInt(l.info-2, 1)))
	if len(starts) == 0 {
		return
	}

	cur := m.info.offset
	switch {
	case delta > 0:
		for _, s := range starts {
			if s > cur {
				m.info.offset = s
				return
			}
		}
		m.info.offset = starts[len(starts)-1]
	default:
		for i := len(starts) - 1; i >= 0; i-- {
			if starts[i] < cur {
				m.info.offset = starts[i]
				return
			}
		}
		m.info.offset = 0
	}
}

// paragraphStarts is the line index each paragraph begins on: a non-blank
// line at the top, or one following a blank line. Blank lines are the seams
// the card panel puts between a card's type, its text, its printing and each
// of its rulings.
func paragraphStarts(body []string) []int {
	var out []int
	prevBlank := true
	for i, line := range body {
		blank := strings.TrimSpace(stripStyles(line)) == ""
		if !blank && prevBlank {
			out = append(out, i)
		}
		prevBlank = blank
	}
	return out
}

// scrollTo brings a line into view with as little movement as possible.
func scrollTo(line, offset, height, total int) int {
	if line < offset {
		offset = line
	}
	if line >= offset+height {
		offset = line - height + 1
	}
	if max := total - height; offset > max {
		offset = maxInt(max, 0)
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// infoBody is what the focused panel has to say about its highlighted row.
func (m Model) infoBody(width int) []string {
	p := m.ws.current()
	if p == nil {
		return nil
	}
	v := p.top()
	if v == nil {
		return nil
	}
	return v.info(width)
}

// infoVersions is gv: a card's printed wordings, or — when what is
// highlighted isn't a card — whatever the view has to say.
func (m Model) infoVersions(width int) []string {
	if c := m.focusedCardValue(); c != nil {
		return m.renderHistory(*c, width)
	}
	return m.infoBody(width)
}

// ── The bottom line ─────────────────────────────────────────────

// viewFooter is the leader menu while the leader is waiting, and otherwise
// the keys that apply where you are.
func (m Model) viewFooter(l layout) string {
	switch {
	case m.quitting:
		return m.viewQuitQuestion()
	case m.leader:
		return m.viewLeaderBar()
	}
	// The notice used to replace this line rather than sit beside it, so
	// the result of what you had just done stood on top of the keys for
	// what to do next — and nothing cleared it, so it stood there for good.
	return m.viewHint(l)
}

// viewQuitQuestion is what stands between an unsaved deck and losing it.
func (m Model) viewQuitQuestion() string {
	decks := m.dirtyDecks()
	what := decks[0]
	if len(decks) > 1 {
		what = itoa(len(decks)) + " decks have"
	} else {
		what += " has"
	}

	key := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	text := lipgloss.NewStyle().Foreground(theme.Text)
	return lipgloss.NewStyle().
		Background(theme.SurfaceAlt).Width(m.width).MaxWidth(m.width).
		Render(" " + text.Render(what+" unsaved edits — ") +
			key.Render("w") + text.Render(" save and quit · ") +
			key.Render("y") + text.Render(" quit anyway · any other key stays"))
}

// viewLeaderBar is the menu the leader raises, so it never has to be
// memorised. Being able to see the menu is what makes a two-key binding
// cheaper in practice than a one-key chord you can't remember.
func (m Model) viewLeaderBar() string {
	// No background fill: a band of colour across the bottom contrasts with
	// an otherwise semi-transparent terminal, where nothing else here paints
	// one. The menu is just text, indented a space like the hint bar.
	lines := m.leaderBarLines()
	painted := make([]string, len(lines))
	for i, line := range lines {
		painted[i] = " " + line
	}
	return strings.Join(painted, "\n")
}

// leaderBarLines is the menu, wrapped onto as many lines as it needs.
//
// It used to be cut off at the width, which is the wrong thing to do to a
// menu: the entries you can't see are exactly the ones you opened it to
// read, and the cut lands mid-entry where it looks like a rendering fault.
// The layout asks how tall this is, so growing it takes room from the panels
// rather than pushing them off the screen.
func (m Model) leaderBarLines() []string {
	key := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	what := lipgloss.NewStyle().Foreground(theme.Text)
	sep := lipgloss.NewStyle().Foreground(theme.TextMuted).Render(" · ")

	var parts []string
	for _, c := range leaderMenu {
		parts = append(parts, key.Render(c.key)+" "+what.Render(c.what))
	}
	return packStyled(parts, sep, maxInt(m.width-2, 1))
}

// footerHeight is how many rows the bottom of the screen needs. The leader
// menu and the grouped hint bar can each want more than one.
func (m Model) footerHeight() int {
	if m.leader {
		return maxInt(len(m.leaderBarLines()), 1)
	}
	if m.quitting {
		return 1
	}
	// The hint bar is the whole contextual keymap now — one row per group,
	// with a line for the last result above them when there is one. The
	// panels give up the room, the way they do for the leader menu; being
	// pushed off the top of the screen instead is a bug this project has
	// already had once.
	return maxInt(len(m.footerLines()), 1)
}

// viewHint is the keys that work where you are, one grouped row each.
func (m Model) viewHint(l layout) string {
	return strings.Join(m.footerLines(), "\n")
}

// footerLines is the bottom of the screen: the last thing you did on its own
// line, then the grouped keys — one row per group, led by what the group is.
//
// The notice used to share the keys' first line, off to the right, and the
// groups wrapped over as many lines as they liked; a result worth reading and
// the keys for what to do next fought over the same row, and the bar grew
// tall. The notice is on its own line above them now, and each group is a
// single row across the full width.
func (m Model) footerLines() []string {
	hints := m.hintGroupLines(m.footerGroups(), maxInt(m.width-2, 1))
	for i := range hints {
		hints[i] = " " + hints[i]
	}

	var lines []string
	if notice := m.viewNotice(); notice != "" {
		lines = append(lines, " "+notice)
	}
	return append(lines, hints...)
}

// footerGroups is what the hint bar shows: at rest, the three keys that reach
// everything else; pressing ? grows it to the whole contextual keymap.
//
// A focused search bar is the exception — it shows its own small keymap and ?
// is a character there, so there is nothing to collapse or grow.
func (m Model) footerGroups() []hintGroup {
	p := m.ws.current()
	if p != nil && p.searchOpen && p.search.Focused() {
		return m.hintGroups()
	}
	if m.hintsExpanded {
		return m.hintGroups()
	}
	return []hintGroup{{"", [][2]string{
		{"space", "menu"},
		{"?", "keys"},
		{"q", "quit"},
	}}}
}

// hintGroupLines renders grouped keys, one row per group, each led by its
// title: "navigation: …", "select: …". A group is kept to a single row — keys
// that would overrun the width are dropped rather than wrapped, since the full
// set is one ? away. The labels are terse for the same reason: the bar is a
// reminder, not the manual.
func (m Model) hintGroupLines(groups []hintGroup, width int) []string {
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	key := lipgloss.NewStyle().Foreground(theme.Accent)
	what := lipgloss.NewStyle().Foreground(theme.TextMuted)
	sep := what.Render("  ")
	sepW := textWidth("  ")

	var lines []string
	for _, g := range groups {
		var b strings.Builder
		used := 0
		if g.title != "" {
			b.WriteString(head.Render(g.title + ":"))
			used = textWidth(g.title) + 1 // the colon
		}

		for _, r := range g.keys {
			part := key.Render(r[0]) + " " + what.Render(r[1])
			w := textWidth(r[0]) + 1 + textWidth(r[1])
			if used+sepW+w > width {
				break // one row only; the rest is in the reference
			}
			if used > 0 {
				b.WriteString(sep)
				used += sepW
			}
			b.WriteString(part)
			used += w
		}
		lines = append(lines, b.String())
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// viewNotice is the last thing you did, on its own line above the keys.
func (m Model) viewNotice() string {
	if m.notice == "" {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(theme.Success)
	if strings.HasPrefix(m.notice, "error:") {
		style = lipgloss.NewStyle().Foreground(theme.Error)
	}
	// A line of its own, so it can afford to be readable — a notice cut
	// short is one you have to guess at, which is what "that deck isn't
	// yours — p nee…" reads like.
	return style.Render(truncate(m.notice, maxInt(m.width-2, 24)))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// mutedLine is a full-width line of dim text, which every view uses to say
// it has nothing to show.
func mutedLine(s string, width int) string {
	return lipgloss.NewStyle().Foreground(theme.TextMuted).Render(fit(s, width))
}

// wrapStyled wraps text and paints each line, for the information panel.
func wrapStyled(s string, width int, style lipgloss.Style) []string {
	lines := wrap(s, width)
	for i, line := range lines {
		lines[i] = style.Render(fit(line, width))
	}
	return lines
}
