package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/scryfall"
)

func TestOtagNamesAreWhatWasTyped(t *testing.T) {
	got := otagNames("Removal  otag:ramp otag-draw removal")
	want := []string{"removal", "ramp", "draw"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestOtagQueriesQuoteEachNameAndStayShort(t *testing.T) {
	one := otagQueries("removal", []string{"Sol Ring", "Fire // Ice"})
	if len(one) != 1 || one[0] != `otag:removal (!"Sol Ring" or !"Fire // Ice")` {
		t.Errorf("got %q", one)
	}

	var names []string
	for i := 0; i < 200; i++ {
		names = append(names, "Some Fairly Long Card Name "+itoa(i))
	}
	qs := otagQueries("ramp", names)
	if len(qs) < 2 {
		t.Fatalf("200 names went in %d query", len(qs))
	}
	n := 0
	for _, q := range qs {
		if len(q) > otagQueryLen {
			t.Errorf("query is %d long", len(q))
		}
		n += strings.Count(q, "!\"")
	}
	if n != 200 {
		t.Errorf("%d names asked about, want 200", n)
	}
}

func TestATwoFacedCardIsFoundByEitherName(t *testing.T) {
	if !sameCard("Fire // Ice", "Fire // Ice") || !sameCard("Delver of Secrets", "Delver of Secrets // Insectile Aberration") {
		t.Error("didn't match")
	}
	if sameCard("Fire", "Fireball") {
		t.Error("matched a different card")
	}
}

func TestTabInTheAddBarTagsTheDeckByOtag(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		asked = append(asked, q)
		var sr mtg.SearchResponse
		if strings.HasPrefix(q, "otag:ramp ") {
			sr.Data = []mtg.Card{{Name: "Sol Ring"}, {Name: "Llanowar Elves"}}
			sr.TotalCards = 2
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(sr)
	}))
	defer srv.Close()
	old := scryfall.SearchURL
	scryfall.SearchURL = srv.URL
	t.Cleanup(func() { scryfall.SearchURL = old })

	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })
	m, l := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", sample())

	m = drive(m, "i")
	if m.ws.current().asking != askAddCard {
		t.Fatal("i didn't open the add bar")
	}
	m = drive(m, "tab")
	if m.ws.current().asking != askOtag {
		t.Fatal("tab didn't turn it to otag")
	}
	for _, r := range "ramp removal" {
		m = drive(m, string(r))
	}
	m, cmd := press(m, "enter")
	next, _ := m.Update(msgOf(cmd))
	m = next.(Model)

	if len(asked) != 2 {
		t.Errorf("asked %d questions, want one per tag: %q", len(asked), asked)
	}
	tagged := map[string]bool{}
	for _, c := range l.all {
		for _, tg := range c.Tags {
			if tg == "otag-ramp" {
				tagged[c.Card.Name] = true
			}
			if tg == "otag-removal" {
				t.Errorf("%s tagged removal", c.Card.Name)
			}
		}
	}
	if !tagged["Sol Ring"] || !tagged["Llanowar Elves"] || len(tagged) != 2 {
		t.Errorf("tagged %v", tagged)
	}
	if !strings.Contains(m.notice, "otag-ramp: 2 of 4") {
		t.Errorf("notice %q", m.notice)
	}
}
