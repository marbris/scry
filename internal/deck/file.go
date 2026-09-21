package deck

import (
	"scry/internal/paths"

	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A deck on disk. One `.deck` file per deck, line-oriented so that git diffs
// read the way the change did — adding a card is a one-line diff — and so the
// file is worth opening in an editor:
//
//	# a note to self, kept as-is
//	name: Ghen, Arcanum Weaver
//	format: commander
//	source: https://moxfield.com/decks/AbC123
//
//	[commander]
//	1 Ghen, Arcanum Weaver [wincon]
//
//	[mainboard]
//	1 Sol Ring (c21) 263 [ramp, artifact]
//	7 Plains
//
// Cards are named rather than held by Scryfall id, with an optional
// `(set) collector` pinning one printing — the Moxfield and Archidekt
// convention, so a file pastes back into either once the tags are stripped.
// Names resolve through the card cache (cardcache.go), not on every read.
//
// Rewriting a file is lossless for the leading comment block and for header
// keys this version doesn't know about, but not for comments further down:
// the card lines are regenerated in a canonical order so that two people —
// or the TUI and your editor — always produce the same bytes.

const FileExt = ".deck"

// Sections a deck file can carry. Anything else found in a file is kept and
// written back after these, so a hand-added section survives a rewrite.
var knownSections = []string{"commander", "mainboard", "sideboard", "maybeboard"}

type File struct {
	Name   string
	Format string
	Source string // the Moxfield URL it came from, if any

	// Notes is the comment block at the top of the file, "#" and all.
	Notes []string

	// Extra keeps header keys this version doesn't recognise, in the order
	// they were read, so hand-written fields aren't eaten on the next write.
	Extra [][2]string

	Entries []Entry
}

// Entry is one line of a deck file: a card, how many, which section it's
// in, and its tags. It names a card rather than resolving one — turning
// these into deckCards is Resolve' job.
type Entry struct {
	Qty       int
	Name      string
	Set       string // pinned printing, both empty when unpinned
	Collector string
	Tags      []string
	Section   string
}

func (e Entry) commander() bool { return e.Section == "commander" }

// ── Parsing ─────────────────────────────────────────────────────

// entryLineRe splits a card line into quantity, name, pinned printing and
// tags. The name is lazy and the trailing groups optional, so a card whose
// name really does end in brackets — "Erase (Not the Urza's Legacy One)" —
// keeps them: the printing group only matches a set-code-shaped token
// followed by a collector number.
var entryLineRe = regexp.MustCompile(
	`^(?:(\d+)\s+)?(.+?)(?:\s+\(([A-Za-z0-9]{3,5})\)\s+([^\s\[\]]+))?(?:\s+\[([^\]]*)\])?$`)

var sectionRe = regexp.MustCompile(`^\[([^\]]+)\]$`)

// ParseFile reads the format above. It is forgiving on the way in —
// a missing quantity means one, a card before any section header is in the
// mainboard — and strict only about things it cannot guess at.
func ParseFile(r io.Reader) (*File, error) {
	d := &File{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	section := "mainboard"
	inHeader := true // still above the first section
	leading := true  // still in the top comment block

	for line := 1; sc.Scan(); line++ {
		raw := strings.TrimRight(sc.Text(), " \t")
		text := strings.TrimSpace(raw)

		if text == "" {
			leading = false
			continue
		}

		if strings.HasPrefix(text, "#") {
			if leading {
				d.Notes = append(d.Notes, text)
			}
			continue
		}
		leading = false

		if m := sectionRe.FindStringSubmatch(text); m != nil {
			section = strings.ToLower(strings.TrimSpace(m[1]))
			if section == "" {
				return nil, fmt.Errorf("line %d: empty section name", line)
			}
			inHeader = false
			continue
		}

		// `key: value` above the first section is a header field. Only up
		// top: below a section header the same text is a card, and reading
		// one as a stray header would silently drop it from the deck.
		if inHeader {
			if k, v, ok := strings.Cut(text, ":"); ok && !strings.HasPrefix(text, "[") {
				key := strings.ToLower(strings.TrimSpace(k))
				val := strings.TrimSpace(v)
				switch key {
				case "name":
					d.Name = val
				case "format":
					d.Format = val
				case "source":
					d.Source = val
				default:
					d.Extra = append(d.Extra, [2]string{key, val})
				}
				continue
			}
			// Not a header field after all — fall through and read it as a
			// card in the default section.
			inHeader = false
		}

		e, err := parseEntry(text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		e.Section = section
		d.Entries = append(d.Entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return d, nil
}

func parseEntry(text string) (Entry, error) {
	m := entryLineRe.FindStringSubmatch(text)
	if m == nil {
		return Entry{}, fmt.Errorf("cannot read %q as a card", text)
	}

	e := Entry{Qty: 1, Name: strings.TrimSpace(m[2]), Set: strings.ToLower(m[3]), Collector: m[4]}
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			return Entry{}, fmt.Errorf("bad quantity %q", m[1])
		}
		e.Qty = n
	}
	if e.Name == "" {
		return Entry{}, fmt.Errorf("no card name in %q", text)
	}
	e.Tags = ParseTags(m[5])
	return e, nil
}

// parseTags splits and normalises a bracketed tag list. Tags are lowercased
// and de-duplicated so that tagging in bulk stays consistent however the tag
// was typed, and sorted so the line is canonical.
func ParseTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ── Serialising ─────────────────────────────────────────────────

// String renders the deck back to the file format. Output is canonical:
// sections in a fixed order, cards sorted by name inside each, tags sorted.
// Two writes of the same deck produce identical bytes, so git only ever sees
// the lines that actually changed.
func (d *File) String() string {
	var b strings.Builder

	for _, n := range d.Notes {
		b.WriteString(n + "\n")
	}
	if len(d.Notes) > 0 {
		b.WriteString("\n")
	}

	header := false
	write := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
			header = true
		}
	}
	write("name", d.Name)
	write("format", d.Format)
	write("source", d.Source)
	for _, kv := range d.Extra {
		write(kv[0], kv[1])
	}
	if header {
		b.WriteString("\n")
	}

	for _, sec := range d.sectionOrder() {
		entries := d.Section(sec)
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[%s]\n", sec)
		for _, e := range entries {
			b.WriteString(e.String() + "\n")
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func (e Entry) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s", e.Qty, e.Name)
	if e.Set != "" && e.Collector != "" {
		fmt.Fprintf(&b, " (%s) %s", e.Set, e.Collector)
	}
	if len(e.Tags) > 0 {
		fmt.Fprintf(&b, " [%s]", strings.Join(e.Tags, ", "))
	}
	return b.String()
}

// section returns one section's entries, sorted by name.
// Section is the entries filed under one heading.
func (d *File) Section(name string) []Entry {
	var out []Entry
	for _, e := range d.Entries {
		if e.Section == name {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sectionOrder lists the known sections first, then any others the file
// carried, alphabetically.
func (d *File) sectionOrder() []string {
	known := map[string]bool{}
	for _, s := range knownSections {
		known[s] = true
	}
	extra := map[string]bool{}
	for _, e := range d.Entries {
		if !known[e.Section] {
			extra[e.Section] = true
		}
	}
	rest := make([]string, 0, len(extra))
	for s := range extra {
		rest = append(rest, s)
	}
	sort.Strings(rest)
	return append(append([]string{}, knownSections...), rest...)
}

// ── Totals ──────────────────────────────────────────────────────

// counts totals the sections that make up the deck proper; a maybeboard is
// a shortlist, not part of it.
func (d *File) Counts() (total, unique int) {
	for _, e := range d.Entries {
		if e.Section == "maybeboard" {
			continue
		}
		total += e.Qty
		unique++
	}
	return total, unique
}

// ── The decks directory ─────────────────────────────────────────

// Dir is where deck files live, and in phase 3 the git repo tracking
// them. SCRY_DECKS_DIR moves it somewhere you'd rather back up.
func Dir() string {
	dir := os.Getenv("SCRY_DECKS_DIR")
	if dir == "" {
		dir = filepath.Join(paths.Data(), "decks")
	}
	os.MkdirAll(dir, 0755)
	return dir
}

// Path is where a deck's file lives. A slug may be a path — "aggro/mono-red" —
// so decks group into folders; filepath.Join turns the slug's forward slashes
// into whatever the OS uses.
func Path(slug string) string {
	return filepath.Join(Dir(), filepath.FromSlash(slug)+FileExt)
}

// List returns the slugs of the decks on disk, alphabetically. It walks the
// whole tree, so a slug is the deck's path under the decks directory with the
// extension trimmed and separators as forward slashes — "aggro/mono-red".
func List() ([]string, error) {
	root := Dir()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable entry is skipped, not fatal
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), FileExt) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(strings.TrimSuffix(rel, FileExt)))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// Folders returns the paths of every directory under the decks directory, so
// the tree can show a folder even when it holds no decks yet. Paths are relative
// with forward slashes, like slugs — "aggro", "aggro/mono-red".
func Folders() ([]string, error) {
	root := Dir()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// NewFolder makes an empty directory in the decks tree and returns the folder
// path it settled on. The name is slugified segment by segment, the same as a
// deck's, so a ".." or a leading slash can't escape the decks directory.
func NewFolder(name string) (string, error) {
	slug := Slugify(name)
	if slug == "" {
		return "", fmt.Errorf("%q doesn't make a usable folder name", name)
	}
	if err := os.MkdirAll(filepath.Join(Dir(), filepath.FromSlash(slug)), 0755); err != nil {
		return "", err
	}
	return slug, nil
}

func Exists(slug string) bool {
	_, err := os.Stat(Path(slug))
	return err == nil
}

func Read(slug string) (*File, error) {
	f, err := os.Open(Path(slug))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d, err := ParseFile(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", slug+FileExt, err)
	}
	d.Name = displayName(slug, d.Name)
	return d, nil
}

// displayName is a deck's name as it should be shown: the folder is the slug's
// directory, never part of the name. A header is kept as-is once its folder
// prefix — left over from when a nested deck's name carried its path — is
// stripped; a deck with no header names itself after its file's last segment.
func displayName(slug, header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return slugBase(slug)
	}
	if dir := slugDir(slug); dir != "" {
		return strings.TrimPrefix(header, dir+"/")
	}
	return header
}

// slugDir and slugBase split a slug's folder from its last segment. Slugs use
// forward slashes whatever the OS, so they go through path, not filepath.
func slugDir(slug string) string {
	if dir := path.Dir(slug); dir != "." {
		return dir
	}
	return ""
}

func slugBase(slug string) string { return path.Base(slug) }

// Write saves a deck, via a temporary file so that a crash mid-write
// can't leave a half-written deck behind — git will be reading these.
func Write(slug string, d *File) error {
	path := Path(slug)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	// The temp file's pattern can't carry the slug's own separators, so it is
	// named from the last segment only — the directory it lands in already
	// carries the rest.
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(slug)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(d.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func Delete(slug string) error {
	return os.Remove(Path(slug))
}

// DefaultFormat is what a deck is assumed to be when you don't say. This is
// a Commander tool first.
const DefaultFormat = "commander"

// New creates an empty deck and returns the name it was filed under.
// Empty is a legitimate state: you fill it by adding cards, and until then
// there's nothing to write but a header.
func New(name, format string) (slug string, d *File, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("a deck needs a name")
	}
	slug = Slugify(name)
	if slug == "" {
		return "", nil, fmt.Errorf("%q doesn't make a usable file name", name)
	}
	if Exists(slug) {
		return "", nil, fmt.Errorf("you already have a deck called %q", slug)
	}
	if format == "" {
		format = DefaultFormat
	}
	return slug, &File{Name: baseName(name), Format: format}, nil
}

// baseName is a deck's own name, without the folder a slash puts it in: the
// text after the last slash, trimmed. "Aggro / Mono Red" is the Mono Red deck
// filed under aggro, so its name is "Mono Red".
func baseName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSpace(name)
}
