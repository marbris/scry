package deck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"scry/internal/config"
)

// runGit runs a git command in dir, failing the test on error. Used to stand
// up a bare "remote" and a second working clone, so the sync is exercised
// against real git rather than a mock of it.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent"),
		"GIT_CONFIG_SYSTEM="+filepath.Join(dir, "nonexistent"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// bareRemote makes an empty bare repository to act as the GitHub the tests
// don't have, and returns a path git can push to.
func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--bare", "-q")
	return dir
}

func TestSyncFirstPushThenPull(t *testing.T) {
	gitRepo(t) // isolates dirs and skips without git

	// A deck of ours, committed the way the app commits.
	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}

	remote := bareRemote(t)

	// Nothing is set up yet.
	if SyncConfigured() {
		t.Fatal("SyncConfigured before any Connect")
	}
	if err := Connect(remote, DefaultBranch); err != nil {
		t.Fatal(err)
	}
	if !SyncConfigured() {
		t.Fatal("SyncConfigured false after Connect")
	}
	if c := config.Load(); c.Sync == nil || c.Sync.Remote != remote {
		t.Fatalf("remote not saved to config: %+v", c.Sync)
	}

	// First sync publishes what we have.
	res, err := Sync()
	if err != nil {
		t.Fatal(err)
	}
	if !res.FirstPush || res.Pushed == 0 {
		t.Fatalf("first sync: got %+v, want a first push with commits", res)
	}

	// A second sync with no changes on either side is a no-op.
	res, err = Sync()
	if err != nil {
		t.Fatal(err)
	}
	if res.Pulled != 0 || res.Pushed != 0 {
		t.Fatalf("idle sync moved commits: %+v", res)
	}

	// Now a "second machine": clone the remote, add a deck, push it.
	other := t.TempDir()
	runGit(t, filepath.Dir(other), "clone", "-q", remote, filepath.Base(other))
	// The bare remote's HEAD needn't point at our branch, so pin the checkout
	// to it rather than trusting clone's default.
	runGit(t, other, "checkout", "-q", "-B", DefaultBranch, "origin/"+DefaultBranch)
	if err := os.WriteFile(filepath.Join(other, "izzet.deck"),
		[]byte("name: Izzet\nformat: commander\n\n[mainboard]\n1 Opt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "-A")
	runGit(t, other, "commit", "-q", "-m", "Add Izzet")
	runGit(t, other, "push", "-q", "origin", "HEAD:"+DefaultBranch)

	// Back on our machine, a sync pulls the other machine's deck in.
	res, err = Sync()
	if err != nil {
		t.Fatal(err)
	}
	if res.Pulled == 0 {
		t.Fatalf("expected to pull the remote's commit, got %+v", res)
	}
	if !Exists("izzet") {
		t.Fatal("the pulled deck isn't on disk")
	}

	// And Disconnect leaves the decks alone while forgetting the remote.
	if err := Disconnect(); err != nil {
		t.Fatal(err)
	}
	if SyncConfigured() {
		t.Fatal("still configured after Disconnect")
	}
	if !Exists("ghen") || !Exists("izzet") {
		t.Fatal("Disconnect lost a deck")
	}
}

func TestSyncConflictAbortsCleanly(t *testing.T) {
	gitRepo(t)

	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}
	remote := bareRemote(t)
	if err := Connect(remote, DefaultBranch); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(); err != nil {
		t.Fatal(err)
	}

	// Second machine changes the commander line one way and pushes.
	other := t.TempDir()
	runGit(t, filepath.Dir(other), "clone", "-q", remote, filepath.Base(other))
	runGit(t, other, "checkout", "-q", "-B", DefaultBranch, "origin/"+DefaultBranch)
	if err := os.WriteFile(filepath.Join(other, "ghen.deck"),
		[]byte("name: Ghen\nformat: commander\n\n[commander]\n1 Ghen, Arcanum Weaver [theirs]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "commit", "-aqm", "Theirs")
	runGit(t, other, "push", "-q", "origin", DefaultBranch)

	// We change the same line the other way and try to sync.
	mine := deckOf(t, gitBaseDeck)
	mine.Entries[0].Tags = []string{"mine"} // the commander entry
	if _, _, err := SaveVersioned("ghen", mine); err != nil {
		t.Fatal(err)
	}

	if _, err := Sync(); err == nil {
		t.Fatal("a conflicting sync should report an error")
	}

	// The abort must leave the tree clean — no merge in progress, no markers —
	// so the next scry commit isn't poisoned.
	if _, err := os.Stat(filepath.Join(RepoPath(), ".git", "MERGE_HEAD")); err == nil {
		t.Fatal("a merge is still in progress after the aborted sync")
	}
	body, err := os.ReadFile(Path("ghen"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "<<<<<<<") || strings.Contains(string(body), ">>>>>>>") {
		t.Fatalf("conflict markers left in the deck file:\n%s", body)
	}
}

// A remote created through a host's UI often already has a commit — a README
// or licence — so it shares no history with your decks. The first sync must
// stitch the two together rather than refusing an unrelated-histories merge.
func TestSyncUnrelatedHistoriesOnFirstSync(t *testing.T) {
	gitRepo(t)
	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}

	remote := bareRemote(t)

	// Seed the remote's branch with a README, via a throwaway clone.
	seed := t.TempDir()
	runGit(t, filepath.Dir(seed), "clone", "-q", remote, filepath.Base(seed))
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("# my decks\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", "-A")
	runGit(t, seed, "commit", "-q", "-m", "Initial commit")
	runGit(t, seed, "push", "-q", "origin", "HEAD:"+DefaultBranch)

	if err := Connect(remote, DefaultBranch); err != nil {
		t.Fatal(err)
	}
	res, err := Sync()
	if err != nil {
		t.Fatalf("first sync into a pre-seeded remote failed: %v", err)
	}
	if res.Pulled == 0 || res.Pushed == 0 {
		t.Fatalf("expected to pull the README and push the deck, got %+v", res)
	}
	if !Exists("ghen") {
		t.Fatal("our deck vanished across the merge")
	}
	if _, err := os.Stat(filepath.Join(Dir(), "README.md")); err != nil {
		t.Fatal("the remote's README wasn't merged in")
	}
}

func TestSyncWithoutConfigFails(t *testing.T) {
	gitRepo(t)
	if _, err := Sync(); err == nil {
		t.Fatal("Sync with no remote configured should error")
	}
}
