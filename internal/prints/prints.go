// Package prints is a card's printed-text history.
//
// Scryfall only ever serves a card's *current* oracle text — its printed_text
// field is populated for non-English cards only, and there is no oracle
// revision history in the API at all. MTGJSON does carry the wording as
// printed, per printing, in its originalText field, so that's where this
// comes from.
//
// MTGJSON only publishes whole sets, so a heavily reprinted card means one
// file per set it appeared in. Each set is distilled down to a name -> text
// map and cached on disk, and nothing is fetched until the user asks for it.
package prints

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"scry/internal/fetch"
	"scry/internal/mtg"
	"scry/internal/paths"
)

// sem keeps set downloads to a civilised number at a time.
var sem = make(chan struct{}, 4)

// Printing is one set a card appeared in.
type Printing struct {
	Name     string
	Set      string // upper-case, as MTGJSON names them
	SetName  string
	Released string // YYYY-MM-DD
}

// TextRevision is one distinct wording, and the printing it first appeared on.
type TextRevision struct {
	Text      string
	SetCode   string
	SetName   string
	Released  string
	Printings int  // how many printings share this wording
	Current   bool // whether this is the wording in force today
}

// Printings lists every paper English printing of a card, oldest first.
func Printings(uri string) ([]Printing, error) {
	if uri == "" {
		return nil, fmt.Errorf("no printings link for this card")
	}

	var out []Printing
	seen := make(map[string]bool)

	for page, next := 0, uri; next != "" && page < 6; page++ {
		body, err := fetch.Get(next)
		if err != nil {
			if _, ok := err.(fetch.NotFound); ok {
				break
			}
			return nil, err
		}

		var sr mtg.SearchResponse
		if err := json.Unmarshal(body, &sr); err != nil {
			return nil, err
		}

		for _, c := range sr.Data {
			// Digital-only and translated printings aren't part of the
			// paper wording history.
			if c.Digital || (c.Lang != "" && c.Lang != "en") || c.Set == "" {
				continue
			}
			set := strings.ToUpper(c.Set)
			if seen[set] {
				continue // one printing per set is enough to read its text
			}
			seen[set] = true
			out = append(out, Printing{
				Name:     c.Name,
				Set:      set,
				SetName:  c.SetName,
				Released: c.ReleasedAt,
			})
		}

		if !sr.HasMore {
			break
		}
		next = sr.NextPage
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Released < out[j].Released })
	return out, nil
}

// SetOriginals pulls one set's printed text, from disk if it's already been
// distilled, otherwise from MTGJSON — and caches it on the way past.
func SetOriginals(set string) (map[string]string, error) {
	if cards, ok := readCache(set); ok {
		return cards, nil
	}

	sem <- struct{}{}
	defer func() { <-sem }()

	// Another card may have fetched it while we waited for a slot.
	if cards, ok := readCache(set); ok {
		return cards, nil
	}

	cards, err := download(set)
	if err != nil {
		return nil, err
	}
	writeCache(set, cards)
	return cards, nil
}

func download(set string) (map[string]string, error) {
	u := fmt.Sprintf("https://mtgjson.com/api/v5/%s.json.gz", set)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fetch.UserAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Sets Scryfall knows and MTGJSON doesn't are cached as empty so we
	// don't ask again.
	if resp.StatusCode == 404 {
		return map[string]string{}, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("mtgjson (%d) for %s", resp.StatusCode, set)
	}

	// Decoded as a stream: a large set is tens of megabytes decompressed,
	// and only two fields per card are wanted.
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var payload struct {
		Data struct {
			Cards []struct {
				Name         string `json:"name"`
				OriginalText string `json:"originalText"`
			} `json:"cards"`
		} `json:"data"`
	}
	if err := json.NewDecoder(zr).Decode(&payload); err != nil {
		return nil, err
	}

	cards := make(map[string]string)
	for _, c := range payload.Data.Cards {
		if c.OriginalText == "" {
			continue
		}
		key := strings.ToLower(c.Name)
		if _, dup := cards[key]; !dup {
			cards[key] = c.OriginalText
		}
	}
	return cards, nil
}

// ── Disk cache ──────────────────────────────────────────────────

func cacheDir() string {
	dir := filepath.Join(paths.Data(), "originals")
	os.MkdirAll(dir, 0755)
	return dir
}

func cachePath(set string) string { return filepath.Join(cacheDir(), set+".json") }

func readCache(set string) (map[string]string, bool) {
	body, err := os.ReadFile(cachePath(set))
	if err != nil {
		return nil, false
	}
	var cards map[string]string
	if err := json.Unmarshal(body, &cards); err != nil {
		return nil, false
	}
	return cards, true
}

func writeCache(set string, cards map[string]string) {
	body, err := json.Marshal(cards)
	if err != nil {
		return
	}
	os.WriteFile(cachePath(set), body, 0644)
}

// IsCached reports whether a set's text is already on disk, which is what
// tells an instant answer from one that needs the network.
func IsCached(set string) bool {
	_, err := os.Stat(cachePath(set))
	return err == nil
}

// ── Building the history ────────────────────────────────────────

// BuildRevisions collapses the printings into one entry per distinct
// wording — a card reprinted twenty times with the same text is one entry.
func BuildRevisions(c mtg.Card, printings []Printing, originals map[string]map[string]string) []TextRevision {
	var revs []TextRevision

	for _, p := range printings {
		text := cleanOriginal(lookupOriginal(originals[p.Set], p.Name))
		if text == "" {
			continue
		}
		if n := len(revs); n > 0 && sameText(revs[n-1].Text, text) {
			revs[n-1].Printings++
			continue
		}
		revs = append(revs, TextRevision{
			Text:      text,
			SetCode:   p.Set,
			SetName:   p.SetName,
			Released:  p.Released,
			Printings: 1,
		})
	}

	// The current oracle wording is the last revision, whether or not any
	// printing carries it verbatim.
	current := c.CombinedOracle()
	if n := len(revs); n > 0 && sameText(revs[n-1].Text, current) {
		revs[n-1].Current = true
	} else if current != "" {
		revs = append(revs, TextRevision{Text: current, Current: true})
	}

	return revs
}

// cleanOriginal converts MTGJSON's Gatherer-derived encoding into the same
// shape as oracle text. "ocT" is how the old tap symbol comes through, and
// "//" stands in for a line break. Normalising before comparing keeps a mere
// change of encoding from looking like a change of wording.
func cleanOriginal(s string) string {
	s = strings.ReplaceAll(s, "ocT", "{T}")
	s = originalBreakRe.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}

// "//" stands in for a line break, with or without spaces around it.
var originalBreakRe = regexp.MustCompile(`[ \t]*//[ \t]*`)

func lookupOriginal(cards map[string]string, name string) string {
	if cards == nil {
		return ""
	}
	if text, ok := cards[strings.ToLower(name)]; ok {
		return text
	}
	// Split and double-faced cards are sometimes filed under one face.
	if i := strings.Index(name, " // "); i > 0 {
		if text, ok := cards[strings.ToLower(name[:i])]; ok {
			return text
		}
	}
	return ""
}

// sameText compares wording while ignoring how it was laid out.
func sameText(a, b string) bool { return normalizeText(a) == normalizeText(b) }

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "//", "\n")
	return strings.Join(strings.Fields(s), " ")
}
