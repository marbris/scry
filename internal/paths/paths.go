// Package paths is where scry keeps its files.
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

const app = "scry"

// Config holds settings and themes.
func Config() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return ensure(filepath.Join(dir, app))
	}
	// Handles macOS (~/Library/Application Support) and Windows (%AppData%).
	if dir, err := os.UserConfigDir(); err == nil {
		return ensure(filepath.Join(dir, app))
	}
	return ensure(filepath.Join(home(), ".config", app))
}

// Data holds your decks — the one directory here worth backing up.
func Data() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return ensure(filepath.Join(dir, app))
	}
	if unixish() {
		return ensure(filepath.Join(home(), ".local", "share", app))
	}
	// macOS and Windows draw no line between config and data; the platform
	// convention is that application support holds both.
	return Config()
}

// State holds the session and the query history.
func State() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return ensure(filepath.Join(dir, app))
	}
	if unixish() {
		return ensure(filepath.Join(home(), ".local", "state", app))
	}
	return Data()
}

// Cache holds everything re-downloadable.
func Cache() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return ensure(filepath.Join(dir, app))
	}
	// Handles macOS (~/Library/Caches) and Windows (%LocalAppData%).
	if dir, err := os.UserCacheDir(); err == nil {
		return ensure(filepath.Join(dir, app))
	}
	return ensure(filepath.Join(home(), ".cache", app))
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
