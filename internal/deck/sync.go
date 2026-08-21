package deck

import (
	"fmt"
	"strconv"
	"strings"

	"scry/internal/config"
)

// Syncing the decks directory with a git remote you own — GitHub, or anything
// else git can push to.
//
// There is nothing bespoke here: the decks directory is already a git
// repository (git.go), so "sync" is git fetch, merge, and push, wrapped so the
// TUI and the CLI can both drive it. The remote you choose is recorded in
// scry's own config file, which is the one setting that has to survive a fresh
// checkout — the .git/config that git keeps is rebuilt from it on the next
// sync, so a decks directory copied to a new machine only needs the config to
// find its way home.
//
// Two-way by construction: because the local side is a real history and the
// remote is another, a merge is what reconciles them, and the only thing that
// can go wrong is the one thing git can't decide for you — the same deck edited
// on two machines between syncs. That surfaces as a conflict you resolve with
// git, in a repository you already own.

const (
	// RemoteName is the git remote scry manages. Named the usual thing so the
	// repository reads like any other from the terminal.
	RemoteName = "origin"

	// DefaultBranch is where decks live on the remote. A fixed name means both
	// ends always agree without anyone having to choose one.
	DefaultBranch = "main"
)

// ── Configuration ───────────────────────────────────────────────

// SyncConfigured reports whether a remote has been set up.
func SyncConfigured() bool {
	_, _, ok := SyncRemote()
	return ok
}

// SyncRemote is the configured remote and branch, and whether there is one.
func SyncRemote() (url, branch string, ok bool) {
	c := config.Load()
	if c.Sync == nil || strings.TrimSpace(c.Sync.Remote) == "" {
		return "", "", false
	}
	branch = c.Sync.Branch
	if branch == "" {
		branch = DefaultBranch
	}
	return c.Sync.Remote, branch, true
}

// Connect records a remote and wires the repository up to it, but transfers
// nothing — the first `Sync` does that. Setting it up and doing it are kept
// apart so connecting can't surprise you with a push.
func Connect(url, branch string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("a remote needs a URL")
	}
	if branch == "" {
		branch = DefaultBranch
	}
	if err := ensureRepo(); err != nil {
		return err
	}
	if err := setGitRemote(url); err != nil {
		return err
	}
	ensureBranch(branch)

	c := config.Load()
	c.Sync = &config.Sync{Remote: url, Branch: branch}
	return config.Save(c)
}

// Disconnect forgets the remote, in scry's config and in the repository. It
// touches neither the decks nor their history: syncing is a mirror, and
// unplugging the mirror leaves the thing it reflected exactly as it was.
func Disconnect() error {
	c := config.Load()
	c.Sync = nil
	if err := config.Save(c); err != nil {
		return err
	}
	if GitAvailable() {
		// The remote may not exist — a config written by hand, say — so its
		// absence is not a failure.
		git("remote", "remove", RemoteName)
	}
	return nil
}

// setGitRemote points origin at url, adding it or repointing an existing one.
func setGitRemote(url string) error {
	if _, err := git("remote", "get-url", RemoteName); err == nil {
		_, err := git("remote", "set-url", RemoteName, url)
		return err
	}
	_, err := git("remote", "add", RemoteName, url)
	return err
}

// ensureBranch makes the current branch `name`. Best effort: a repository with
// no commits yet has no branch to rename, so HEAD is pointed at the branch
// instead, and the first commit is born on it.
func ensureBranch(name string) {
	if !GitAvailable() {
		return
	}
	if _, err := git("rev-parse", "--verify", "HEAD"); err == nil {
		git("branch", "-M", name)
		return
	}
	git("symbolic-ref", "HEAD", "refs/heads/"+name)
}

// ── Doing the sync ──────────────────────────────────────────────

// SyncResult is what one sync moved, for a one-line report.
type SyncResult struct {
	Remote    string
	Branch    string
	Pulled    int // commits merged in from the remote
	Pushed    int // commits sent to the remote
	FirstPush bool
}

// Summary is the sync in a sentence.
func (r SyncResult) Summary() string {
	switch {
	case r.FirstPush:
		if r.Pushed == 0 {
			return "connected — nothing to push yet"
		}
		return fmt.Sprintf("pushed %d %s to the new remote", r.Pushed, plural("commit", r.Pushed))
	case r.Pulled == 0 && r.Pushed == 0:
		return "already up to date"
	case r.Pulled > 0 && r.Pushed > 0:
		return fmt.Sprintf("pulled %d, pushed %d", r.Pulled, r.Pushed)
	case r.Pulled > 0:
		return fmt.Sprintf("pulled %d %s", r.Pulled, plural("commit", r.Pulled))
	default:
		return fmt.Sprintf("pushed %d %s", r.Pushed, plural("commit", r.Pushed))
	}
}

// Sync brings the decks repository and its remote into step: local edits are
// committed, the remote is merged in, and the result is pushed. It is the one
// call behind both `scry sync` and the s key in the decks list.
func Sync() (SyncResult, error) {
	if !GitAvailable() {
		return SyncResult{}, fmt.Errorf("git isn't installed, so there's nothing to sync with")
	}
	url, branch, ok := SyncRemote()
	if !ok {
		return SyncResult{}, fmt.Errorf("syncing isn't set up — run `scry sync remote <url>`")
	}
	if err := ensureRepo(); err != nil {
		return SyncResult{}, err
	}
	if err := setGitRemote(url); err != nil {
		return SyncResult{}, err
	}

	res := SyncResult{Remote: url, Branch: branch}

	// Commit anything edited outside scry, so the push carries every change and
	// not just the ones scry itself wrote.
	if err := commitPending(); err != nil {
		return res, err
	}
	ensureBranch(branch)

	if _, err := git("rev-parse", "--verify", "HEAD"); err != nil {
		return res, fmt.Errorf("no decks to sync yet — make or import one first")
	}

	if _, err := git("fetch", RemoteName); err != nil {
		return res, err
	}

	remoteRef := RemoteName + "/" + branch
	remoteHasBranch := false
	if _, err := git("rev-parse", "--verify", remoteRef); err == nil {
		remoteHasBranch = true
	}

	if remoteHasBranch {
		res.Pulled = countRange(branch + ".." + remoteRef)
		if res.Pulled > 0 {
			merge := []string{"merge", "--no-edit", remoteRef}
			// A remote you created with its own README or licence shares no
			// history with your decks; stitch the two roots together on the
			// first sync rather than refusing an "unrelated histories" merge.
			if _, err := git("merge-base", branch, remoteRef); err != nil {
				merge = []string{"merge", "--no-edit", "--allow-unrelated-histories", remoteRef}
			}
			if out, err := git(merge...); err != nil {
				// Leave the working tree clean rather than dropping the user
				// into a half-merged repository the next scry commit would
				// trip over. The two versions are both safe in the history.
				git("merge", "--abort")
				return res, fmt.Errorf(
					"the same deck was changed on two machines — sync can't merge that on its own.\n"+
						"Resolve it with git in %s, then sync again.\n%s",
					RepoPath(), firstLine(out))
			}
		}
		res.Pushed = countRange(remoteRef + ".." + branch)
	} else {
		res.FirstPush = true
		res.Pushed = countRange(branch)
	}

	if res.Pushed > 0 || res.FirstPush {
		if _, err := git("push", "-u", RemoteName, branch); err != nil {
			return res, err
		}
	}
	return res, nil
}

// SyncStatus is what `scry sync status` reports without changing anything.
type SyncStatus struct {
	Remote          string
	Branch          string
	Ahead           int // local commits the remote doesn't have
	Behind          int // remote commits we don't have
	HasRemoteBranch bool
}

// Status fetches from the remote and works out how far apart the two ends are,
// without merging or pushing.
func Status() (SyncStatus, error) {
	url, branch, ok := SyncRemote()
	st := SyncStatus{Remote: url, Branch: branch}
	if !ok {
		return st, fmt.Errorf("not connected")
	}
	if !GitAvailable() {
		return st, fmt.Errorf("git isn't installed")
	}
	if err := ensureRepo(); err != nil {
		return st, err
	}
	if err := setGitRemote(url); err != nil {
		return st, err
	}
	if _, err := git("fetch", RemoteName); err != nil {
		return st, err
	}

	remoteRef := RemoteName + "/" + branch
	if _, err := git("rev-parse", "--verify", remoteRef); err != nil {
		return st, nil // the branch isn't on the remote yet — ahead/behind mean nothing
	}
	st.HasRemoteBranch = true
	st.Ahead = countRange(remoteRef + ".." + branch)
	st.Behind = countRange(branch + ".." + remoteRef)
	return st, nil
}

// commitPending records anything changed in the decks directory but not yet
// committed — an edit made in your editor, or on another tool. Scoped to the
// directory by the "." pathspec, so a repository you keep other things in
// isn't swept into the commit.
func commitPending() error {
	if _, err := git("add", "-A", "--", "."); err != nil {
		return err
	}
	if _, err := git("diff", "--cached", "--quiet", "--", "."); err == nil {
		return nil // nothing staged: nothing to commit
	}
	_, err := git("commit", "-q", "-m", "Sync local deck changes")
	return err
}

// countRange counts the commits in a `a..b` range, treating any trouble as
// zero — the count only feeds a human-readable summary, so a wrong zero is a
// vaguer sentence, never a failed sync.
func countRange(rng string) int {
	out, err := git("rev-list", "--count", rng)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}
