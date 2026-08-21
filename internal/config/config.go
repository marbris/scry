// Package config is scry's settings file: one small JSON object in the config
// directory, holding the handful of choices that outlive a session.
//
// It is deliberately one owner for one file. The theme lived here first and
// wrote the whole file itself; sync now shares it, so both read-modify-write
// through here rather than each marshalling its own struct and stepping on the
// other's keys.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"scry/internal/paths"
)

// Config is the whole settings file. Every field carries omitempty, so a file
// only ever holds what was actually set — an untouched install writes nothing
// it didn't have to.
type Config struct {
	Theme string `json:"theme,omitempty"`
	Sync  *Sync  `json:"sync,omitempty"`
}

// Sync is where your decks are mirrored: a git remote you own, and the branch
// they live on. Absent until `scry sync` is set up.
type Sync struct {
	Remote string `json:"remote"`
	Branch string `json:"branch,omitempty"`
}

// Path is the settings file itself.
func Path() string { return filepath.Join(paths.Config(), "config.json") }

// Load reads the settings. A missing or unreadable file is not an error — it
// is simply the zero config, which every caller already treats as "nothing set
// yet".
func Load() Config {
	var c Config
	body, err := os.ReadFile(Path())
	if err != nil {
		return c
	}
	json.Unmarshal(body, &c)
	return c
}

// Save writes the settings back, whole. Callers Load, change one field, and
// Save, so the keys they don't touch survive.
func Save(c Config) error {
	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(body, '\n'), 0644)
}
