package theme

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func configHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "cfg"))
	t.Cleanup(func() { Use(Theme{Palette: fallbackPalette}) })
	return root
}

func TestAPaletteIsAWholeTheme(t *testing.T) {
	// The point of the split: thirteen colours and no roles at all still
	// produces a complete, coherent interface, because the default mapping
	// resolves against whatever palette it's given.
	Use(Theme{
		Name: "inverted",
		Palette: map[string]string{
			"bg": "#ffffff", "bgAlt": "#eeeeee", "fg": "#000000",
			"fgDim": "#555555", "white": "#000000", "gray": "#888888",
			"red": "#aa0000", "green": "#00aa00", "yellow": "#aaaa00",
			"blue": "#0000aa", "purple": "#aa00aa", "aqua": "#00aaaa",
			"orange": "#aa5500",
		},
	})

	for name, got := range map[string]lipgloss.Color{
		"Surface": Surface, "Accent": Accent, "Error": Error,
		"ManaG": ManaG, "RarityMythic": RarityMythic, "BarFill": BarFill,
	} {
		if got == "" {
			t.Errorf("%s is empty; every role should have resolved", name)
		}
	}

	// Accent follows the mapping (orange), not gruvbox's orange.
	if Accent != lipgloss.Color("#aa5500") {
		t.Errorf("Accent = %s, want the theme's own orange", Accent)
	}
	if Surface != lipgloss.Color("#ffffff") {
		t.Errorf("Surface = %s, want the theme's own bg", Surface)
	}
}

func TestARoleCanBeOverridden(t *testing.T) {
	Use(Theme{
		Palette: map[string]string{"bg": "#000000", "aqua": "#00ffff", "orange": "#ff8800"},
		Roles:   map[string]string{"borderFocus": "aqua"},
	})
	if BorderFocus != lipgloss.Color("#00ffff") {
		t.Errorf("BorderFocus = %s, want the aqua it was pointed at", BorderFocus)
	}
	if Accent != lipgloss.Color("#ff8800") {
		t.Errorf("Accent = %s, want the default mapping's orange", Accent)
	}
}

func TestARoleCanNameAColourOutright(t *testing.T) {
	Use(Theme{
		Palette: map[string]string{"bg": "#000000"},
		Roles:   map[string]string{"accent": "#ff00ff"},
	})
	if Accent != lipgloss.Color("#ff00ff") {
		t.Errorf("Accent = %s, want the literal it was given", Accent)
	}
}

func TestMissingColoursFallBackToTheDefault(t *testing.T) {
	// A theme that names three colours is still usable; the rest come from
	// gruvbox rather than coming out blank.
	Use(Theme{Palette: map[string]string{"bg": "#101010", "fg": "#f0f0f0", "red": "#ff0000"}})

	if Bg != lipgloss.Color("#101010") {
		t.Errorf("Bg = %s, want the theme's own", Bg)
	}
	if Error != lipgloss.Color("#ff0000") {
		t.Errorf("Error = %s, want the theme's own red", Error)
	}
	if Purple != lipgloss.Color(fallbackPalette["purple"]) {
		t.Errorf("Purple = %s, want gruvbox's %s", Purple, fallbackPalette["purple"])
	}
}

func TestAnsiIndicesAreAccepted(t *testing.T) {
	// A theme can defer to the terminal's own scheme, which is the whole
	// idea behind the "terminal" built-in.
	Use(Theme{Palette: map[string]string{"bg": "0", "fg": "7", "orange": "11"}})
	if Accent != lipgloss.Color("11") {
		t.Errorf("Accent = %s, want ANSI 11", Accent)
	}
}

func TestNonsenseValuesDoNotProduceBlankColours(t *testing.T) {
	Use(Theme{Palette: map[string]string{"bg": "not a colour", "orange": ""}})
	if Bg != lipgloss.Color(fallbackPalette["bg"]) {
		t.Errorf("Bg = %s, want the fallback", Bg)
	}
	if Accent == "" {
		t.Error("Accent came out blank")
	}
}

func TestEveryBuiltinThemeResolves(t *testing.T) {
	for _, name := range List() {
		th, err := Find(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		Use(th)
		for role := range roleVars {
			if *roleVars[role] == "" {
				t.Errorf("%s left role %q blank", name, role)
			}
		}
	}
}

func TestYourThemeShadowsTheBuiltin(t *testing.T) {
	configHome(t)
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"name":"gruvbox","palette":{"orange":"#123456"}}`)
	if err := os.WriteFile(filepath.Join(Dir(), "gruvbox.json"), body, 0644); err != nil {
		t.Fatal(err)
	}

	th, err := Find("gruvbox")
	if err != nil {
		t.Fatal(err)
	}
	if th.Palette["orange"] != "#123456" {
		t.Errorf("orange = %s, want the copy in the config directory to win", th.Palette["orange"])
	}
}

func TestSetRefusesAThemeThatIsntThere(t *testing.T) {
	configHome(t)
	if err := Set("nosuch"); err == nil {
		t.Error("Set accepted a theme that doesn't exist")
	}
	if Current() != DefaultName {
		t.Errorf("Current = %s, want the default to be untouched", Current())
	}
}

func TestSetAndCurrentRoundTrip(t *testing.T) {
	configHome(t)
	if err := Set("nord"); err != nil {
		t.Fatal(err)
	}
	if Current() != "nord" {
		t.Errorf("Current = %s, want nord", Current())
	}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestExportWritesEveryRole(t *testing.T) {
	body, err := Export("gruvbox")
	if err != nil {
		t.Fatal(err)
	}
	th, err := parse(body)
	if err != nil {
		t.Fatal(err)
	}
	for role := range roleVars {
		if _, ok := th.Roles[role]; !ok {
			t.Errorf("exported theme is missing role %q", role)
		}
	}
}
