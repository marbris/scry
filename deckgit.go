package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// The decks directory is a git repository, so every change to a deck is
// recorded and nothing is ever really lost. We shell out to git rather than
// vendoring an implementation: it's a hundred lines instead of a dependency,
// and it leaves you a repository you can use from the terminal like any
// other — `git log`, `git diff`, `git revert` all work on your decks.
//
// Git is never load-bearing. The deck file is written first and committed
// afterwards, so a machine without git, or a repository in some state we
// didn't expect, costs you the history and not the deck.

const gitNotInstalled = "git isn't installed — decks are saved, but not versioned"

var (
	gitLookupOnce sync.Once
	gitPath       string
)

// gitAvailable reports whether we have a git to shell out to. Looked up
// once: it can't change while the program is running.
func gitAvailable() bool {
	gitLookupOnce.Do(func() {
		if p, err := exec.LookPath("git"); err == nil {
			gitPath = p
		}
	})
	return gitPath != ""
}

// git runs a git command in the decks directory.
func git(args ...string) (string, error) {
	if !gitAvailable() {
		return "", fmt.Errorf("git not found")
	}
	cmd := exec.Command(gitPath, args...)
	cmd.Dir = decksDir()

	out, err := cmd.CombinedOutput()
	text := strings.TrimRight(string(out), "\n")
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("git %s: %s", args[0], firstLine(text))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return text, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ── The repository ──────────────────────────────────────────────

// ensureDeckRepo makes the decks directory a git repository the first time
// it's needed. If it already sits inside one — someone pointed
// SCRY_DECKS_DIR at a checkout they keep themselves — that repository is
// used as it is, rather than nesting a second one inside it.
func ensureDeckRepo() error {
	if !gitAvailable() {
		return fmt.Errorf("git not found")
	}
	if _, err := git("rev-parse", "--git-dir"); err == nil {
		return nil
	}

	if _, err := git("init", "-q"); err != nil {
		return err
	}

	// Committing fails outright without an identity. Rather than refuse to
	// version anything, fall back to one scoped to this repository, leaving
	// a configured identity — global or otherwise — well alone.
	if out, err := git("config", "user.email"); err != nil || strings.TrimSpace(out) == "" {
		if _, err := git("config", "user.email", "scry@localhost"); err != nil {
			return err
		}
		if _, err := git("config", "user.name", "scry"); err != nil {
			return err
		}
	}
	return nil
}

// commitDeck records one deck file. Only that file is staged, so a
// repository holding other things — or another deck edited in your editor —
// is never swept into a commit it didn't ask to be in.
func commitDeck(slug, message string) error {
	if err := ensureDeckRepo(); err != nil {
		return err
	}

	rel := slug + deckFileExt
	if _, err := git("add", "--", rel); err != nil {
		return err
	}

	// Nothing staged means nothing changed — saving a deck you didn't edit
	// is not an error, it just isn't a commit.
	if _, err := git("diff", "--cached", "--quiet", "--", rel); err == nil {
		return nil
	}

	_, err := git("commit", "-q", "-m", message, "--", rel)
	return err
}

// deckHasUncommittedEdits reports whether a tracked deck file differs from
// what was last committed. An untracked deck has no edits to lose — its
// first commit records the whole thing anyway.
func deckHasUncommittedEdits(slug string) bool {
	if !gitAvailable() || !deckExists(slug) {
		return false
	}
	if err := ensureDeckRepo(); err != nil {
		return false
	}

	rel := slug + deckFileExt
	if _, err := git("ls-files", "--error-unmatch", "--", rel); err != nil {
		return false // not tracked yet
	}
	_, err := git("diff", "--quiet", "--", rel)
	return err != nil
}

// deleteDeckCommitted removes a deck and records that too, so a deck you
// delete by mistake is still in the history.
func deleteDeckCommitted(slug string) error {
	if err := deleteDeck(slug); err != nil {
		return err
	}
	return commitDeck(slug, "Delete "+slug)
}

// ── History ─────────────────────────────────────────────────────

type deckCommit struct {
	hash    string
	short   string
	when    string // relative, as git prints it
	subject string
}

// deckHistory lists the commits that touched one deck, newest first.
func deckHistory(slug string, limit int) ([]deckCommit, error) {
	if err := ensureDeckRepo(); err != nil {
		return nil, err
	}

	// %x1f is a unit separator — safe in a way that any character a commit
	// subject might contain is not.
	out, err := git("log", fmt.Sprintf("-%d", limit),
		"--format=%H%x1f%h%x1f%cr%x1f%s", "--", slug+deckFileExt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	var commits []deckCommit
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		commits = append(commits, deckCommit{hash: f[0], short: f[1], when: f[2], subject: f[3]})
	}
	return commits, nil
}

// deckDiff is what one commit did to one deck.
func deckDiff(slug, hash string) (string, error) {
	return git("show", "--format=%b", "--patch", hash, "--", slug+deckFileExt)
}

// deckAt reads a deck as it stood at a commit.
func deckAt(slug, hash string) (*deckFile, error) {
	out, err := git("show", hash+":"+slug+deckFileExt)
	if err != nil {
		return nil, err
	}
	return parseDeckFile(strings.NewReader(out))
}

// restoreDeck brings an old version back as a new commit, rather than
// rewinding history — the version you restored from is still there, and so
// is the one you restored over.
func restoreDeck(slug, hash string) error {
	old, err := deckAt(slug, hash)
	if err != nil {
		return err
	}
	if err := writeDeck(slug, old); err != nil {
		return err
	}

	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	return commitDeck(slug, "Restore "+slug+" to "+short)
}

// ── Saving ──────────────────────────────────────────────────────

// saveDeckVersioned writes a deck and commits it, describing the change in
// the commit message. The write comes first and its error is the only one
// that can fail the save: a deck that's on disk but unversioned is a much
// better outcome than one that is neither.
//
// It returns the commit subject, so the caller can say what it recorded, and
// a warning for anything that went wrong with git alone.
func saveDeckVersioned(slug string, d *deckFile) (subject, warning string, err error) {
	before, _ := readDeck(slug) // nil when the deck is new, which is fine

	// A deck file is meant to be edited in your own editor too, and those
	// edits are only ever committed the next time scry writes. Record them
	// first, or writing over them would lose them for good — the one thing
	// keeping the history is supposed to prevent.
	if deckHasUncommittedEdits(slug) {
		_ = commitDeck(slug, "Edit "+slug+" outside scry")
	}

	if err := writeDeck(slug, d); err != nil {
		return "", "", err
	}

	subject = commitSubject(before, d)
	if !gitAvailable() {
		return subject, gitNotInstalled, nil
	}
	if err := commitDeck(slug, commitMessage(before, d)); err != nil {
		return subject, "saved, but not committed: " + err.Error(), nil
	}
	return subject, "", nil
}

// ── Describing a change ─────────────────────────────────────────

type deckChange struct {
	added    []string
	removed  []string
	requant  []string // quantity changed
	retagged []string
}

func (c deckChange) empty() bool {
	return len(c.added)+len(c.removed)+len(c.requant)+len(c.retagged) == 0
}

func (c deckChange) count() int {
	return len(c.added) + len(c.removed) + len(c.requant) + len(c.retagged)
}

// diffDecks works out what happened between two versions of a deck, by card
// name — the identity a person thinks in.
func diffDecks(before, after *deckFile) deckChange {
	var c deckChange
	if after == nil {
		return c
	}

	index := func(d *deckFile) map[string]deckEntry {
		out := map[string]deckEntry{}
		if d == nil {
			return out
		}
		for _, e := range d.Entries {
			out[strings.ToLower(e.Name)] = e
		}
		return out
	}
	old, now := index(before), index(after)

	for key, e := range now {
		prev, existed := old[key]
		switch {
		case !existed:
			c.added = append(c.added, e.Name)
		case prev.Qty != e.Qty:
			c.requant = append(c.requant, e.Name)
		case !sameTags(prev.Tags, e.Tags):
			c.retagged = append(c.retagged, e.Name)
		}
	}
	for key, e := range old {
		if _, still := now[key]; !still {
			c.removed = append(c.removed, e.Name)
		}
	}

	// Map iteration is random; a commit message must not be.
	for _, s := range [][]string{c.added, c.removed, c.requant, c.retagged} {
		sort.Strings(s)
	}
	return c
}

func sameTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// commitSubject is the one-line summary of a change. Small changes name the
// cards, because "+1 Sol Ring" is the whole story; larger ones count them,
// because forty card names is not a subject line.
func commitSubject(before, after *deckFile) string {
	if before == nil {
		name := after.Name
		if name == "" {
			name = "deck"
		}
		return "Add " + name
	}

	c := diffDecks(before, after)
	if c.empty() {
		return "Update " + after.Name
	}

	if c.count() <= 3 {
		var parts []string
		for _, n := range c.added {
			parts = append(parts, "+"+n)
		}
		for _, n := range c.removed {
			parts = append(parts, "-"+n)
		}
		for _, n := range c.requant {
			parts = append(parts, "requantify "+n)
		}
		for _, n := range c.retagged {
			parts = append(parts, "retag "+n)
		}
		return strings.Join(parts, ", ")
	}

	var parts []string
	if n := len(c.added); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d", n))
	}
	if n := len(c.removed); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d", n))
	}
	if n := len(c.requant); n > 0 {
		parts = append(parts, fmt.Sprintf("%d requantified", n))
	}
	if n := len(c.retagged); n > 0 {
		parts = append(parts, fmt.Sprintf("%d retagged", n))
	}
	return strings.Join(parts, ", ") + plural(" card", c.count())
}

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// commitMessage is the subject plus, when the subject had to summarise, the
// cards it summarised — so `git log` stays skimmable and `git log --format=%b`
// still tells you exactly what moved.
func commitMessage(before, after *deckFile) string {
	subject := commitSubject(before, after)
	if before == nil {
		return subject
	}

	c := diffDecks(before, after)
	if c.count() <= 3 {
		return subject
	}

	var b strings.Builder
	b.WriteString(subject + "\n")
	section := func(label string, names []string) {
		if len(names) == 0 {
			return
		}
		b.WriteString("\n" + label + "\n")
		for _, n := range names {
			b.WriteString("  " + n + "\n")
		}
	}
	section("Added:", c.added)
	section("Removed:", c.removed)
	section("Quantity changed:", c.requant)
	section("Retagged:", c.retagged)
	return strings.TrimRight(b.String(), "\n")
}

// deckRepoPath is where the deck repository lives, for the CLI to print.
func deckRepoPath() string {
	if out, err := git("rev-parse", "--show-toplevel"); err == nil && out != "" {
		return out
	}
	return filepath.Clean(decksDir())
}
