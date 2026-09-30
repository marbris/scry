package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// xdgHome points every directory at a fresh temporary tree, the way a Linux
// session with the variables set would.
func xdgHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	return root
}

func TestXDGVariablesAreHonoured(t *testing.T) {
	root := xdgHome(t)

	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"config", Config(), filepath.Join(root, "cfg", "ttr")},
		{"data", Data(), filepath.Join(root, "data", "ttr")},
		{"state", State(), filepath.Join(root, "state", "ttr")},
		{"cache", Cache(), filepath.Join(root, "cache", "ttr")},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
		if _, err := os.Stat(c.got); err != nil {
			t.Errorf("%s was not created: %v", c.name, err)
		}
	}
}

func TestTheFourDirectoriesAreDistinct(t *testing.T) {
	// The whole point of the split is that a backup can copy Data without
	// dragging a hundred megabytes of re-downloadable set files with it.
	xdgHome(t)
	seen := map[string]string{}
	for name, dir := range map[string]string{
		"config": Config(), "data": Data(), "state": State(), "cache": Cache(),
	} {
		if other, dup := seen[dir]; dup {
			t.Errorf("%s and %s are both %s", name, other, dir)
		}
		seen[dir] = name
	}
}

func TestMigrateMovesEachFileToItsCategory(t *testing.T) {
	root := xdgHome(t)

	// The old layout: everything in one directory.
	legacy := filepath.Join(root, ".local", "share", "ttr")
	if err := os.MkdirAll(filepath.Join(legacy, "originals"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"comprules.txt", "cards.json", "session.json", "queries.json"} {
		if err := os.WriteFile(filepath.Join(legacy, f), []byte(f), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(legacy, "originals", "AAA.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	Migrate()

	for _, c := range []struct{ dir, name string }{
		{Cache(), "comprules.txt"},
		{Cache(), "cards.json"},
		{Cache(), filepath.Join("originals", "AAA.json")},
		{State(), "session.json"},
		{State(), "queries.json"},
	} {
		if _, err := os.Stat(filepath.Join(c.dir, c.name)); err != nil {
			t.Errorf("%s did not arrive in %s: %v", c.name, c.dir, err)
		}
	}
}

func TestMigrateLeavesDecksAlone(t *testing.T) {
	// Decks were already in the data directory and that is still where they
	// belong. Moving them would be moving someone's work.
	root := xdgHome(t)
	legacy := filepath.Join(root, ".local", "share", "ttr", "decks")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	deck := filepath.Join(legacy, "ghen.list")
	if err := os.WriteFile(deck, []byte("name: Ghen\n"), 0644); err != nil {
		t.Fatal(err)
	}

	Migrate()

	if _, err := os.Stat(deck); err != nil {
		t.Errorf("the deck moved, or went missing: %v", err)
	}
}

func TestMigrateNeverOverwrites(t *testing.T) {
	// A file already at the destination is the current one; the copy left
	// behind in the old place is stale by definition.
	root := xdgHome(t)
	legacy := filepath.Join(root, ".local", "share", "ttr")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "session.json"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(State(), "session.json"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	Migrate()

	body, err := os.ReadFile(filepath.Join(State(), "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "new" {
		t.Errorf("session.json = %q, want the newer copy to survive", body)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	root := xdgHome(t)
	legacy := filepath.Join(root, ".local", "share", "ttr")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "cards.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	Migrate()
	Migrate()

	if _, err := os.Stat(filepath.Join(Cache(), "cards.json")); err != nil {
		t.Errorf("the second run lost the file: %v", err)
	}
}

func TestTheOldScryDirectoriesMoveToTheNewName(t *testing.T) {
	root := xdgHome(t)
	oldDecks := filepath.Join(root, "data", "scry", "decks")
	os.MkdirAll(oldDecks, 0755)
	os.WriteFile(filepath.Join(oldDecks, "elves.txt"), []byte("1 Llanowar Elves\n"), 0644)
	os.MkdirAll(filepath.Join(root, "cfg", "scry"), 0755)
	os.WriteFile(filepath.Join(root, "cfg", "scry", "config.json"), []byte(`{"theme":"nord"}`), 0644)
	// A run before the move would have made an empty new directory.
	os.MkdirAll(filepath.Join(root, "cfg", "ttr"), 0755)
	// And a new directory with something in it wins.
	os.MkdirAll(filepath.Join(root, "state", "scry"), 0755)
	os.MkdirAll(filepath.Join(root, "state", "ttr"), 0755)
	os.WriteFile(filepath.Join(root, "state", "ttr", "session.json"), []byte("{}"), 0644)

	Migrate()

	if _, err := os.Stat(filepath.Join(Data(), "decks", "elves.txt")); err != nil {
		t.Errorf("the decks didn't move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(Config(), "config.json")); err != nil {
		t.Errorf("the settings didn't move over an empty directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "scry")); !os.IsNotExist(err) {
		t.Error("the old data directory is still there")
	}
	if _, err := os.Stat(filepath.Join(root, "state", "scry")); err != nil {
		t.Error("the old state directory was moved over a new one that has files")
	}
}
