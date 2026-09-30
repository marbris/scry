package deck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// oldFileExt is what list files were called before they held tag lists as
// well as decks.
const oldFileExt = ".deck"

// MigrateExt renames every .deck file in the decks directory to .list, once.
// It uses git mv where it can, so each file's history follows it, and records
// the renames in one commit. A .list already at the new name wins, and the
// .deck is left for you to look at. It returns how many files it renamed.
func MigrateExt() (int, error) {
	root := Dir()
	var olds []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), oldFileExt) {
			if rel, err := filepath.Rel(root, path); err == nil {
				olds = append(olds, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if len(olds) == 0 {
		return 0, nil
	}

	useGit := false
	if GitAvailable() {
		_, err := git("rev-parse", "--git-dir")
		useGit = err == nil
	}

	var staged []string
	n := 0
	for _, rel := range olds {
		to := strings.TrimSuffix(rel, oldFileExt) + FileExt
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(to))); err == nil {
			continue
		}
		if useGit {
			if _, err := git("mv", rel, to); err == nil {
				staged = append(staged, rel, to)
				n++
				continue
			}
		}
		if err := os.Rename(filepath.Join(root, filepath.FromSlash(rel)), filepath.Join(root, filepath.FromSlash(to))); err != nil {
			return n, err
		}
		n++
	}
	if len(staged) > 0 {
		args := []string{"commit", "-q", "-m", "Rename deck files to .list", "--"}
		// Committing fails outright with no identity; fall back to the one
		// ensureRepo would have given a repository ttr made itself.
		if out, err := git("config", "user.email"); err != nil || strings.TrimSpace(out) == "" {
			args = append([]string{"-c", "user.name=ttr", "-c", "user.email=ttr@localhost"}, args...)
		}
		args = append(args, staged...)
		if _, err := git(args...); err != nil {
			return n, err
		}
	}
	return n, nil
}
