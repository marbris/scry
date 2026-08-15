package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Deck files name their cards instead of holding Scryfall ids, which keeps
// them readable and diffable but means every open would otherwise be a
// round trip to Scryfall before anything could be drawn. So resolved cards
// are kept in one file beside the rules cache, keyed by what the deck line
// asked for. After a deck's first load it opens instantly, and offline.
//
// The cache is a convenience, never a source of truth: a card that isn't in
// it is fetched, and a cache that won't parse is discarded rather than
// reported, because the only cost is one more request.

const cardCacheFile = "cards.json"

type cardCache struct {
	cards map[string]ScryfallCard
	dirty bool
}

func cardCachePath() string {
	return filepath.Join(dataDir(), cardCacheFile)
}

// entryKey is what a deck line resolves by: a pinned printing if it has one,
// otherwise just the name. Lowercased, so "sol ring" and "Sol Ring" share an
// entry.
func entryKey(e deckEntry) string {
	if e.Set != "" && e.Collector != "" {
		return strings.ToLower(e.Set + "/" + e.Collector)
	}
	return strings.ToLower(e.Name)
}

// searchableName is the name to ask Scryfall for. Split and double-faced
// cards are printed — and written in decklists — as "Fire // Ice", but the
// collection endpoint doesn't match that: it wants one face, and answers
// with the whole card under its full name. Asking for the front half is the
// form that works for both kinds of name.
func searchableName(name string) string {
	if front, _, ok := strings.Cut(name, " // "); ok {
		return front
	}
	return name
}

func loadCardCache() *cardCache {
	c := &cardCache{cards: map[string]ScryfallCard{}}

	body, err := os.ReadFile(cardCachePath())
	if err != nil {
		return c
	}
	// A cache written by a different version, or half-written, is simply
	// dropped — everything in it can be fetched again.
	if err := json.Unmarshal(body, &c.cards); err != nil {
		c.cards = map[string]ScryfallCard{}
	}
	return c
}

func (c *cardCache) get(e deckEntry) (ScryfallCard, bool) {
	card, ok := c.cards[entryKey(e)]
	return card, ok
}

func (c *cardCache) put(e deckEntry, card ScryfallCard) {
	c.cards[entryKey(e)] = card
	c.dirty = true
}

// save writes the cache back, atomically, and only when something changed.
func (c *cardCache) save() error {
	if !c.dirty {
		return nil
	}
	body, err := json.Marshal(c.cards)
	if err != nil {
		return err
	}

	path := cardCachePath()
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+cardCacheFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	c.dirty = false
	return nil
}

// ── Resolving ───────────────────────────────────────────────────

// unresolvedError lists the deck lines Scryfall didn't recognise. The deck
// still opens — the cards it did resolve are worth showing — so this is
// reported alongside the result rather than instead of it.
type unresolvedError struct{ names []string }

func (e unresolvedError) Error() string {
	if len(e.names) == 1 {
		return fmt.Sprintf("no card named %q", e.names[0])
	}
	return fmt.Sprintf("%d cards not found: %s", len(e.names), strings.Join(e.names, ", "))
}

// resolveEntries turns deck lines into cards, taking what it can from the
// cache and fetching the rest in one batch. Entries that resolve are
// returned even when others don't; the misses come back as an
// unresolvedError so the caller can show both.
func resolveEntries(entries []deckEntry) ([]deckCard, error) {
	cache := loadCardCache()

	// Only the entries the cache can't answer go to Scryfall, and each
	// distinct key is asked for once however many lines want it.
	var idents []map[string]string
	askedFor := map[string]bool{}
	for _, e := range entries {
		key := entryKey(e)
		if _, ok := cache.get(e); ok || askedFor[key] {
			continue
		}
		askedFor[key] = true
		if e.Set != "" && e.Collector != "" {
			idents = append(idents, map[string]string{"set": e.Set, "collector_number": e.Collector})
		} else {
			idents = append(idents, map[string]string{"name": searchableName(e.Name)})
		}
	}

	// A fetch that fails isn't fatal: the cache may still hold most of the
	// deck, and a deck that opens on a train missing its two newest cards
	// beats one that won't open at all. The error is kept and reported only
	// if something actually went unresolved because of it.
	var fetchErr error
	if len(idents) > 0 {
		found, _, err := fetchIdentifiers(idents)
		if err != nil {
			fetchErr = err
		} else {
			index := indexCards(found)
			for _, e := range entries {
				if _, ok := cache.get(e); ok {
					continue
				}
				if card, ok := index.lookup(e); ok {
					cache.put(e, card)
				}
			}
			// A failed write costs nothing but the next load's speed.
			_ = cache.save()
		}
	}

	cards := make([]deckCard, 0, len(entries))
	var missing []string
	for _, e := range entries {
		card, ok := cache.get(e)
		if !ok {
			missing = append(missing, e.Name)
			continue
		}
		cards = append(cards, deckCard{
			card:      card,
			qty:       e.Qty,
			commander: e.commander(),
			tags:      e.Tags,
		})
	}

	if len(missing) > 0 {
		// If the lookup never happened, say so — "no card named X" would be
		// a lie when the truth is that Scryfall was unreachable.
		if fetchErr != nil {
			return cards, fetchErr
		}
		sort.Strings(missing)
		return cards, unresolvedError{names: missing}
	}
	return cards, nil
}

// cardIndex matches fetched cards back to the entries that asked for them.
// Scryfall answers a batch in its own order, and a name lookup comes back
// under the card's full name, so a request for "Fire" returns "Fire // Ice".
type cardIndex struct {
	byName     map[string]ScryfallCard
	byPrinting map[string]ScryfallCard
}

func indexCards(cards []ScryfallCard) cardIndex {
	idx := cardIndex{
		byName:     make(map[string]ScryfallCard, len(cards)),
		byPrinting: make(map[string]ScryfallCard, len(cards)),
	}
	for _, c := range cards {
		name := strings.ToLower(c.Name)
		idx.byName[name] = c
		// Double-faced cards are named "Front // Back"; a deck that asks
		// for either half should still find them.
		if front, back, ok := strings.Cut(name, " // "); ok {
			if _, taken := idx.byName[front]; !taken {
				idx.byName[front] = c
			}
			if _, taken := idx.byName[back]; !taken {
				idx.byName[back] = c
			}
		}
		if c.Set != "" {
			idx.byPrinting[strings.ToLower(c.Set+"/"+c.CollectorNumber)] = c
		}
	}
	return idx
}

func (idx cardIndex) lookup(e deckEntry) (ScryfallCard, bool) {
	if e.Set != "" && e.Collector != "" {
		c, ok := idx.byPrinting[strings.ToLower(e.Set+"/"+e.Collector)]
		return c, ok
	}
	c, ok := idx.byName[strings.ToLower(e.Name)]
	return c, ok
}
