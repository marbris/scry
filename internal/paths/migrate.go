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
