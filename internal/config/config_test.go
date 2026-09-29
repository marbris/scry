package config

import (
	"os"
	"path/filepath"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
}

func TestLoadMissingIsZero(t *testing.T) {
	isolate(t)
	c := Load()
	if c.Theme != "" || c.Sync != nil {
		t.Fatalf("a missing config should load as zero, got %+v", c)
	}
}

// The two writers of this file — the theme and sync setups — must not tread on
// each other: saving one back must keep the other. This is the reason the file
// has a single owner rather than two structs each marshalling their own view.
func TestThemeAndSyncCoexist(t *testing.T) {
	isolate(t)

	c := Load()
	c.Theme = "nord"
	if err := Save(c); err != nil {
		t.Fatal(err)
	}

	// A later, independent load-modify-save for sync must leave the theme be.
	c = Load()
	c.Sync = &Sync{Remote: "git@example.com:me/decks.git", Branch: "main"}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}

	got := Load()
	if got.Theme != "nord" {
		t.Fatalf("theme lost when sync was saved: %q", got.Theme)
	}
	if got.Sync == nil || got.Sync.Remote != "git@example.com:me/decks.git" {
		t.Fatalf("sync not persisted: %+v", got.Sync)
	}
}

func TestSaveLeavesABrokenFileAlone(t *testing.T) {
	isolate(t)
	os.MkdirAll(filepath.Dir(Path()), 0755)
	broken := []byte("{ \"theme\": \"nord\",, }")
	if err := os.WriteFile(Path(), broken, 0644); err != nil {
		t.Fatal(err)
	}
	if err := Save(Config{Theme: "gruvbox"}); err == nil {
		t.Error("saved over a file that couldn't be read")
	}
	if got, _ := os.ReadFile(Path()); string(got) != string(broken) {
		t.Errorf("the file was changed: %s", got)
	}
}

func TestACommentedFileLoads(t *testing.T) {
	isolate(t)
	os.MkdirAll(filepath.Dir(Path()), 0755)
	body := "// a comment\n{\n  // \"theme\": \"nord\",\n  \"theme\": \"gruvbox\" // chosen\n}\n"
	if err := os.WriteFile(Path(), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Load().Theme; got != "gruvbox" {
		t.Errorf("theme %q", got)
	}
}
