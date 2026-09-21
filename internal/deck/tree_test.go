package deck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListWalksFoldersAndReturnsPathSlugs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SCRY_DECKS_DIR", dir)

	for _, slug := range []string{"loose", "aggro/mono-red", "aggro/mono-red-budget", "control/azorius"} {
		if err := Write(slug, deckOf(t, "name: X\nformat: commander\n")); err != nil {
			t.Fatalf("Write(%q): %v", slug, err)
		}
	}

	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"aggro/mono-red", "aggro/mono-red-budget", "control/azorius", "loose"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

func TestWriteReadRoundTripInAFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SCRY_DECKS_DIR", dir)

	if err := Write("aggro/mono-red", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}
	// The folder was created for it, and the file sits inside.
	if _, err := os.Stat(filepath.Join(dir, "aggro", "mono-red.deck")); err != nil {
		t.Fatalf("file not written into its folder: %v", err)
	}
	d, err := Read("aggro/mono-red")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Ghen" {
		t.Errorf("round-trip lost the deck: name is %q", d.Name)
	}
}

func TestMoveBetweenFoldersKeepsContentAndPrunes(t *testing.T) {
	dir := gitRepo(t)

	if _, _, err := SaveVersioned("aggro/mono-red", deckOf(t, gitBaseDeck)); err != nil {
		t.Fatal(err)
	}
	if err := Move("aggro/mono-red", "control/mono-red"); err != nil {
		t.Fatal(err)
	}

	if Exists("aggro/mono-red") {
		t.Error("the deck is still at its old slug")
	}
	d, err := Read("control/mono-red")
	if err != nil {
		t.Fatalf("the deck isn't at its new slug: %v", err)
	}
	if d.Name != "Ghen" {
		t.Errorf("move lost the deck: name is %q", d.Name)
	}
	// The emptied folder is swept up, but the decks directory stays.
	if _, err := os.Stat(filepath.Join(dir, "aggro")); !os.IsNotExist(err) {
		t.Error("the emptied folder was left behind")
	}
	// History follows the move.
	commits, err := History("control/mono-red", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) < 2 {
		t.Errorf("history didn't follow the move: %d commits", len(commits))
	}
}

func TestMoveFolderRelocatesEveryDeckUnderIt(t *testing.T) {
	dir := gitRepo(t)

	for slug, body := range map[string]string{
		"aggro/mono-red": "name: Mono Red\nformat: commander\n[mainboard]\n1 Sol Ring\n",
		"aggro/budget":   "name: Budget\nformat: commander\n[mainboard]\n1 Sol Ring\n",
		"other/keep":     "name: Keep\nformat: commander\n[mainboard]\n1 Sol Ring\n",
	} {
		if _, _, err := SaveVersioned(slug, deckOf(t, body)); err != nil {
			t.Fatalf("seeding %q: %v", slug, err)
		}
	}

	if err := MoveFolder("aggro", "midrange"); err != nil {
		t.Fatal(err)
	}

	if Exists("aggro/mono-red") || Exists("aggro/budget") {
		t.Error("a deck is still under the old folder")
	}
	for slug, want := range map[string]string{"midrange/mono-red": "Mono Red", "midrange/budget": "Budget"} {
		d, err := Read(slug)
		if err != nil {
			t.Fatalf("not at %q: %v", slug, err)
		}
		if d.Name != want {
			t.Errorf("%s name is %q, want %q unchanged", slug, d.Name, want)
		}
	}
	// A folder outside the renamed one is untouched.
	if !Exists("other/keep") {
		t.Error("a deck in another folder was moved")
	}
	// The emptied folder is swept up.
	if _, err := os.Stat(filepath.Join(dir, "aggro")); !os.IsNotExist(err) {
		t.Error("the emptied folder was left behind")
	}
}

func TestNewFolderMakesAnEmptyDirectoryTheTreeCanSee(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SCRY_DECKS_DIR", dir)

	slug, err := NewFolder("Aggro / Mono Red")
	if err != nil {
		t.Fatal(err)
	}
	if slug != "aggro/mono-red" {
		t.Errorf("folder slug is %q, want aggro/mono-red", slug)
	}
	if info, err := os.Stat(filepath.Join(dir, "aggro", "mono-red")); err != nil || !info.IsDir() {
		t.Fatalf("the directory wasn't made: %v", err)
	}

	// It holds no decks, but Folders still reports it (and its parent).
	folders, err := Folders()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(folders, ",") != "aggro,aggro/mono-red" {
		t.Errorf("Folders() = %v, want the empty folders listed", folders)
	}
}
