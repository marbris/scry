package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects every directory the program writes to, the same as
// every other package here.
//
// It matters more in this one than most: the decks panel reads the deck
// directory the moment it opens, and its keys create, rename and delete
// decks. A test run that found the real directory would be editing someone's
// collection.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "scry-ui-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	os.Setenv("SCRY_DECKS_DIR", filepath.Join(root, "decks"))

	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
