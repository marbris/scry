package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/theme"
)

// A deck's versions.
//
// gv means "versions of the thing under the cursor" throughout: here, the
// commits that touched a deck; on a card, the wordings it has been printed
// with. Same question, two kinds of thing — which is why it is one key
// rather than two.

type versionList struct {
	cursor
	slug    string
	name    string
	all     []deck.Commit
	commits []deck.Commit
	filter  string
	// diffs are what each commit did, fetched as the cursor reaches it.
	// Local git, so this is fast enough not to need the debouncing a
	// network request does.
	diffs map[string]string
}

type versionsMsg struct {
	panel   int
	slug    string
	name    string
	all     []deck.Commit
	commits []deck.Commit
	filter  string
	err     error
}

// versionLimit is deep enough to find the change you half-remember making,
// and shallow enough that opening it is instant.
const versionLimit = 200

func loadVersions(panelID int, slug, name string) tea.Cmd {
	return func() tea.Msg {
		commits, err := deck.History(slug, versionLimit)
		return versionsMsg{panel: panelID, slug: slug, name: name, commits: commits, err: err}
	}
}

func (m Model) handleVersions(msg versionsMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	p.loading = false
	if msg.err != nil {
		p.err = msg.err
		return m, nil
	}

	v := &versionList{
		slug: msg.slug, name: msg.name, all: msg.commits, commits: msg.commits,
		diffs: map[string]string{},
	}
	p.push(v)
	p.title = v.title()
	return m, v.wantDiff()
}

func (l *versionList) title() string { return l.name + " · versions" }

func (l *versionList) subtitle() string {
	if len(l.commits) == 0 {
		return ""
	}
	return itoa(len(l.commits)) + " " + plural("version", len(l.commits))
}

func (l *versionList) lines(width, height int, focused bool, m *Model) []string {
	if len(l.commits) == 0 {
		return fillTo([]string{mutedLine("no versions recorded", width)}, width, height)
	}

	l.cursor.scrollInto(height, len(l.commits))
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.commits) && len(lines) < height; i++ {
		c := l.commits[i]
		under := focused && i == l.cursor.at

		when := c.When
		subject := fit(c.Subject, maxInt(width-textWidth(when)-1, 0))

		style := lipgloss.NewStyle().Foreground(theme.Text)
		if under {
			style = style.Foreground(theme.SelectionFg).Bold(true)
		}
		line := style.Render(subject) + " " +
			lipgloss.NewStyle().Foreground(theme.TextDim).Render(when)
		if under {
			line = lipgloss.NewStyle().Background(theme.SelectionBg).Width(width).Render(line)
		}
		lines = append(lines, line)
	}
	return fillTo(lines, width, height)
}

func (l *versionList) setFilter(s string) {
	l.filter = s
	l.commits = l.all
	if terms := filterTerms(s); len(terms) > 0 {
		kept := make([]deck.Commit, 0, len(l.all))
		for _, c := range l.all {
			hay := strings.ToLower(c.Subject + " " + c.When)
			keep := true
			for _, t := range terms {
				if !strings.Contains(hay, t) {
					keep = false
					break
				}
			}
			if keep {
				kept = append(kept, c)
			}
		}
		l.commits = kept
	}
	l.cursor.clamp(len(l.commits))
}

func (l *versionList) clear() bool {
	if l.filter != "" {
		l.setFilter("")
		return true
	}
	return false
}

func (l *versionList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if l.cursor.navKey(k, len(l.commits), m.pageStep()) {
		return true, l.wantDiff()
	}
	if k == "/" {
		p.openFilter(l.filter)
		return true, nil
	}
	return false, nil
}

// wantDiff fetches the diff for whatever the cursor has landed on, if it
// isn't already in hand.
func (l *versionList) wantDiff() tea.Cmd {
	if l.cursor.at >= len(l.commits) {
		return nil
	}
	c := l.commits[l.cursor.at]
	if _, have := l.diffs[c.Hash]; have {
		return nil
	}
	slug, hash := l.slug, c.Hash
	return func() tea.Msg {
		text, err := deck.Diff(slug, hash)
		if err != nil {
			text = "couldn't read this version: " + err.Error()
		}
		return diffMsg{slug: slug, hash: hash, diff: text}
	}
}

type diffMsg struct {
	slug string
	hash string
	diff string
}

func (m Model) handleDiff(msg diffMsg) (tea.Model, tea.Cmd) {
	for _, p := range m.ws.panels {
		if v, ok := p.top().(*versionList); ok && v.slug == msg.slug {
			v.diffs[msg.hash] = msg.diff
		}
	}
	return m, nil
}

// info is what the version under the cursor did to the deck. A commit
// subject says "+3 cards"; the diff says which three.
func (l *versionList) info(width int) []string {
	if l.cursor.at >= len(l.commits) {
		return nil
	}
	c := l.commits[l.cursor.at]

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	out := []string{
		head.Render(fit(c.Subject, width)),
		dim.Render(fit(c.Short+" · "+c.When, width)),
		"",
	}

	diff, have := l.diffs[c.Hash]
	if !have {
		return append(out, mutedLine("…", width))
	}
	return append(out, renderDiff(diff, width)...)
}

// renderDiff shows what changed and nothing else. Git's headers — the index
// line, the file names, the @@ markers — are noise here: there is one file,
// and you know which.
func renderDiff(diff string, width int) []string {
	add := lipgloss.NewStyle().Foreground(theme.DiffAdd)
	remove := lipgloss.NewStyle().Foreground(theme.DiffRemove)
	same := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var out []string
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
			strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "@@"):
			continue
		case strings.HasPrefix(line, "+"):
			out = append(out, add.Render(fit(line, width)))
		case strings.HasPrefix(line, "-"):
			out = append(out, remove.Render(fit(line, width)))
		case strings.TrimSpace(line) == "":
			continue
		default:
			out = append(out, same.Render(fit(line, width)))
		}
	}
	if len(out) == 0 {
		return []string{mutedLine("nothing changed in the deck itself", width)}
	}
	return out
}
