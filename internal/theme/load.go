package theme

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"scry/internal/paths"
)

//go:embed themes/*.json
var builtin embed.FS

// DefaultName is the theme in force when nothing says otherwise, and the
// source of every fallback.
const DefaultName = "gruvbox"

// fallbackPalette is the default theme's colours, used for anything a theme
// leaves out. Filled in at startup from the embedded file, so there is one
// definition of gruvbox rather than two that can drift.
var fallbackPalette = map[string]string{}

func init() {
	if t, err := builtinTheme(DefaultName); err == nil {
		fallbackPalette = t.Palette
	}
	// A palette before any config is read, so a program that never calls
	// Load still draws in colour rather than in the zero value.
	Use(Theme{Palette: fallbackPalette})
}

// ── Configuration ───────────────────────────────────────────────

type config struct {
	Theme string `json:"theme"`
}

func configPath() string { return filepath.Join(paths.Config(), "config.json") }

func readConfig() config {
	var c config
	body, err := os.ReadFile(configPath())
	if err != nil {
		return c
	}
	json.Unmarshal(body, &c)
	return c
}

// Set writes the chosen theme to the config file, having checked it exists.
func Set(name string) error {
	if _, err := Find(name); err != nil {
		return err
	}
	body, err := json.MarshalIndent(config{Theme: name}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), append(body, '\n'), 0644)
}

// Current is the name of the configured theme.
func Current() string {
	if name := readConfig().Theme; name != "" {
		return name
	}
	return DefaultName
}

// Load puts the configured theme in force. A theme that is missing or
// unreadable is reported, and the default stays up — a typo in a config file
// shouldn't leave someone staring at an unusable screen.
func Load() error {
	// Lay the built-ins down as real files first, so there is always one to
	// read and one to copy the format from.
	SeedBuiltins()

	name := Current()
	t, err := Find(name)
	if err != nil {
		return err
	}
	Use(t)
	return nil
}

// SeedBuiltins writes the built-in themes into your config themes directory,
// so the theme in force is a file you can read and the format is one you can
// see and copy. It only ever fills gaps: a theme file already there may be one
// you have edited — including a built-in you have changed — so it is left
// exactly as it is. Best-effort throughout; a read-only config directory just
// means the embedded copies keep serving as the fallback.
func SeedBuiltins() {
	dir := Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	files, _ := builtin.ReadDir("themes")
	for _, f := range files {
		name, ok := themeName(f.Name())
		if !ok {
			continue
		}
		path := filepath.Join(dir, name+".json")
		if _, err := os.Stat(path); err == nil {
			continue // already there — leave any edits alone
		}
		body, err := builtin.ReadFile("themes/" + f.Name())
		if err != nil {
			continue
		}
		os.WriteFile(path, body, 0644)
	}
}

// ── Finding themes ──────────────────────────────────────────────

// Dir is where your own themes go.
func Dir() string { return filepath.Join(paths.Config(), "themes") }

// Find looks for a theme by name: yours first, so a file in your config
// directory replaces a built-in of the same name.
func Find(name string) (Theme, error) {
	if t, err := readTheme(filepath.Join(Dir(), name+".json")); err == nil {
		return t, nil
	}
	t, err := builtinTheme(name)
	if err != nil {
		return Theme{}, fmt.Errorf("no theme called %q — try `scry theme`", name)
	}
	return t, nil
}

// List names every theme available, yours and the built-ins, without
// duplicates.
func List() []string {
	seen := map[string]bool{}
	var out []string

	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}

	entries, _ := os.ReadDir(Dir())
	for _, e := range entries {
		if name, ok := themeName(e.Name()); ok {
			add(name)
		}
	}
	files, _ := builtin.ReadDir("themes")
	for _, f := range files {
		if name, ok := themeName(f.Name()); ok {
			add(name)
		}
	}

	sort.Strings(out)
	return out
}

// IsBuiltin reports whether a name is one of ours, which is what tells a
// listing that a theme can be copied as a starting point.
func IsBuiltin(name string) bool {
	_, err := builtinTheme(name)
	return err == nil
}

func themeName(file string) (string, bool) {
	if !strings.HasSuffix(file, ".json") {
		return "", false
	}
	return strings.TrimSuffix(file, ".json"), true
}

func builtinTheme(name string) (Theme, error) {
	body, err := builtin.ReadFile("themes/" + name + ".json")
	if err != nil {
		return Theme{}, err
	}
	return parse(body)
}

func readTheme(path string) (Theme, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, err
	}
	return parse(body)
}

func parse(body []byte) (Theme, error) {
	var t Theme
	if err := json.Unmarshal(body, &t); err != nil {
		return Theme{}, err
	}
	return t, nil
}

// Export writes a theme out as JSON, for copying a built-in into your config
// directory and editing it.
func Export(name string) ([]byte, error) {
	t, err := Find(name)
	if err != nil {
		return nil, err
	}
	// Roles are written out in full, so the file shows every knob there is
	// rather than only the ones this theme happened to change.
	if t.Roles == nil {
		t.Roles = map[string]string{}
	}
	for role, dflt := range defaultRoles {
		if _, set := t.Roles[role]; !set {
			t.Roles[role] = dflt
		}
	}
	body, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

// PaletteNames is the colours a theme may name, in a sensible reading order.
func PaletteNames() []string { return paletteNames }
