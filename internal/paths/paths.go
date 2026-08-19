// Package paths is where scry keeps its files.
//
// Data and configuration are deliberately separate. Data is what the program
// accumulates on your behalf — the rules download, card caches, deck files,
// session state — and is disposable in the sense that deleting it costs you
// nothing but a re-download. Configuration is what you wrote yourself, and
// isn't.
package paths

import (
	"os"
	"path/filepath"
)

// Data is where downloads, caches and decks live.
//
// Deliberately not $XDG_DATA_HOME. Someone with that variable pointing
// somewhere else would find their decks apparently gone, and a refactor is
// not the place to move a person's files. Worth doing properly one day, with
// a migration; not worth doing by accident.
func Data() string {
	return ensure(filepath.Join(os.Getenv("HOME"), ".local", "share", "scry"))
}

// Config is where themes and settings live: $XDG_CONFIG_HOME/scry, or
// ~/.config/scry when that isn't set. New, so there's nothing to strand.
func Config() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return ensure(filepath.Join(dir, "scry"))
}

// ensure makes the directory and hands back the path either way. A failure
// here means the write that follows fails with a message that actually says
// what was being written, which is more use than one from in here.
func ensure(dir string) string {
	os.MkdirAll(dir, 0755)
	return dir
}
