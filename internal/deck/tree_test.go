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
