package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points the state directory somewhere temporary, so a test never
// reads or writes the query history of whoever is running it.
func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	return root
}

func writeString(path, body string) error {
	return os.WriteFile(path, []byte(body), 0644)
}

func TestRememberQuery(t *testing.T) {
	var h []string
	h = RememberQuery(h, "t:dragon")
	h = RememberQuery(h, "t:angel")
	if strings.Join(h, "|") != "t:dragon|t:angel" {
		t.Errorf("history = %v, want oldest first", h)
	}

	// Running an old search again moves it to the end rather than adding a
	// second copy, so walking back never steps through the same query twice.
	h = RememberQuery(h, "t:dragon")
	if strings.Join(h, "|") != "t:angel|t:dragon" {
		t.Errorf("history = %v, want the repeat moved to the end", h)
	}

	// Blank searches aren't searches.
	h = RememberQuery(h, "   ")
	if len(h) != 2 {
		t.Errorf("a blank query was remembered: %v", h)
	}

	// It doesn't grow without limit.
	for i := 0; i < queryHistoryMax*2; i++ {
		h = RememberQuery(h, string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	if len(h) > queryHistoryMax {
		t.Errorf("history grew to %d, cap is %d", len(h), queryHistoryMax)
	}
}

func TestQueryHistorySurvivesTheSession(t *testing.T) {
	isolate(t)

	if got := LoadQueryHistory(); got != nil {
		t.Errorf("a fresh install has history: %v", got)
	}
	if err := SaveQueryHistory([]string{"t:dragon", "t:angel"}); err != nil {
		t.Fatal(err)
	}
	if got := LoadQueryHistory(); strings.Join(got, "|") != "t:dragon|t:angel" {
		t.Errorf("history did not come back: %v", got)
	}

	// Rubbish on disk is no history rather than a crash.
	if err := writeString(queryHistoryPath(), "{not json"); err != nil {
		t.Fatal(err)
	}
	if got := LoadQueryHistory(); got != nil {
		t.Errorf("a corrupt history file loaded as %v", got)
	}
}
