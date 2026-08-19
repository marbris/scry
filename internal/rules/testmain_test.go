package rules

import (
	"os"
	"path/filepath"
	"testing"

	"scry/internal/paths"
)

// TestMain isolates the test run the same way every other package does, with
// one exception: the rulebook itself.
//
// These tests assert the parser against the real comprehensive rules, and
// Load downloads them when they're absent. Isolating the cache outright
// would mean fetching a megabyte from Wizards on every run. So the real
// rulebook — if it's already been downloaded — is linked into the isolated
// cache, read-only in practice since nothing here writes it. Without it the
// tests skip, which is what they did before.
func TestMain(m *testing.M) {
	existing := ""
	if path := FilePath(); path != "" {
		if _, err := os.Stat(path); err == nil {
			existing = path
		}
	}

	root, err := os.MkdirTemp("", "scry-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	os.Setenv("SCRY_DECKS_DIR", filepath.Join(root, "decks"))

	if existing != "" {
		os.MkdirAll(paths.Cache(), 0755)
		os.Symlink(existing, filepath.Join(paths.Cache(), "comprules.txt"))
	}

	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
