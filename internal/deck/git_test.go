package deck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo puts the decks directory somewhere temporary and skips the test if
// there's no git to shell out to.
func gitRepo(t *testing.T) string {
	t.Helper()
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir := filepath.Join(isolate(t), "decks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCRY_DECKS_DIR", dir)
	// Keep the test off whatever the machine's git identity is.
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "nonexistent-gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(dir, "nonexistent-gitconfig"))
	return dir
}

func deckOf(t *testing.T, text string) *File {
	t.Helper()
	d, err := ParseFile(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const gitBaseDeck = `name: Ghen
format: commander

[commander]
1 Ghen, Arcanum Weaver [wincon]

[mainboard]
1 Sol Ring [ramp]
1 Arcane Signet [ramp]
7 Plains
`

func TestCommitSubject(t *testing.T) {
	base := deckOf(t, gitBaseDeck)

	tests := []struct {
		name   string
		before *File
		after  string
		want   string
	}{
		{
			name:   "a new deck",
			before: nil,
			after:  gitBaseDeck,
			want:   "Add Ghen",
		},
		{
			name:   "one card added",
			before: base,
			after:  gitBaseDeck + "1 Smothering Tithe [ramp]\n",
			want:   "+Smothering Tithe",
		},
		{
			name:   "one card removed",
			before: base,
			after:  strings.Replace(gitBaseDeck, "1 Sol Ring [ramp]\n", "", 1),
			want:   "-Sol Ring",
		},
		{
			name:   "a swap names both cards",
			before: base,
			after:  strings.Replace(gitBaseDeck, "1 Sol Ring [ramp]", "1 Mana Crypt [ramp]", 1),
			want:   "+Mana Crypt, -Sol Ring",
		},
		{
			name:   "quantity",
			before: base,
			after:  strings.Replace(gitBaseDeck, "7 Plains", "9 Plains", 1),
			want:   "requantify Plains",
		},
		{
			name:   "tags",
			before: base,
			after:  strings.Replace(gitBaseDeck, "1 Sol Ring [ramp]", "1 Sol Ring [ramp, artifact]", 1),
			want:   "retag Sol Ring",
		},
		{
			name:   "nothing at all",
			before: base,
			after:  gitBaseDeck,
			want:   "Update Ghen",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commitSubject(tt.before, deckOf(t, tt.after)); got != tt.want {
				t.Errorf("commitSubject = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommitSubjectSummarisesLargeChanges(t *testing.T) {
	base := deckOf(t, gitBaseDeck)

	// Tagging in bulk is the change this has to survive: forty card names is
	// not a subject line.
	after := deckOf(t, gitBaseDeck)
	for i := range after.Entries {
		after.Entries[i].Tags = append(after.Entries[i].Tags, "flying")
	}

	got := commitSubject(base, after)
	if !strings.Contains(got, "4 retagged") {
		t.Errorf("commitSubject = %q, want a count of what was retagged", got)
	}
	if strings.Contains(got, "Sol Ring") {
		t.Errorf("commitSubject named cards in a bulk change: %q", got)
	}

	// The detail still has to be somewhere, so the body carries it.
	msg := commitMessage(base, after)
	if !strings.Contains(msg, "Retagged:") || !strings.Contains(msg, "Sol Ring") {
		t.Errorf("commit body lost the detail:\n%s", msg)
	}
	if !strings.HasPrefix(msg, got) {
		t.Errorf("commit message should start with its subject:\n%s", msg)
	}
}

func TestCommitSubjectIsStable(t *testing.T) {
	// Built from maps, so without sorting the subject would shuffle between
	// runs and every commit message would be a coin toss.
	base := deckOf(t, gitBaseDeck)
	after := deckOf(t, gitBaseDeck+"1 Smothering Tithe\n1 Mana Crypt\n1 Chrome Mox\n")

	first := commitMessage(base, after)
	for i := 0; i < 20; i++ {
		if got := commitMessage(base, after); got != first {
			t.Fatalf("commit message is not deterministic:\n%q\nvs\n%q", first, got)
		}
	}
}

func TestSaveDeckVersioned(t *testing.T) {
	gitRepo(t)

	subject, warning, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck))
	if err != nil {
		t.Fatalf("SaveVersioned: %v", err)
	}
	if warning != "" {
		t.Errorf("unexpected warning: %q", warning)
	}
	if subject != "Add Ghen" {
		t.Errorf("subject = %q", subject)
	}

	commits, err := History("ghen", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	if commits[0].Subject != "Add Ghen" {
		t.Errorf("commit subject = %q", commits[0].Subject)
	}

	// A second save with a real change is a second commit.
	changed := deckOf(t, strings.Replace(gitBaseDeck, "1 Sol Ring [ramp]", "1 Mana Crypt [ramp]", 1))
	if _, _, err := SaveVersioned("ghen", changed); err != nil {
		t.Fatal(err)
	}
	commits, _ = History("ghen", 10)
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
	if commits[0].Subject != "+Mana Crypt, -Sol Ring" {
		t.Errorf("commit subject = %q", commits[0].Subject)
	}

	// Saving an unchanged deck is not an error, and not a commit either.
	if _, _, err := SaveVersioned("ghen", changed); err != nil {
		t.Fatal(err)
	}
	commits, _ = History("ghen", 10)
	if len(commits) != 2 {
		t.Errorf("saving an unchanged deck made a commit: %d commits", len(commits))
	}
}

func TestSaveCommitsEditorChangesBeforeOverwriting(t *testing.T) {
	dir := gitRepo(t)

	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}

	// Someone edits the file in vim and doesn't commit it.
	edited := strings.Replace(gitBaseDeck, "7 Plains", "7 Plains\n1 Smothering Tithe [ramp]", 1)
	if err := os.WriteFile(filepath.Join(dir, "ghen.deck"), []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	// Then scry writes over it. The editor's work must be in the history,
	// not lost — the whole point of versioning the decks.
	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}

	commits, err := History("ghen", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range commits {
		if strings.Contains(c.Subject, "outside scry") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the editor's changes were overwritten without being recorded: %+v", commits)
	}

	// And they can be got back.
	for _, c := range commits {
		if strings.Contains(c.Subject, "outside scry") {
			d, err := At("ghen", c.Hash)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(d.String(), "Smothering Tithe") {
				t.Error("the recovered version doesn't have the edit in it")
			}
		}
	}
}

func TestRestoreDeck(t *testing.T) {
	gitRepo(t)

	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}
	changed := deckOf(t, strings.Replace(gitBaseDeck, "1 Sol Ring [ramp]", "1 Mana Crypt [ramp]", 1))
	if _, _, err := SaveVersioned("ghen", changed); err != nil {
		t.Fatal(err)
	}

	commits, _ := History("ghen", 10)
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
	first := commits[1].Hash

	if err := Restore("ghen", first); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	back, err := Read("ghen")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(back.String(), "Sol Ring") || strings.Contains(back.String(), "Mana Crypt") {
		t.Errorf("restore didn't bring the old version back:\n%s", back.String())
	}

	// Restoring moves forward rather than rewinding: the version restored
	// over is still there.
	after, _ := History("ghen", 10)
	if len(after) != 3 {
		t.Errorf("got %d commits after a restore, want 3 — history should not be rewritten", len(after))
	}
	if !strings.HasPrefix(after[0].Subject, "Restore") {
		t.Errorf("newest commit = %q", after[0].Subject)
	}
}

func TestDeleteDeckIsRecoverable(t *testing.T) {
	gitRepo(t)

	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}
	if err := DeleteCommitted("ghen"); err != nil {
		t.Fatalf("DeleteCommitted: %v", err)
	}
	if Exists("ghen") {
		t.Fatal("deck still on disk")
	}

	commits, err := History("ghen", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2 (the add and the delete)", len(commits))
	}

	// A deck deleted by mistake is still in there.
	d, err := At("ghen", commits[1].Hash)
	if err != nil {
		t.Fatalf("cannot read a deleted deck out of the history: %v", err)
	}
	if len(d.Entries) != 4 {
		t.Errorf("recovered %d entries, want 4", len(d.Entries))
	}
}

func TestRepoWorksWithNoGitIdentity(t *testing.T) {
	// A machine where git has never been configured must still version
	// decks rather than silently failing to commit.
	gitRepo(t)

	subject, warning, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck))
	if err != nil {
		t.Fatal(err)
	}
	if warning != "" {
		t.Errorf("git with no identity configured warned: %q", warning)
	}
	if subject != "Add Ghen" {
		t.Errorf("subject = %q", subject)
	}
	if commits, _ := History("ghen", 10); len(commits) != 1 {
		t.Errorf("nothing was committed without a global git identity")
	}
}

func TestDeckSurvivesAnUnusableRepo(t *testing.T) {
	dir := gitRepo(t)

	// A .git that is a file rather than a directory: git will refuse to do
	// anything here. The deck still has to be saved.
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("not a repo"), 0644); err != nil {
		t.Fatal(err)
	}

	subject, warning, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck))
	if err != nil {
		t.Fatalf("a broken repo must not fail the save: %v", err)
	}
	if !Exists("ghen") {
		t.Fatal("the deck was not written")
	}
	if warning == "" {
		t.Error("a failed commit should have been reported")
	}
	if subject == "" {
		t.Error("the change should still be described")
	}

	// And the file itself is intact.
	if _, err := Read("ghen"); err != nil {
		t.Errorf("deck does not read back: %v", err)
	}
}

func TestOnlyTheNamedDeckIsCommitted(t *testing.T) {
	dir := gitRepo(t)

	if _, _, err := SaveVersioned("ghen", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}

	// Something else in the directory — another deck mid-edit, or anything
	// the person keeps there — must not be swept into a commit.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("private"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.deck"), []byte("name: Other\n[mainboard]\n1 Plains\n"), 0644); err != nil {
		t.Fatal(err)
	}

	changed := deckOf(t, strings.Replace(gitBaseDeck, "7 Plains", "9 Plains", 1))
	if _, _, err := SaveVersioned("ghen", changed); err != nil {
		t.Fatal(err)
	}

	out, err := git("show", "--name-only", "--format=", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	files := strings.Fields(out)
	if len(files) != 1 || files[0] != "ghen.deck" {
		t.Errorf("commit touched %v, want only ghen.deck", files)
	}
}
