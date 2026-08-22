package rules

// Keeping the comprehensive rules up to date.
//
// The rules change a few times a year, and each release is a fresh text file
// at a date-stamped URL — so the current link can't be hard-coded, it has to
// be found. Sync reads the rules page, works out whether what is offered there
// is newer than what is cached, and if so downloads it, keeping the version it
// replaced so the two can be compared.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"scry/internal/fetch"
	"scry/internal/paths"
)

// rulesPage lists the current download. The link on it is what we actually
// fetch; this page is the one thing that stays put across releases.
const rulesPage = "https://magic.wizards.com/en/rules"

// ErrNoLink is returned when the rules page can be read but no rules link is
// found on it — the one case worth handling specially, since the answer is to
// point the user at a manual download rather than to report a failure.
var ErrNoLink = errors.New("no rules link found on the rules page")

// linkRe finds the rules download on the page. The href may carry the space as
// %20 or a literal space depending on how the page is served, so both are
// allowed; the run of digits before .txt is the version.
var linkRe = regexp.MustCompile(`https://media\.wizards\.com/[^"'\s]*MagicCompRules(?:%20|\s)*(\d{8})\.txt`)

func PrevPath() string { return filepath.Join(paths.Cache(), "comprules.prev.txt") }
func MetaPath() string { return filepath.Join(paths.Cache(), "comprules.meta.json") }

// meta records which version is cached and which one it displaced, so a sync
// can tell whether a download is needed and a diff knows there is something to
// compare against.
type meta struct {
	Current  string `json:"current"`
	Previous string `json:"previous"`
}

func readMeta() meta {
	var m meta
	if b, err := os.ReadFile(MetaPath()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func writeMeta(m meta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(MetaPath(), b, 0644)
}

// versionRe is the eight-digit date immediately before .txt — anchored there
// so the %20 that escapes the space in the filename can't be read as digits.
var versionRe = regexp.MustCompile(`(\d{8})\.txt`)

// versionFromURL pulls the eight-digit date out of a rules link. Empty when
// there isn't one, which only happens for a hand-edited URL.
func versionFromURL(u string) string {
	if m := versionRe.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// LatestURL reads the rules page and returns the current download link and its
// version. ErrNoLink means the page loaded but held no link — download it by
// hand — while any other error is the network or the page being unreachable.
func LatestURL() (url, version string, err error) {
	body, err := fetch.Get(rulesPage)
	if err != nil {
		return "", "", err
	}
	m := linkRe.FindSubmatch(body)
	if m == nil {
		return "", "", ErrNoLink
	}
	return string(m[0]), string(m[1]), nil
}

// Result is the outcome of a sync: whether it changed anything and the two
// versions it moved between.
type Result struct {
	Changed bool
	From    string // the version replaced, empty on a first download
	To      string // the version now cached
}

// Sync brings the cached rules up to the current release. It downloads only
// when the page offers something newer than what is on disk, and when it does,
// it keeps the displaced file so a diff has a previous to compare with.
func Sync() (Result, error) {
	url, version, err := LatestURL()
	if err != nil {
		return Result{}, err
	}

	m := readMeta()
	_, statErr := os.Stat(FilePath())
	cached := statErr == nil

	// Versions are YYYYMMDD, so a string compare is a date compare. Nothing to
	// do when what's offered isn't newer than what we already hold.
	if cached && version != "" && version <= m.Current {
		return Result{To: m.Current}, nil
	}

	body, err := fetch.GetFile(url)
	if err != nil {
		return Result{}, err
	}

	// Keep the version being replaced as the previous, so gv can diff against
	// it. Only a real file counts — there is nothing to compare on a first run.
	from := ""
	if cached {
		from = m.Current
		if err := copyFile(FilePath(), PrevPath()); err != nil {
			return Result{}, err
		}
	}

	if err := os.WriteFile(FilePath(), body, 0644); err != nil {
		return Result{}, err
	}
	if err := writeMeta(meta{Current: version, Previous: from}); err != nil {
		return Result{}, err
	}

	return Result{Changed: true, From: from, To: version}, nil
}

// HasPrevious reports whether there is a displaced version to diff against.
func HasPrevious() bool {
	_, err := os.Stat(PrevPath())
	return err == nil
}

// DiffPrevious is the rule-by-rule diff from the previous cached rules to the
// current ones: which rules were added, removed or reworded.
func DiffPrevious() ([]RuleChange, error) {
	prev, err := readText(PrevPath())
	if err != nil {
		return nil, err
	}
	cur, err := readText(FilePath())
	if err != nil {
		return nil, err
	}
	return DiffRules(prev, cur), nil
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0644)
}
