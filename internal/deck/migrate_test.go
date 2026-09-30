package deck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeckFilesBecomeListsWithTheirHistory(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir := Dir()
	if err := ensureRepo(); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"ghen.deck", "aggro/mono-red.deck"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte("name: X\n[mainboard]\n1 Sol Ring\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-A")
	git("commit", "-q", "-m", "seed")
	t.Cleanup(func() { Delete("ghen"); Delete("aggro/mono-red") })

	n, err := MigrateExt()
	if err != nil || n != 2 {
		t.Fatalf("renamed %d, err %v", n, err)
	}
	if !Exists("ghen") || !Exists("aggro/mono-red") {
		t.Error("the lists aren't there")
	}
	if _, err := os.Stat(filepath.Join(dir, "ghen.deck")); err == nil {
		t.Error("the .deck is still there")
	}
	log, _ := git("log", "-1", "--format=%s")
	if log != "Rename deck files to .list" {
		t.Errorf("last commit is %q", log)
	}
	status, _ := git("status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Errorf("left uncommitted: %s", status)
	}

	if n, _ := MigrateExt(); n != 0 {
		t.Errorf("second run renamed %d", n)
	}
}
