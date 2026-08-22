package ui

import (
	"path"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/moxfield"
	"scry/internal/theme"
)

// The decks panel: everything you can open, in one list.
//
// Three kinds of thing live here and the design is deliberate about keeping
// them together rather than in three panes. They answer the same question —
// what can I look at? — and the differences between them are one letter
// wide:
//
//	L  a local deck, a file you can edit
//	R  a remote deck on Moxfield, which you can take a copy of
//	U  somebody whose decks you can look through
//
// The row is name, kind, legality, size and age. In a narrow panel the tail
// is given up from the right, because a name with no age beside it is still
// a name, and an age with no name is nothing.

type entryKind int

const (
	entryLocal entryKind = iota
	entryRemote
	entryUser
	entryFolder
)

func (k entryKind) letter() string {
	switch k {
	case entryRemote:
		return "R"
	case entryUser:
		return "U"
	}
	return "L"
}

// moxFolder is the virtual folder the followed remotes and people live under.
// It isn't a directory on disk — it's built from the bookmarks each refresh —
// so the tree can show Moxfield as one branch beside your own folders.
const moxFolder = "moxfield"

// deckEntry is one row.
type deckEntry struct {
	kind entryKind
	name string
	// slug identifies a local deck; id identifies a remote one; user is the
	// Moxfield name. Exactly one is set.
	slug string
	id   string
	user string

	format string
	count  int // cards for a deck, decks for a user or a folder
	// colours is the deck's colour identity, in WUBRG order. Empty for a
	// remote, whose cards we haven't looked at, and for a person.
	colours  []string
	modified time.Time
	// legal is the verdict, which stays unknown until the background pass
	// gets to this deck.
	legal  deck.Legality
	broken bool

	// depth is how deep in the folder tree this row sits, for indentation. A
	// folder row also uses slug for its full path and name for its last
	// segment, and open for whether it is expanded.
	depth int
	open  bool
}

type deckListSort int

const (
	byModified deckListSort = iota
	byName
	bySize
	byKind
)

func (s deckListSort) String() string {
	switch s {
	case byName:
		return "name"
	case bySize:
		return "size"
	case byKind:
		return "kind"
	}
	return "last touched"
}

type deckList struct {
	cursor
	all  []deckEntry
	rows []deckEntry

	order  deckListSort
	filter string

	// legality is what the background pass has worked out so far, kept
	// across reloads so the flags don't blink off every time a deck is
	// written.
	legality map[string]deck.Legality
	colours  map[string][]string

	// confirming is a deletion waiting for a yes, and holds the row it
	// would delete so that moving the cursor can't redirect it.
	confirming *deckEntry

	// collapsed is the set of folder paths the tree is holding shut. Folders
	// are open by default — a folder absent here is expanded — so nothing you
	// have is hidden until you choose to fold it away.
	collapsed map[string]bool

	// moving is a deck staged by y (copy) or x (cut), waiting for a p to place
	// it in a folder. Held here rather than acted on at once so you can move the
	// cursor to the destination first.
	moving *deckMove
}

// deckMove is a deck picked up for a move (cut) or a copy (yank).
type deckMove struct {
	slug string
	name string
	cut  bool
}

// newDeckList reads what's on disk and what's bookmarked.
func newDeckList() *deckList {
	l := &deckList{
		order:     byModified,
		legality:  map[string]deck.Legality{},
		colours:   map[string][]string{},
		collapsed: map[string]bool{},
	}
	l.reload()
	return l
}

// localSlugs is every deck of yours in the list, for the legality pass.
func (l *deckList) localSlugs() []string {
	var out []string
	for _, e := range l.all {
		if e.kind == entryLocal {
			out = append(out, e.slug)
		}
	}
	return out
}

// setLegality files what the background pass worked out and puts it on the
// row it belongs to.
func (l *deckList) setLegality(slug string, verdict deck.Legality, colours []string) {
	if l.legality == nil {
		l.legality = map[string]deck.Legality{}
	}
	if l.colours == nil {
		l.colours = map[string][]string{}
	}
	l.legality[slug] = verdict
	l.colours[slug] = colours
	for i := range l.all {
		if l.all[i].kind == entryLocal && l.all[i].slug == slug {
			l.all[i].legal = verdict
			l.all[i].colours = colours
		}
	}
	l.refresh()
}

// setRemoteMeta fills a followed deck's row with the summary a background
// fetch worked out: its colours, its size, and when it last changed.
func (l *deckList) setRemoteMeta(id string, meta moxfield.Meta) {
	for i := range l.all {
		if l.all[i].kind == entryRemote && l.all[i].id == id {
			l.all[i].colours = meta.Colors
			l.all[i].count = meta.Count
			l.all[i].modified = meta.Updated
		}
	}
	l.refresh()
}

// reload rebuilds from disk. Every change goes through it, so the list can
// never disagree with the files behind it.
func (l *deckList) reload() {
	var all []deckEntry

	summaries, _ := deck.Summaries()
	for _, s := range summaries {
		all = append(all, deckEntry{
			kind: entryLocal, name: s.Name, slug: s.Slug, format: s.Format,
			count: s.Total, modified: s.Modified, broken: s.Broken,
			legal: l.legality[s.Slug], colours: l.colours[s.Slug],
		})
	}

	b := deck.LoadBookmarks()
	for _, r := range b.Remotes {
		all = append(all, deckEntry{
			kind: entryRemote, name: r.Name, id: r.ID,
			colours: r.Colors, count: r.Count, modified: r.Updated,
		})
	}
	for _, u := range b.Users {
		all = append(all, deckEntry{kind: entryUser, name: u, user: u})
	}

	l.all = all
	l.refresh()
}

func (l *deckList) refresh() {
	// A filter flattens the tree: fold the folders away and show every leaf
	// that matches, wherever it lives, so a match can't hide inside a folder
	// you happen to have shut.
	if terms := filterTerms(l.filter); len(terms) > 0 {
		var kept []deckEntry
		for _, e := range l.all {
			if matchesEntry(e, terms) {
				e.depth = 0
				kept = append(kept, e)
			}
		}
		sort.SliceStable(kept, func(i, j int) bool { return lessEntry(kept[i], kept[j], l.order) })
		l.rows = kept
		l.cursor.clamp(len(l.rows))
		return
	}

	l.rows = l.buildTree()
	l.cursor.clamp(len(l.rows))
}

// buildTree lays the entries out as a folder tree flattened for display: each
// folder row, then — unless it is collapsed — its subfolders and the decks
// inside it, indented one level deeper. Local decks group by the folder part of
// their slug; the followed remotes and people all sit under one virtual
// moxfield folder.
func (l *deckList) buildTree() []deckEntry {
	type leaf struct {
		e   deckEntry
		dir string
	}
	var leaves []leaf
	hasMox := false
	for _, e := range l.all {
		switch e.kind {
		case entryLocal:
			leaves = append(leaves, leaf{e, folderOf(e.slug)})
		case entryRemote, entryUser:
			hasMox = true
			leaves = append(leaves, leaf{e, moxFolder})
		}
	}

	// The leaves each folder holds, and how many there are anywhere beneath it.
	leavesByDir := map[string][]deckEntry{}
	count := map[string]int{}
	for _, lf := range leaves {
		leavesByDir[lf.dir] = append(leavesByDir[lf.dir], lf.e)
		for d := lf.dir; d != ""; d = folderOf(d) {
			count[d]++
		}
	}

	// The set of folders, and each one's direct subfolders.
	subfolders := map[string][]string{}
	seen := map[string]bool{}
	addFolder := func(f string) {
		for f != "" {
			parent := folderOf(f)
			if key := parent + "\x00" + f; !seen[key] {
				seen[key] = true
				subfolders[parent] = append(subfolders[parent], f)
			}
			f = parent
		}
	}
	for _, lf := range leaves {
		addFolder(lf.dir)
	}
	if hasMox {
		addFolder(moxFolder)
	}

	var out []deckEntry
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		subs := append([]string(nil), subfolders[dir]...)
		sort.Strings(subs)
		for _, f := range subs {
			open := !l.collapsed[f]
			out = append(out, deckEntry{
				kind: entryFolder, name: pathBase(f), slug: f,
				depth: depth, count: count[f], open: open,
			})
			if open {
				walk(f, depth+1)
			}
		}
		ls := append([]deckEntry(nil), leavesByDir[dir]...)
		sort.SliceStable(ls, func(i, j int) bool { return lessEntry(ls[i], ls[j], l.order) })
		for _, e := range ls {
			e.depth = depth
			out = append(out, e)
		}
	}
	walk("", 0)
	return out
}

// folderOf is the folder a slug or folder path sits in — "aggro" for
// "aggro/mono-red", "" for a root deck. Slugs use forward slashes, so this is
// path.Dir with the "." it returns for a bare name folded to "".
func folderOf(slug string) string {
	if dir := path.Dir(slug); dir != "." {
		return dir
	}
	return ""
}

// pathBase is the last segment of a folder path — "mono-red" for
// "aggro/mono-red".
func pathBase(p string) string { return path.Base(p) }

func matchesEntry(e deckEntry, terms []string) bool {
	hay := strings.ToLower(e.name + " " + e.slug + " " + e.format + " " + e.kind.letter())
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func lessEntry(a, b deckEntry, s deckListSort) bool {
	switch s {
	case byName:
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	case bySize:
		return a.count > b.count
	case byKind:
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	}
	// Last touched, newest first. Things with no date — remotes and users —
	// sort to the bottom rather than claiming 1970.
	if a.modified.IsZero() != b.modified.IsZero() {
		return !a.modified.IsZero()
	}
	if !a.modified.Equal(b.modified) {
		return a.modified.After(b.modified)
	}
	return strings.ToLower(a.name) < strings.ToLower(b.name)
}

func (l *deckList) current() (deckEntry, bool) {
	if l.cursor.at < 0 || l.cursor.at >= len(l.rows) {
		return deckEntry{}, false
	}
	return l.rows[l.cursor.at], true
}

func (l *deckList) setFilter(s string) {
	l.filter = s
	l.refresh()
}

func (l *deckList) cycleSort(delta int) {
	const n = 4
	l.order = deckListSort(((int(l.order)+delta)%n + n) % n)
	l.refresh()
}

// ── As a view ───────────────────────────────────────────────────

func (l *deckList) title() string { return "decks" }

func (l *deckList) subtitle() string {
	leaves := 0
	for _, r := range l.rows {
		if r.kind != entryFolder {
			leaves++
		}
	}
	out := itoa(leaves)
	if leaves != len(l.all) {
		out += "/" + itoa(len(l.all))
	}
	out += " · " + l.order.String()
	if l.filter != "" {
		out += " · /" + l.filter
	}
	// A staged deck is invisible otherwise: nothing on the row says it's about
	// to move, so the subtitle carries it until a p places it.
	if l.moving != nil {
		verb := "copy"
		if l.moving.cut {
			verb = "move"
		}
		out += " · " + verb + " " + l.moving.name
	}
	return out
}

func (l *deckList) lines(width, height int, focused bool, m *Model) []string {
	if l.confirming != nil {
		return fillTo([]string{
			lipgloss.NewStyle().Foreground(theme.Error).Bold(true).
				Render(fit("delete "+l.confirming.name+"?", width)),
			mutedLine("y to confirm · any other key cancels", width),
		}, width, height)
	}
	if len(l.rows) == 0 {
		what := "no decks yet — n to make one"
		if l.filter != "" {
			what = "nothing matches"
		}
		return fillTo([]string{mutedLine(what, width)}, width, height)
	}

	l.cursor.scrollInto(height, len(l.rows))
	// One set of column widths for the whole list, so the kind letter and each
	// value line up down the panel however little any single row carries — a
	// user, who has only their letter, still sits it under the L and R above.
	cols := measureDeckCols(l.rows, width)
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.rows) && len(lines) < height; i++ {
		lines = append(lines, renderEntryCols(l.rows[i], cols, width, focused && i == l.cursor.at))
	}
	return fillTo(lines, width, height)
}

// clear has nothing transient to drop for esc; the filter is cleared with b.
func (l *deckList) clear() bool { return false }

// ── Drawing a row ───────────────────────────────────────────────

// deckCols are the widths the decks list gives its right-hand columns. Shared
// across the list and held fixed per row — a row without colours or a count
// still reserves the space — so every column lines up vertically. A width of
// zero means no row has that column, or the panel is too narrow to keep it.
type deckCols struct {
	pips  int
	count int
	age   int
}

// measureDeckCols works out those widths from the rows, then gives columns up
// from the right until the tail leaves room for a name — the same order the
// old per-row tail yielded in, decided once for the list so the drop is
// uniform and the columns stay aligned.
func measureDeckCols(rows []deckEntry, width int) deckCols {
	var c deckCols
	for _, e := range rows {
		if e.kind == entryFolder {
			continue // folders draw their own row, without these columns
		}
		c.pips = maxInt(c.pips, textWidth(manaPips(e.colours)))
		c.count = maxInt(c.count, textWidth(entryCount(e)))
		c.age = maxInt(c.age, textWidth(shortAge(e.modified)))
	}

	const minName = 12
	for deckTailWidth(c)+minName+1 > width {
		switch {
		case c.age > 0:
			c.age = 0
		case c.count > 0:
			c.count = 0
		case c.pips > 0:
			c.pips = 0
		default:
			return c
		}
	}
	return c
}

// deckTailWidth is how wide the right-hand block is: the two-character kind and
// legality flag, then a space and its width for each column still standing.
func deckTailWidth(c deckCols) int {
	w := 2 // the kind letter and the legality mark
	for _, col := range []int{c.pips, c.count, c.age} {
		if col > 0 {
			w += 1 + col
		}
	}
	return w
}

// entryCount is the size column's text: a local deck says its size even when
// that's zero — blank reads as "we haven't looked", and an empty deck is a
// fact — while a remote says nothing until we've looked, and a person never.
func entryCount(e deckEntry) string {
	if e.kind == entryLocal || e.count > 0 {
		return itoa(e.count)
	}
	return ""
}

// renderEntry draws one row with columns sized to itself, which is what the
// tests want to measure. The list draws with renderEntryCols instead, so its
// columns are shared and aligned.
func renderEntry(e deckEntry, width int, under bool) string {
	return renderEntryCols(e, measureDeckCols([]deckEntry{e}, width), width, under)
}

// renderEntryCols draws one row against a shared set of column widths: the
// name, then a right-aligned tail of kind, legality, colours, size and age.
// Each column keeps its slot whether or not this row fills it, so the columns
// line up down the list.
func renderEntryCols(e deckEntry, cols deckCols, width int, under bool) string {
	if width < 1 {
		return ""
	}

	indent := strings.Repeat("  ", e.depth)

	if e.kind == entryFolder {
		return renderFolderRow(e, indent, width, under)
	}

	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	// The kind letter and its legality mark travel together: two characters
	// saying what this is and whether it's playable.
	slots := []string{dim.Render(e.kind.letter()) + legalMark(e, e.legal.Flag())}
	if cols.pips > 0 {
		slots = append(slots, paintMana(padLeft(manaPips(e.colours), cols.pips)))
	}
	if cols.count > 0 {
		slots = append(slots, dim.Render(padLeft(entryCount(e), cols.count)))
	}
	if cols.age > 0 {
		slots = append(slots, dim.Render(padLeft(shortAge(e.modified), cols.age)))
	}
	right := strings.Join(slots, " ")
	tailWidth := textWidth(stripStyles(right))

	name := indent + e.name
	if e.broken {
		name += " (unreadable)"
	}
	left := fit(name, maxInt(width-tailWidth-1, 0))

	nameStyle := lipgloss.NewStyle().Foreground(theme.Text)
	switch {
	case e.broken:
		nameStyle = nameStyle.Foreground(theme.Error)
	case e.kind == entryUser:
		nameStyle = nameStyle.Foreground(theme.Info)
	case e.kind == entryRemote:
		nameStyle = nameStyle.Foreground(theme.TextDim)
	}
	if under {
		nameStyle = nameStyle.Foreground(theme.SelectionFg).Bold(true)
	}

	line := nameStyle.Render(left) + " " + right
	if under {
		return highlightLine(line, width, theme.SelectionBg)
	}
	return line
}

// renderFolderRow draws a folder: a disclosure marker, its name, and how many
// decks are under it — no legality or colour columns, since a folder has
// neither. The count sits on the right the way a deck's size does.
func renderFolderRow(e deckEntry, indent string, width int, under bool) string {
	marker := "▸"
	if e.open {
		marker = "▾"
	}

	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	count := dim.Render(itoa(e.count))
	tailWidth := textWidth(itoa(e.count))

	nameStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	if under {
		nameStyle = nameStyle.Foreground(theme.SelectionFg)
	}
	left := fit(indent+marker+" "+e.name, maxInt(width-tailWidth-1, 0))

	line := nameStyle.Render(left) + " " + count
	if under {
		return highlightLine(line, width, theme.SelectionBg)
	}
	return line
}

// shortAge is how long ago, in one or two characters plus a unit. Empty for
// things that have no date, rather than a dash that reads as a value.
func shortAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return itoa(maxInt(int(d.Minutes()), 1)) + "m"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h"
	case d < 365*24*time.Hour:
		return itoa(int(d.Hours()/24)) + "d"
	}
	return itoa(int(d.Hours()/24/365)) + "y"
}

// key is the decks panel's own keymap. The mutations arrive in the next
// commit; this is navigation, ordering and opening.
func (l *deckList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	// A pending deletion swallows the next key, whatever it is: a
	// confirmation you can answer by accident isn't one.
	if l.confirming != nil {
		target := *l.confirming
		l.confirming = nil
		if k == "y" {
			return true, m.deleteEntry(l, target)
		}
		return true, nil
	}

	if l.cursor.navKey(k, len(l.rows)) {
		return true, nil
	}

	switch k {
	case "o":
		l.cycleSort(1)
	case "O":
		l.cycleSort(-1)
	case "/":
		p.openFilter(l.filter)
	case "enter":
		if e, ok := l.current(); ok && e.kind == entryFolder {
			l.toggleFolder(e.slug)
			return true, nil
		}
		return true, m.openEntry(l, p, false)
	case "L":
		if e, ok := l.current(); ok && e.kind == entryFolder {
			l.toggleFolder(e.slug)
			return true, nil
		}
		return true, m.openEntry(l, p, true)

	case "s":
		// Mirror to the git remote. Unlike the mutations above this touches
		// the network, so it runs off the main thread and reports back as a
		// notice; the reload a notice triggers shows anything a pull brought in.
		return true, syncDecks

	case "n":
		p.ask(askNewDeck, "name", "")

	case "r":
		if e, ok := l.current(); ok && (e.kind == entryLocal || e.kind == entryRemote) {
			p.ask(askRename, "rename", e.name)
		}

	case "c":
		if e, ok := l.current(); ok && e.kind != entryFolder {
			return true, copyEntry(e)
		}

	case "C":
		// A remote's Considering list, alongside the main copy c would take.
		if e, ok := l.current(); ok && e.kind == entryRemote {
			return true, copyEntryBoth(e)
		}

	// ── Moving decks between folders ─────────────────────────────
	// y picks a deck up to copy, x to move; p drops it into the folder the
	// cursor is in. Staged rather than immediate so you can navigate first.
	case "y":
		if e, ok := l.current(); ok && e.kind == entryLocal {
			l.moving = &deckMove{slug: e.slug, name: e.name, cut: false}
		}
	case "p":
		if l.moving != nil {
			mv := *l.moving
			l.moving = nil
			return true, m.putDeck(mv, l.currentFolder())
		}

	case "x":
		if e, ok := l.current(); ok && e.kind == entryLocal {
			l.moving = &deckMove{slug: e.slug, name: e.name, cut: true}
		}

	case "d":
		e, ok := l.current()
		if !ok || e.kind == entryFolder {
			return true, nil // a folder goes when its last deck does
		}
		if e.kind == entryLocal {
			// A deck file is the only one of the three that loses work.
			// Following and unfollowing are free, so they just happen.
			l.confirming = &e
			return true, nil
		}
		return true, m.deleteEntry(l, e)

	default:
		return false, nil
	}
	return true, nil
}

// toggleFolder opens or shuts a folder, keeping the cursor on it so the row you
// pressed doesn't slide out from under you.
func (l *deckList) toggleFolder(slug string) {
	l.collapsed[slug] = !l.collapsed[slug]
	l.refresh()
	for i, r := range l.rows {
		if r.kind == entryFolder && r.slug == slug {
			l.cursor.at = i
			break
		}
	}
}

// currentFolder is where a put would land: the folder under the cursor, or the
// folder the highlighted deck sits in, or the root.
func (l *deckList) currentFolder() string {
	e, ok := l.current()
	if !ok {
		return ""
	}
	switch e.kind {
	case entryFolder:
		return e.slug
	case entryLocal:
		return folderOf(e.slug)
	}
	return ""
}

// info describes the highlighted row: what it is, how big, how old.
func (l *deckList) info(width int) []string {
	e, ok := l.current()
	if !ok {
		return nil
	}

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	out := []string{head.Render(fit(e.name, width)), ""}
	switch e.kind {
	case entryFolder:
		out = append(out, dim.Render(fit("a folder", width)))
		out = append(out, dim.Render(fit(itoa(e.count)+" decks inside", width)))
		state := "enter to collapse"
		if !e.open {
			state = "enter to expand"
		}
		out = append(out, "", mutedLine(state, width),
			mutedLine("p puts a picked-up deck here", width))
	case entryLocal:
		out = append(out, dim.Render(fit("a deck of yours", width)))
		out = append(out, dim.Render(fit(itoa(e.count)+" cards", width)))
		if age := shortAge(e.modified); age != "" {
			out = append(out, dim.Render(fit("touched "+age+" ago", width)))
		}
		out = append(out, "")
		out = append(out, legalityLines(e.legal, width)...)
		out = append(out, "", mutedLine("enter to open · gv for versions", width))
	case entryRemote:
		out = append(out, dim.Render(fit("on Moxfield", width)))
		out = append(out, "",
			mutedLine("enter to look · c to take a copy", width),
			mutedLine("C copies the considering list too", width))
	case entryUser:
		out = append(out, dim.Render(fit("a person on Moxfield", width)))
		out = append(out, "", mutedLine("enter for their decks", width))
	}
	return out
}

// legalityLines is the verdict and, when it is bad news, what is wrong with
// it. A deck that is merely "illegal" tells you nothing you can act on.
func legalityLines(verdict deck.Legality, width int) []string {
	style := lipgloss.NewStyle().Foreground(theme.TextMuted)
	switch {
	case verdict.Legal:
		style = lipgloss.NewStyle().Foreground(theme.Success)
	case verdict.Known:
		style = lipgloss.NewStyle().Foreground(theme.Error)
	}

	out := wrapStyled(verdict.Summary(), width, style)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	for _, p := range verdict.Problems {
		out = append(out, wrapStyled("· "+p.Text, width, dim)...)
		if len(p.Cards) > 0 {
			out = append(out, wrapStyled("  "+strings.Join(p.Cards, ", "), width,
				lipgloss.NewStyle().Foreground(theme.TextMuted))...)
		}
	}
	return out
}

// manaPips is a deck's colours as the letters people say them in. Five
// characters at most, which is worth the room: "is this the Mardu deck or
// the Simic one" is the question a list of deck names can't answer.
func manaPips(colours []string) string {
	if len(colours) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range []string{"W", "U", "B", "R", "G"} {
		for _, have := range colours {
			if have == c {
				b.WriteString(c)
				break
			}
		}
	}
	return b.String()
}

func legalMark(e deckEntry, mark string) string {
	switch {
	case !e.legal.Known:
		return mark
	case e.legal.Legal:
		return lipgloss.NewStyle().Foreground(theme.Success).Render(mark)
	}
	return lipgloss.NewStyle().Foreground(theme.Error).Render(mark)
}

func (l *deckList) keys() []hintGroup {
	return []hintGroup{
		{"navigation", [][2]string{
			{"j k", "up/down"},
			{"o O", "sort"},
			{"/", "filter"},
		}},
		{"decks", [][2]string{
			{"enter", "open/fold"},
			{"L", "beside"},
			{"n", "new deck"},
			{"r", "rename"},
			{"c", "copy/sync"},
			{"C", "considering"},
			{"x y p", "cut/yank/put"},
			{"s", "sync"},
			{"d", "delete"},
			{"gv", "versions"},
		}},
	}
}
