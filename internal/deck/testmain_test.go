package deck

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects every directory the program writes to, so a test can
// never touch the decks, settings or cache of whoever is running it.
//
// This lives here rather than in each helper because an individual test
// can't be trusted to remember. Setting HOME used to be enough; it stopped
// being enough when the paths started honouring XDG_*, and the first thing
// that happened was a test fixture appearing in a real deck directory.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "scry-test-*")
	if err != nil {
		panic(err)
	}

	os.Setenv("HOME", root)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	// Belt and braces: decks have their own override, and they are the one
	// thing here that would be someone's own work.
	os.Setenv("SCRY_DECKS_DIR", filepath.Join(root, "decks"))

	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

// isolate gives one test its own directories. TestMain isolates the package
// from the machine; this isolates a test from its neighbours, which matters
// for anything asserting that a file was or wasn't written.
func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("SCRY_DECKS_DIR", filepath.Join(root, "decks"))
	return root
}
