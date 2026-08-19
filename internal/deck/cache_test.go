package deck

import (
	"scry/internal/mtg"

	"scry/internal/scryfall"

	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// seedCache writes a card cache under a temporary HOME so a test can resolve
// without touching the network.
func seedCache(t *testing.T, cards map[string]mtg.Card) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	body, err := json.Marshal(cards)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CachePath(), body, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveEntriesFromCache(t *testing.T) {
	seedCache(t, map[string]mtg.Card{
		"sol ring":             {Name: "Sol Ring", TypeLine: "Artifact", Set: "c21", CollectorNumber: "263"},
		"ghen, arcanum weaver": {Name: "Ghen, Arcanum Weaver", TypeLine: "Legendary Creature"},
		"c21/263":              {Name: "Sol Ring", TypeLine: "Artifact", Set: "c21", CollectorNumber: "263"},
	})

	entries := []Entry{
		{Qty: 1, Name: "Ghen, Arcanum Weaver", Section: "commander", Tags: []string{"wincon"}},
		{Qty: 1, Name: "Sol Ring", Section: "mainboard", Tags: []string{"ramp"}},
		{Qty: 1, Name: "Sol Ring", Set: "c21", Collector: "263", Section: "mainboard"},
	}

	// Everything is cached, so this must resolve with no network at all —
	// if it reaches out, the test fails or hangs rather than passing quietly.
	cards, err := Resolve(entries)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("got %d cards, want 3", len(cards))
	}

	if !cards[0].Commander {
		t.Error("the commander section didn't survive resolution")
	}
	if cards[1].Commander {
		t.Error("a mainboard card came back as a commander")
	}
	if got := cards[1].Tags; len(got) != 1 || got[0] != "ramp" {
		t.Errorf("tags = %v, want [ramp]", got)
	}
	if cards[1].Qty != 1 {
		t.Errorf("qty = %d", cards[1].Qty)
	}
}

func TestResolveEntriesSurvivesAnUnreachableScryfall(t *testing.T) {
	seedCache(t, map[string]mtg.Card{
		"sol ring": {Name: "Sol Ring", TypeLine: "Artifact"},
	})

	// A deck holding one cached card and one that would need fetching. With
	// the lookup unreachable the cached card must still come back, and the
	// error must describe the real problem rather than claiming the card
	// doesn't exist.
	defer func(u string) { scryfall.CollectionURL = u }(scryfall.CollectionURL)
	scryfall.CollectionURL = "http://127.0.0.1:1/nothing-listening"

	cards, err := Resolve([]Entry{
		{Qty: 1, Name: "Sol Ring", Section: "mainboard"},
		{Qty: 1, Name: "Smothering Tithe", Section: "mainboard"},
	})

	if len(cards) != 1 || cards[0].Card.Name != "Sol Ring" {
		t.Fatalf("the cached card should still open the deck, got %+v", cards)
	}
	if err == nil {
		t.Fatal("the unreachable lookup should have been reported")
	}
	if _, isUnresolved := err.(UnresolvedError); isUnresolved {
		t.Errorf("reported as a missing card when the truth is a network failure: %v", err)
	}
}

func TestCardCacheSurvivesRubbishOnDisk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(CachePath(), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	// A cache that won't parse is dropped, not fatal — everything in it can
	// be fetched again.
	c := loadCardCache()
	if c == nil || len(c.cards) != 0 {
		t.Fatalf("a corrupt cache should load as an empty one, got %+v", c)
	}
}

func TestCardCacheWritesOnlyWhenChanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c := loadCardCache()
	if err := c.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(CachePath()); !os.IsNotExist(err) {
		t.Error("an unchanged cache should not have been written at all")
	}

	c.put(Entry{Name: "Sol Ring"}, mtg.Card{Name: "Sol Ring"})
	if err := c.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(CachePath()); err != nil {
		t.Fatalf("a changed cache should have been written: %v", err)
	}

	// And it must come back.
	again := loadCardCache()
	if card, ok := again.get(Entry{Name: "sol ring"}); !ok || card.Name != "Sol Ring" {
		t.Errorf("cache did not survive a save/load: %+v %v", card, ok)
	}

	// No temporary files left behind by the atomic write.
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(CachePath()), ".cards.json.*"))
	if len(files) > 0 {
		t.Errorf("atomic write left temporary files: %v", files)
	}
}

func TestSearchableName(t *testing.T) {
	// Scryfall's collection endpoint matches one face, not the printed
	// "Fire // Ice" — see searchableName.
	for in, want := range map[string]string{
		"Fire // Ice":          "Fire",
		"Sol Ring":             "Sol Ring",
		"Ghen, Arcanum Weaver": "Ghen, Arcanum Weaver",
	} {
		if got := searchableName(in); got != want {
			t.Errorf("searchableName(%q) = %q, want %q", in, got, want)
		}
	}
}
