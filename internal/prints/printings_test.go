package prints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"scry/internal/mtg"
)

// printingsServer serves one page of Scryfall printings.
func printingsServer(t *testing.T, cards []mtg.Card) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mtg.SearchResponse{Data: cards})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestPrintingsComeBackOldestFirst(t *testing.T) {
	// The whole point is reading a card's wording forwards through time.
	url := printingsServer(t, []mtg.Card{
		{Name: "Bird", Set: "ccc", SetName: "Third", ReleasedAt: "2005-01-01", Lang: "en"},
		{Name: "Bird", Set: "aaa", SetName: "First", ReleasedAt: "1993-08-05", Lang: "en"},
		{Name: "Bird", Set: "bbb", SetName: "Second", ReleasedAt: "1999-04-11", Lang: "en"},
	})

	got, err := Printings(url)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d printings", len(got))
	}
	if got[0].SetName != "First" || got[2].SetName != "Third" {
		t.Errorf("came back %s, %s, %s", got[0].SetName, got[1].SetName, got[2].SetName)
	}
}

func TestSetCodesAreUppercasedForMTGJSON(t *testing.T) {
	// Scryfall lowercases them and MTGJSON doesn't, and the set files are
	// fetched by that code.
	url := printingsServer(t, []mtg.Card{
		{Name: "Bird", Set: "lea", SetName: "Alpha", ReleasedAt: "1993-08-05", Lang: "en"},
	})
	got, _ := Printings(url)
	if len(got) != 1 || got[0].Set != "LEA" {
		t.Errorf("got %v", got)
	}
}

func TestDigitalAndTranslatedPrintingsAreNotPartOfThePaperHistory(t *testing.T) {
	url := printingsServer(t, []mtg.Card{
		{Name: "Bird", Set: "aaa", ReleasedAt: "1993-08-05", Lang: "en"},
		{Name: "Bird", Set: "bbb", ReleasedAt: "1994-01-01", Lang: "en", Digital: true},
		{Name: "Vogel", Set: "ccc", ReleasedAt: "1995-01-01", Lang: "de"},
		{Name: "Bird", Set: "", ReleasedAt: "1996-01-01", Lang: "en"},
	})

	got, err := Printings(url)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Set != "AAA" {
		t.Errorf("got %v, want only the paper English one", got)
	}
}

func TestOnePrintingPerSetIsEnough(t *testing.T) {
	// A set can list a card several times — different collector numbers,
	// borderless, promos — and they all say the same thing.
	url := printingsServer(t, []mtg.Card{
		{Name: "Bird", Set: "aaa", ReleasedAt: "1993-08-05", Lang: "en", CollectorNumber: "1"},
		{Name: "Bird", Set: "aaa", ReleasedAt: "1993-08-05", Lang: "en", CollectorNumber: "2"},
		{Name: "Bird", Set: "AAA", ReleasedAt: "1993-08-05", Lang: "en", CollectorNumber: "3"},
	})

	got, _ := Printings(url)
	if len(got) != 1 {
		t.Errorf("got %d printings of one card in one set", len(got))
	}
}

func TestACardWithNoPrintingsLinkIsAnError(t *testing.T) {
	if _, err := Printings(""); err == nil {
		t.Error("no link came back as success")
	}
}

func TestBuildRevisionsCollapsesIdenticalWordings(t *testing.T) {
	// A card reprinted twenty times with the same text is one entry.
	card := mtg.Card{Name: "Bird", OracleText: "Flying"}
	printings := []Printing{
		{Name: "Bird", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
		{Name: "Bird", Set: "BBB", SetName: "Beta", Released: "1994-01-01"},
		{Name: "Bird", Set: "CCC", SetName: "Third", Released: "1999-01-01"},
	}
	originals := map[string]map[string]string{
		"AAA": {"bird": "Does not tap when attacking."},
		"BBB": {"bird": "Does not tap when attacking."},
		"CCC": {"bird": "Vigilance"},
	}

	revs := BuildRevisions(card, printings, originals)
	if len(revs) != 3 {
		t.Fatalf("got %d revisions:\n%+v", len(revs), revs)
	}
	if revs[0].Printings != 2 {
		t.Errorf("the first wording covers %d printings, want 2", revs[0].Printings)
	}
	if revs[0].SetName != "Alpha" {
		t.Errorf("a wording is dated by where it first appeared, got %s", revs[0].SetName)
	}
	if !revs[len(revs)-1].Current {
		t.Error("the last revision is not marked as the current wording")
	}
}

func TestTheCurrentWordingIsAlwaysTheLastRevision(t *testing.T) {
	// Whether or not any printing carries it verbatim — oracle text changes
	// without a reprint.
	card := mtg.Card{Name: "Bird", OracleText: "Flying"}
	revs := BuildRevisions(card, []Printing{
		{Name: "Bird", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
	}, map[string]map[string]string{
		"AAA": {"bird": "Does not tap when attacking."},
	})

	if len(revs) != 2 {
		t.Fatalf("got %d revisions", len(revs))
	}
	if !revs[1].Current || revs[1].Text != "Flying" {
		t.Errorf("the current wording is %+v", revs[1])
	}
}

func TestEncodingDifferencesAreNotWordingChanges(t *testing.T) {
	// MTGJSON writes the tap symbol as "ocT" and a line break as "//".
	// Treating that as a different wording would show a revision on every
	// single printing.
	card := mtg.Card{Name: "Elf", OracleText: "{T}: Add {G}.\nSomething else."}
	revs := BuildRevisions(card, []Printing{
		{Name: "Elf", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
	}, map[string]map[string]string{
		"AAA": {"elf": "ocT: Add {G}. // Something else."},
	})

	if len(revs) != 1 {
		t.Errorf("got %d revisions, want the encodings treated as the same text:\n%+v",
			len(revs), revs)
	}
}

func TestASplitCardIsFoundUnderEitherFace(t *testing.T) {
	// MTGJSON sometimes files one under just its front half.
	card := mtg.Card{Name: "Fire // Ice", OracleText: "Fire deals 2 damage."}
	revs := BuildRevisions(card, []Printing{
		{Name: "Fire // Ice", Set: "AAA", SetName: "Alpha", Released: "1999-01-01"},
	}, map[string]map[string]string{
		"AAA": {"fire": "Fire deals 2 damage to any target."},
	})

	if len(revs) < 2 {
		t.Errorf("the earlier wording was not found:\n%+v", revs)
	}
}

func TestPrintingsWithNoRecordedTextAreSkipped(t *testing.T) {
	card := mtg.Card{Name: "Bird", OracleText: "Flying"}
	revs := BuildRevisions(card, []Printing{
		{Name: "Bird", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
	}, map[string]map[string]string{"AAA": {}})

	if len(revs) != 1 || !revs[0].Current {
		t.Errorf("got %+v, want just the current wording", revs)
	}
}

func TestTheSetCacheRoundTrips(t *testing.T) {
	// What is on disk is what gets shown without asking; the rest is only
	// offered. So whether a set counts as cached is a user-visible answer.
	if IsCached("ZZZ") {
		t.Fatal("a set nobody has fetched reports as cached")
	}

	writeCache("ZZZ", map[string]string{"test bird": "Flying"})
	if !IsCached("ZZZ") {
		t.Error("a set just written does not report as cached")
	}

	got, ok := readCache("ZZZ")
	if !ok {
		t.Fatal("it could not be read back")
	}
	if got["test bird"] != "Flying" {
		t.Errorf("read back %v", got)
	}
}

func TestAnEmptySetIsStillCached(t *testing.T) {
	// A set Scryfall knows and MTGJSON doesn't is written empty precisely so
	// it isn't asked for again.
	writeCache("EMPTY", map[string]string{})
	if !IsCached("EMPTY") {
		t.Error("an empty set will be fetched again every time")
	}
	if _, ok := readCache("EMPTY"); !ok {
		t.Error("it could not be read back")
	}
}

func TestACorruptCacheEntryReadsAsAbsent(t *testing.T) {
	// Rather than as an error: everything here can be fetched again.
	if err := os.WriteFile(cachePath("BAD"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := readCache("BAD"); ok {
		t.Error("a corrupt file was read as good data")
	}
}
