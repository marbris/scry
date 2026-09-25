package ui

import (
	"fmt"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/deck"
	"scry/internal/moxfield"
)

// What the decks panel actually does: opening things, and changing them.
//
// Everything that touches the network or the disk runs off the main thread
// and comes back as a message. The panel it came from is found by id, since
// the row can be reordered or closed while a deck is being resolved.

// ── Messages ────────────────────────────────────────────────────

type deckOpenedMsg struct {
	panel   int
	newPane bool
	info    deck.Info
	cards   []deck.Card
	err     error
	// uncommitted is a deck of yours whose file has changes since its last
	// commit — edits from last time, written but never committed.
	uncommitted bool
}

type userDecksMsg struct {
	panel   int
	newPane bool
	user    string
	decks   []moxfield.UserDeck
	err     error
}

// reloadDecksMsg tells every decks panel to read the directory again, after
// something changed it.
type reloadDecksMsg struct{}

// noticeMsg is a one-line result — "copied", "deleted" — shown until the
// next keypress.
type noticeMsg struct {
	text string
	err  error
}

// ── Opening ─────────────────────────────────────────────────────

// openEntry acts on the highlighted row: a deck becomes a list of cards, a
// person becomes a list of their decks.
func (m *Model) openEntry(l *deckList, p *panel, newPane bool) tea.Cmd {
	e, ok := l.current()
	if !ok {
		return nil
	}

	target := p
	if newPane {
		// L opens to the right and leaves you where you were, so the list
		// you were browsing is still under the cursor.
		target = m.ws.open(p.kind)
		m.ws.focus(m.ws.indexOf(p))
	}
	target.loading = true
	target.err = nil

	switch e.kind {
	case entryLocal:
		target.title = e.name
		return openLocalDeck(target.id, newPane, e.slug)
	case entryRemote:
		target.title = e.name
		return openRemoteDeck(target.id, newPane, e.id)
	case entryUser:
		target.title = e.user
		return openUserDecks(target.id, newPane, e.user)
	}
	return nil
}

func openLocalDeck(panelID int, newPane bool, slug string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := deck.Open(slug)
		return deckOpenedMsg{panel: panelID, newPane: newPane, info: info, cards: cards, err: err,
			uncommitted: deck.HasUncommittedEdits(slug)}
	}
}

func openRemoteDeck(panelID int, newPane bool, id string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := moxfield.Load(id)
		if err == nil {
			// Looking at somebody's deck is how you decide to follow it, so
			// following happens by looking. It costs a line in a file, and
			// the alternative is finding your way back to a deck you saw
			// once and can't name.
			follow := deck.LoadBookmarks()
			follow.AddRemote(deck.Remote{Name: info.Name, ID: id, URL: info.URL})
			deck.SaveBookmarks(follow)
		}
		return deckOpenedMsg{panel: panelID, newPane: newPane, info: info, cards: cards, err: err}
	}
}

func openUserDecks(panelID int, newPane bool, user string) tea.Cmd {
	return func() tea.Msg {
		name, decks, err := moxfield.UserDecks(user)
		if name == "" {
			name = user
		}
		return userDecksMsg{panel: panelID, newPane: newPane, user: name, decks: decks, err: err}
	}
}

// handleDeckOpened files a resolved deck.
func (m Model) handleDeckOpened(msg deckOpenedMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil // the panel closed while the cards were resolving
	}
	p.loading = false

	// A deck whose cards mostly resolved is still worth showing; the error
	// rides alongside as a notice rather than instead of the deck.
	if msg.err != nil && len(msg.cards) == 0 {
		p.err = msg.err
		return m, nil
	}
	if msg.err != nil {
		m.notice = msg.err.Error()
	}

	l := newCardList(msg.cards, sortArrival, "decklist")
	l.name = msg.info.Name
	l.deck = &msg.info
	l.dirty = msg.uncommitted
	l.recheck()

	// A deck opened in its own panel replaces what was there; one opened
	// from the decks list steps into it, so esc goes back to the list.
	if msg.newPane || p.top() == nil {
		p.show(l)
	} else {
		p.push(l)
		p.title = l.name
	}
	m.ws.deriveEditingIfUnpinned()
	// A remote just followed should appear in any decks list on screen.
	return m, reloadDecks
}

func (m Model) handleUserDecks(msg userDecksMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	p.loading = false
	if msg.err != nil {
		p.err = msg.err
		return m, nil
	}

	v := newUserDeckList(msg.user, msg.decks)
	if msg.newPane || p.top() == nil {
		p.show(v)
	} else {
		p.push(v)
		p.title = v.title()
	}
	return m, nil
}

// ── Changing things ─────────────────────────────────────────────

// deleteEntry removes whatever the row stands for: a deck file, a remote
// link, a person. Only the first of those loses anything that can't be got
// back, which is why only that one asks first.
func (m *Model) deleteEntry(l *deckList, e deckEntry) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			if err := deck.DeleteCommitted(e.slug); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "deleted " + e.name}
		case entryRemote:
			b := deck.LoadBookmarks()
			b.RemoveRemote(e.id)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "stopped following " + e.name}
		case entryUser:
			b := deck.LoadBookmarks()
			b.RemoveUser(e.user)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "removed " + e.user}
		}
		return nil
	}
}

// copyEntry duplicates a local deck, or takes a local copy of a remote one.
// A remote whose copy already exists is synced instead — overwritten, and
// the change recorded in git, which is what makes overwriting safe.
func copyEntry(e deckEntry) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			d, err := deck.Read(e.slug)
			if err != nil {
				return noticeMsg{err: err}
			}
			slug, copied, err := deck.New(uniqueName(d.Name+" copy"), d.Format)
			if err != nil {
				return noticeMsg{err: err}
			}
			copied.Entries = d.Entries
			if _, _, err := deck.SaveVersioned(slug, copied); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "copied to " + slug}

		case entryRemote:
			d, err := moxfield.Import(e.id)
			if err != nil {
				return noticeMsg{err: err}
			}
			slug := deck.Slugify(d.Name)
			existed := deck.Exists(slug)
			subject, _, err := deck.SaveVersioned(slug, d)
			if err != nil {
				return noticeMsg{err: err}
			}
			if existed {
				if subject == "" {
					return noticeMsg{text: slug + " is already up to date"}
				}
				return noticeMsg{text: "synced " + slug + ": " + subject}
			}
			return noticeMsg{text: "copied to " + slug}
		}
		return noticeMsg{text: "nothing to copy"}
	}
}

// copyEntryBoth takes a remote deck's main list and its Considering list as
// two separate local decks — c makes one copy, C makes both. The Considering
// list is where an author keeps the cards they're weighing, and it's often
// the more interesting half to borrow.
func copyEntryBoth(e deckEntry) tea.Cmd {
	return func() tea.Msg {
		if e.kind != entryRemote {
			return noticeMsg{text: "only a Moxfield deck has a considering list"}
		}

		main, err := moxfield.Import(e.id)
		if err != nil {
			return noticeMsg{err: err}
		}
		mainSlug := deck.Slugify(main.Name)
		if _, _, err := deck.SaveVersioned(mainSlug, main); err != nil {
			return noticeMsg{err: err}
		}

		cons, err := moxfield.ImportConsidering(e.id)
		if err != nil {
			return noticeMsg{err: err}
		}
		if cons == nil {
			return noticeMsg{text: "copied to " + mainSlug + " · nothing being considered"}
		}
		consSlug := deck.Slugify(cons.Name)
		if _, _, err := deck.SaveVersioned(consSlug, cons); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "copied to " + mainSlug + " and " + consSlug}
	}
}

// followRemote bookmarks a Moxfield deck without taking a copy of it — the r
// on somebody's decks, which saves the deck as a remote you can come back to.
func followRemote(name, id, url string) tea.Cmd {
	return func() tea.Msg {
		b := deck.LoadBookmarks()
		b.AddRemote(deck.Remote{Name: name, ID: id, URL: url})
		if err := deck.SaveBookmarks(b); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "following " + name}
	}
}

// uniqueName finds a name whose slug isn't taken, so copying twice gives you
// two decks rather than an error.
func uniqueName(base string) string {
	if !deck.Exists(deck.Slugify(base)) {
		return base
	}
	for i := 2; i < 100; i++ {
		name := fmt.Sprintf("%s %d", base, i)
		if !deck.Exists(deck.Slugify(name)) {
			return name
		}
	}
	return base
}

// newDeck makes an empty one, in the folder the decks panel was pointing at. A
// deck you've just created is empty by definition, and you fill it by adding
// cards to it.
func newDeckCmd(dir, name string) tea.Cmd {
	return func() tea.Msg {
		slug, d, err := deck.New(inFolder(dir, name), deck.DefaultFormat)
		if err != nil {
			return noticeMsg{err: err}
		}
		if _, _, err := deck.SaveVersioned(slug, d); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "made " + slug}
	}
}

// newFolder makes an empty directory in the tree, under the folder the panel is
// pointing at. Typing "dirname/" at the new prompt lands here — a folder made
// before it has any decks, so you can file decks into it afterwards.
func newFolderCmd(dir, name string) tea.Cmd {
	return func() tea.Msg {
		slug, err := deck.NewFolder(inFolder(dir, name))
		if err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "made folder " + slug}
	}
}

// inFolder puts a name inside a folder, for slugifying into a path. A name that
// already carries its own slashes nests under the folder all the same.
func inFolder(dir, name string) string {
	if dir == "" || dir == moxFolder {
		return name
	}
	return dir + "/" + name
}

// putDeck places a staged deck into a folder: a cut moves it, a yank copies it.
// The moxfield folder is virtual, so nothing local can be put there.
func (m *Model) putDeck(mv deckMove, dir string) tea.Cmd {
	if dir == moxFolder {
		return func() tea.Msg {
			return noticeMsg{err: fmt.Errorf("the moxfield folder is for followed decks, not yours")}
		}
	}
	dest := uniqueSlug(inFolder(dir, path.Base(mv.slug)))
	// Only the file's location changes; the deck keeps its own name, which no
	// longer carries the folder. The destination folder is where it now lives.
	where := dir
	if where == "" {
		where = "the top level"
	}

	if mv.cut {
		return func() tea.Msg {
			if dest == mv.slug {
				return noticeMsg{text: mv.name + " is already there"}
			}
			if err := deck.Move(mv.slug, dest); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "moved " + mv.name + " to " + where}
		}
	}
	return func() tea.Msg {
		d, err := deck.Read(mv.slug)
		if err != nil {
			return noticeMsg{err: err}
		}
		if _, _, err := deck.SaveVersioned(dest, d); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "copied " + mv.name + " to " + where}
	}
}

// uniqueSlug finds a free slug at or beside the one asked for, so putting a
// deck where one already lives makes a second rather than an error.
func uniqueSlug(slug string) string {
	if !deck.Exists(slug) {
		return slug
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", slug, i)
		if !deck.Exists(candidate) {
			return candidate
		}
	}
	return slug
}

// renameDeck changes a local deck's title, or what a remote is called in your
// list. It changes the name alone: the slug names a file with a git history, so
// the file stays put and the history stays keyed to that path. A deck moves
// between folders with cut and put, not by being renamed.
func renameCmd(e deckEntry, name string) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			d, err := deck.Read(e.slug)
			if err != nil {
				return noticeMsg{err: err}
			}
			d.Name = strings.TrimSpace(name)
			if _, _, err := deck.SaveVersioned(e.slug, d); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "renamed to " + name}
		case entryRemote:
			b := deck.LoadBookmarks()
			b.RenameRemote(e.id, name)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "renamed to " + name}
		}
		return noticeMsg{text: "a person can't be renamed"}
	}
}

// renameFolderCmd renames a folder — the folder is a location on disk, so this
// moves every deck under it to the renamed path, each keeping its own name. The
// moxfield folder is virtual, built from the bookmarks, so there's nothing to
// rename there.
func renameFolderCmd(e deckEntry, name string) tea.Cmd {
	return func() tea.Msg {
		if e.slug == moxFolder {
			return noticeMsg{err: fmt.Errorf("the moxfield folder isn't yours to rename")}
		}
		newPath := inFolder(folderOf(e.slug), deck.Slugify(strings.TrimSpace(name)))
		if newPath == "" {
			return noticeMsg{err: fmt.Errorf("a folder needs a name")}
		}
		if newPath == e.slug {
			return noticeMsg{text: e.slug + " is already called that"}
		}
		if err := deck.MoveFolder(e.slug, newPath); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "renamed folder to " + newPath}
	}
}

// follow records whatever was pasted into the decks panel's search bar: a
// Moxfield deck URL or id becomes a remote, anything else is taken for a
// username.
func follow(input string) tea.Cmd {
	return func() tea.Msg {
		b := deck.LoadBookmarks()

		if id, ok := moxfield.Ref(input); ok {
			name := id
			if d, err := moxfield.Fetch(id); err == nil && d.Name != "" {
				name = d.Name
			}
			b.AddRemote(deck.Remote{Name: name, ID: id, URL: input})
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "following " + name}
		}

		user, ok := moxfield.User(input)
		if !ok {
			return noticeMsg{err: fmt.Errorf("not a Moxfield deck or username: %q", input)}
		}
		b.AddUser(user)
		if err := deck.SaveBookmarks(b); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "following " + user}
	}
}

// syncDecks mirrors the decks directory to its git remote — the s key in the
// decks panel. Setting the remote up is a terminal job (`scry sync init`); this
// is only the recurring push-and-pull, so the whole of it is one background
// call whose result becomes a notice. The reload that follows any notice picks
// up whatever a pull brought in.
func syncDecks() tea.Msg {
	if !deck.SyncConfigured() {
		return noticeMsg{err: fmt.Errorf("syncing isn't set up — run `scry sync remote <url>` in your shell")}
	}
	res, err := deck.Sync()
	if err != nil {
		return noticeMsg{err: err}
	}
	return noticeMsg{text: "sync: " + res.Summary()}
}

// reloadDecks tells every decks panel to read the directory again.
func reloadDecks() tea.Msg { return reloadDecksMsg{} }

// loadDecks is the background work a freshly opened decks list wants doing:
// the legality of the local decks, from the cache, and the summary of the
// followed ones, from Moxfield. Both fill in beside the rows as they arrive.
func loadDecks(l *deckList) tea.Cmd {
	return tea.Batch(checkLegality(l.localSlugs()), refreshRemotes())
}

// ── Legality ────────────────────────────────────────────────────

// legalityMsg carries the verdict on one deck back to the lists showing it,
// and its colours, which come out of the same resolution.
type legalityMsg struct {
	slug     string
	legality deck.Legality
	colours  []string
}

// checkLegality works out whether each local deck is legal, from the card
// cache alone. One command per deck rather than one for all of them, so the
// first answers appear while the rest are still being worked out.
func checkLegality(slugs []string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(slugs))
	for _, slug := range slugs {
		s := slug
		cmds = append(cmds, func() tea.Msg {
			verdict, colours := deck.CheckCached(s)
			return legalityMsg{slug: s, legality: verdict, colours: colours}
		})
	}
	return tea.Batch(cmds...)
}

// ── Remote summaries ────────────────────────────────────────────

// remoteMetaMsg carries a followed deck's summary back to the lists showing
// it — its colours, size and age, worked out without opening it.
type remoteMetaMsg struct {
	id   string
	meta moxfield.Meta
}

// refreshRemotes fetches the summary of every followed deck that hasn't been
// looked at yet, so the decks list can show a remote's colours, size and age
// beside its name. Only the un-fetched ones, and one request each, so a list
// of remotes fills in over a moment rather than blocking on all of them —
// and a second visit to the panel costs nothing.
func refreshRemotes() tea.Cmd {
	b := deck.LoadBookmarks()
	var cmds []tea.Cmd
	for _, r := range b.Remotes {
		if r.Fetched {
			continue
		}
		id := r.ID
		cmds = append(cmds, func() tea.Msg {
			meta, err := moxfield.FetchMeta(id)
			if err != nil {
				return nil // a remote we can't reach keeps just its name
			}
			return remoteMetaMsg{id: id, meta: meta}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m Model) handleRemoteMeta(msg remoteMetaMsg) (tea.Model, tea.Cmd) {
	// Keep it for next time, so the fetch happens once rather than on every
	// visit to the decks panel.
	b := deck.LoadBookmarks()
	b.SetRemoteMeta(msg.id, msg.meta.Colors, msg.meta.Count, msg.meta.Updated)
	deck.SaveBookmarks(b)

	for _, p := range m.ws.panels {
		if l, ok := p.top().(*deckList); ok {
			l.setRemoteMeta(msg.id, msg.meta)
		}
	}
	return m, nil
}

func (m Model) handleLegality(msg legalityMsg) (tea.Model, tea.Cmd) {
	for _, p := range m.ws.panels {
		if l, ok := p.top().(*deckList); ok {
			l.setLegality(msg.slug, msg.legality, msg.colours)
		}
		if l := p.cardsView(); l != nil && l.deck != nil && l.deck.Slug == msg.slug {
			legality := msg.legality
			l.legality = &legality
		}
	}
	return m, nil
}
