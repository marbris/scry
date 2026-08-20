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

	var body []string
	offset := m.info.offset
	switch {
	case m.info.mode == infoStats:
		body = m.renderStats(inner)
		// The statistics scroll to wherever the highlighted category is,
		// rather than remembering a position: the category *is* the
		// position, so deriving it can't drift out of step with it. J past
		// the bottom used to move a cursor you could no longer see.
		offset = scrollTo(statLine(m.statGroups(), m.stats.row),
			offset, maxInt(height-4, 1), len(body))
	case m.info.mode == infoVersions:
		body = m.infoVersions(inner)
	default:
		body = m.infoBody(inner)
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
	lines := m.leaderBarLines()
	painted := make([]string, len(lines))
	for i, line := range lines {
		painted[i] = lipgloss.NewStyle().
			Background(theme.SurfaceAlt).Width(m.width).MaxWidth(m.width).
			Render(" " + line)
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
	if m.ws.count() > 1 {
		parts = append(parts, key.Render("1-9")+" "+what.Render("go to"))
	}
	return packStyled(parts, sep, maxInt(m.width-2, 1))
}

// footerHeight is how many rows the bottom of the screen needs. The leader
// menu is the only thing down there that can want more than one.
func (m Model) footerHeight() int {
	if m.leader {
		return maxInt(len(m.leaderBarLines()), 1)
	}
	if m.quitting {
		return 1
	}
	// The hint bar is the whole contextual keymap now, so it is as tall as
	// that keymap needs. The panels give up the room, the way they do for
	// the leader menu — being pushed off the top of the screen instead is a
	// bug this project has already had once.
	return maxInt(len(m.hintLines(maxInt(m.width-textWidth(m.viewStatus(m.ws.layout()))-6, 1))), 1)
}

// viewHint is the keys that work where you are, wrapped over as many lines
// as they need, with the last thing you did and the panel count kept out to
// the right of them.
func (m Model) viewHint(l layout) string {
	right := m.viewStatus(l)
	rightWidth := textWidth(right)

	// Every hint line is packed to what is left over, so the status can
	// never land on top of one. Two spaces of margin each side, and two
	// between the two columns.
	room := maxInt(m.width-rightWidth-6, 1)
	lines := m.hintLines(room)

	for i, line := range lines {
		pad := maxInt(m.width-textWidth(line)-rightWidth-2, 1)
		if i == 0 && right != "" {
			lines[i] = " " + line + strings.Repeat(" ", pad) + right + " "
			continue
		}
		lines[i] = " " + line
	}
	return strings.Join(lines, "\n")
}

// hintLines is the contextual keymap, packed into lines of the given width.
func (m Model) hintLines(width int) []string {
	key := lipgloss.NewStyle().Foreground(theme.Accent)
	what := lipgloss.NewStyle().Foreground(theme.TextMuted)
	sep := what.Render(" · ")

	var parts []string
	for _, r := range m.contextKeys() {
		parts = append(parts, key.Render(r[0])+" "+what.Render(r[1]))
	}
	lines := packStyled(parts, sep, width)
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// viewStatus is the right-hand column: what the last thing you did produced,
// then which panel of how many and whether any are off screen.
func (m Model) viewStatus(l layout) string {
	var parts []string
	if m.notice != "" {
		style := lipgloss.NewStyle().Foreground(theme.Success)
		if strings.HasPrefix(m.notice, "error:") {
			style = lipgloss.NewStyle().Foreground(theme.Error)
		}
		// Half the screen at most. The keys wrap onto another line rather
		// than being crowded out, so the notice can afford to be readable —
		// and a notice cut short is one you have to guess at, which is what
		// "that deck isn't yours — p nee…" reads like.
		parts = append(parts, style.Render(truncate(m.notice, maxInt(m.width/2, 24))))
	}
	if n := m.ws.count(); n > 0 {
		count := itoa(m.ws.focused+1) + "/" + itoa(n)
		if l.visible() < n {
			count += " ↔"
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(theme.Accent).Render(count))
	}
	return strings.Join(parts, "  ")
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
