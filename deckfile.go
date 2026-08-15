package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
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

const deckFileExt = ".deck"

// Sections a deck file can carry. Anything else found in a file is kept and
// written back after these, so a hand-added section survives a rewrite.
var knownSections = []string{"commander", "mainboard", "sideboard", "maybeboard"}

type deckFile struct {
	Name   string
	Format string
	Source string // the Moxfield URL it came from, if any

	// Notes is the comment block at the top of the file, "#" and all.
	Notes []string

	// Extra keeps header keys this version doesn't recognise, in the order
	// they were read, so hand-written fields aren't eaten on the next write.
	Extra [][2]string

	Entries []deckEntry
}

// deckEntry is one line of a deck file: a card, how many, which section it's
// in, and its tags. It names a card rather than resolving one — turning
// these into deckCards is resolveEntries' job.
type deckEntry struct {
	Qty       int
	Name      string
	Set       string // pinned printing, both empty when unpinned
	Collector string
	Tags      []string
	Section   string
}

func (e deckEntry) commander() bool { return e.Section == "commander" }

// ── Parsing ─────────────────────────────────────────────────────

// deckLineRe splits a card line into quantity, name, pinned printing and
// tags. The name is lazy and the trailing groups optional, so a card whose
// name really does end in brackets — "Erase (Not the Urza's Legacy One)" —
// keeps them: the printing group only matches a set-code-shaped token
// followed by a collector number.
var deckLineRe = regexp.MustCompile(
	`^(?:(\d+)\s+)?(.+?)(?:\s+\(([A-Za-z0-9]{3,5})\)\s+([^\s\[\]]+))?(?:\s+\[([^\]]*)\])?$`)

var deckSectionRe = regexp.MustCompile(`^\[([^\]]+)\]$`)

// parseDeckFile reads the format above. It is forgiving on the way in —
// a missing quantity means one, a card before any section header is in the
// mainboard — and strict only about things it cannot guess at.
func parseDeckFile(r io.Reader) (*deckFile, error) {
	d := &deckFile{}
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

		if m := deckSectionRe.FindStringSubmatch(text); m != nil {
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

		e, err := parseDeckLine(text)
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

func parseDeckLine(text string) (deckEntry, error) {
	m := deckLineRe.FindStringSubmatch(text)
	if m == nil {
		return deckEntry{}, fmt.Errorf("cannot read %q as a card", text)
	}

	e := deckEntry{Qty: 1, Name: strings.TrimSpace(m[2]), Set: strings.ToLower(m[3]), Collector: m[4]}
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			return deckEntry{}, fmt.Errorf("bad quantity %q", m[1])
		}
		e.Qty = n
	}
	if e.Name == "" {
		return deckEntry{}, fmt.Errorf("no card name in %q", text)
	}
	e.Tags = parseTags(m[5])
	return e, nil
}

// parseTags splits and normalises a bracketed tag list. Tags are lowercased
// and de-duplicated so that tagging in bulk stays consistent however the tag
// was typed, and sorted so the line is canonical.
func parseTags(s string) []string {
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
func (d *deckFile) String() string {
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
		entries := d.section(sec)
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

func (e deckEntry) String() string {
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
func (d *deckFile) section(name string) []deckEntry {
	var out []deckEntry
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
func (d *deckFile) sectionOrder() []string {
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
func (d *deckFile) counts() (total, unique int) {
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

// decksDir is where deck files live, and in phase 3 the git repo tracking
// them. SCRY_DECKS_DIR moves it somewhere you'd rather back up.
func decksDir() string {
	dir := os.Getenv("SCRY_DECKS_DIR")
	if dir == "" {
		dir = filepath.Join(dataDir(), "decks")
	}
	os.MkdirAll(dir, 0755)
	return dir
}

func deckFilePath(slug string) string {
	return filepath.Join(decksDir(), slug+deckFileExt)
}

// listDecks returns the slugs of the decks on disk, alphabetically.
func listDecks() ([]string, error) {
	entries, err := os.ReadDir(decksDir())
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), deckFileExt) {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), deckFileExt))
	}
	sort.Strings(out)
	return out, nil
}

func deckExists(slug string) bool {
	_, err := os.Stat(deckFilePath(slug))
	return err == nil
}

func readDeck(slug string) (*deckFile, error) {
	f, err := os.Open(deckFilePath(slug))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d, err := parseDeckFile(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", slug+deckFileExt, err)
	}
	if d.Name == "" {
		d.Name = slug
	}
	return d, nil
}

// writeDeck saves a deck, via a temporary file so that a crash mid-write
// can't leave a half-written deck behind — git will be reading these.
func writeDeck(slug string, d *deckFile) error {
	path := deckFilePath(slug)
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+slug+".*")
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

func deleteDeck(slug string) error {
	return os.Remove(deckFilePath(slug))
}
