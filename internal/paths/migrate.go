package paths

import (
	"os"
	"path/filepath"
)

// Everything used to live in the data directory, downloads and session state
// alongside the decks. Migrate puts each file where it now belongs.
//
// Only moves, never deletes, and never overwrites: a file already at the
// destination wins, and one that fails to move is left where it is. The
// worst case is a stale copy in the old place and a re-download, which is
// why nothing here reports an error.
//
// Decks aren't in the list. They were already in the data directory and
// that's still where they belong.
func Migrate() {
	MigrateName()
	legacy := filepath.Join(home(), ".local", "share", app)

	for _, m := range []struct {
		name string
		to   func() string
	}{
		{"comprules.txt", Cache},
		{"cards.json", Cache},
		{"originals", Cache},
		{"session.json", State},
		{"queries.json", State},
	} {
		move(filepath.Join(legacy, m.name), filepath.Join(m.to(), m.name))
	}
}

func move(from, to string) {
	if from == to {
		return
	}
	if _, err := os.Stat(from); err != nil {
		return // nothing to move
	}
	if _, err := os.Stat(to); err == nil {
		return // something is already there; it wins
	}
	os.MkdirAll(filepath.Dir(to), 0755)
	os.Rename(from, to)
}

// MigrateName moves each of the old scry directories to its new name, the
// first time ttr runs: your decks, settings, session and downloads come
// along with the rename rather than being left behind under the old one.
//
// A directory is only moved onto a name that isn't taken — or is taken only
// by an empty directory, which is what a run before the move would have
// left. A new directory with anything in it wins, and the old one is left
// where it is for you to look at.
func MigrateName() {
	for _, dir := range []func(string) string{configDir, dataDir, stateDir, cacheDir} {
		from, to := dir(oldApp), dir(app)
		if from == to {
			continue
		}
		if info, err := os.Stat(from); err != nil || !info.IsDir() {
			continue // nothing to move — or already moved, where two share a directory
		}
		if entries, err := os.ReadDir(to); err == nil {
			if len(entries) > 0 {
				continue
			}
			os.Remove(to)
		}
		os.MkdirAll(filepath.Dir(to), 0755)
		os.Rename(from, to)
	}
}
