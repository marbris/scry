// Package paths is where ttr keeps its files.
//
// Four kinds of file, four places, following the XDG Base Directory spec on
// Linux and the platform convention elsewhere:
//
//   - Config is what you wrote: settings and themes. Never written by us
//     without being asked.
//   - Data is what you made: your decks. Losing it loses work.
//   - State is where you were: the session, the query history. Losing it is
//     a mild annoyance.
//   - Cache is what we downloaded: the rules, card data, printed text.
//     Losing it costs a re-download and nothing else.
//
// The split matters because it tells a backup what to copy and `rm` what is
// safe to remove. Everything used to live in one directory, which said none
// of that.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const app = "ttr"

// oldApp is what the directories were called before the program was renamed
// from scry to Tutor. MigrateName moves them.
const oldApp = "scry"

// Config holds settings and themes.
func Config() string { return ensure(configDir(app)) }

// Data holds your decks — the one directory here worth backing up.
func Data() string { return ensure(dataDir(app)) }

// State holds the session and the query history.
func State() string { return ensure(stateDir(app)) }

// Cache holds everything re-downloadable.
func Cache() string { return ensure(cacheDir(app)) }

// The four directories for a name, worked out without being made — so the
// old names can be looked for without leaving empty directories behind.

func configDir(name string) string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, name)
	}
	// Handles macOS (~/Library/Application Support) and Windows (%AppData%).
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, name)
	}
	return filepath.Join(home(), ".config", name)
}

func dataDir(name string) string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, name)
	}
	if unixish() {
		return filepath.Join(home(), ".local", "share", name)
	}
	// macOS and Windows draw no line between config and data; the platform
	// convention is that application support holds both.
	return configDir(name)
}

func stateDir(name string) string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, name)
	}
	if unixish() {
		return filepath.Join(home(), ".local", "state", name)
	}
	return dataDir(name)
}

func cacheDir(name string) string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, name)
	}
	// Handles macOS (~/Library/Caches) and Windows (%LocalAppData%).
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, name)
	}
	return filepath.Join(home(), ".cache", name)
}

func unixish() bool {
	return runtime.GOOS != "darwin" && runtime.GOOS != "windows" && runtime.GOOS != "plan9"
}

func home() string {
	if dir, err := os.UserHomeDir(); err == nil {
		return dir
	}
	return os.Getenv("HOME")
}

// ensure makes the directory and hands back the path either way. A failure
// here means the write that follows fails with a message that actually says
// what was being written, which is more use than one from in here.
func ensure(dir string) string {
	os.MkdirAll(dir, 0755)
	return dir
}
