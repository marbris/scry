package prints

import (
	"testing"

	"ttr/internal/mtg"
)

func printing(set, setType string, edit func(*mtg.Card)) mtg.Card {
	c := mtg.Card{
		Name: "Sol Ring", Set: set, SetType: setType, Lang: "en", BorderColor: "black",
		ImageURIs: mtg.ImageURIs{Normal: "https://img/" + set + ".jpg"},
	}
	if edit != nil {
		edit(&c)
	}
	return c
}

func TestPreferSkipsTheSpecialPrintingsForTheNewestOrdinaryOne(t *testing.T) {
	newestFirst := []mtg.Card{
		printing("sld", "box", nil),
		printing("pw25", "promo", func(c *mtg.Card) { c.Promo = true }),
		printing("dsc", "commander", func(c *mtg.Card) { c.BorderColor = "borderless" }),
		printing("fdn", "core", func(c *mtg.Card) { c.FrameEffects = []string{"showcase"} }),
		printing("ltc", "commander", nil),
		printing("cmm", "masters", nil),
	}
	got, ok := Prefer(newestFirst, "2026-01-01")
	if !ok || got.Set != "ltc" {
		t.Errorf("chose %q, want the newest ordinary printing, ltc", got.Set)
	}
}

func TestPreferFallsBackToTheNewestWithAPicture(t *testing.T) {
	newestFirst := []mtg.Card{
		printing("none", "promo", func(c *mtg.Card) { c.ImageURIs = mtg.ImageURIs{} }),
		printing("sld", "box", nil),
		printing("pw25", "promo", func(c *mtg.Card) { c.Promo = true }),
	}
	got, ok := Prefer(newestFirst, "2026-01-01")
	if !ok || got.Set != "sld" {
		t.Errorf("chose %q, want sld", got.Set)
	}
}

func TestADoubleFacedCardShowsItsFront(t *testing.T) {
	c := mtg.Card{CardFaces: []mtg.Face{
		{Name: "Front", ImageURIs: mtg.ImageURIs{Normal: "front.jpg"}},
		{Name: "Back", ImageURIs: mtg.ImageURIs{Normal: "back.jpg"}},
	}}
	if got := c.Image("normal"); got != "front.jpg" {
		t.Errorf("image is %q", got)
	}
}

func TestPrintsQueryAsksNewestFirst(t *testing.T) {
	got := printsQuery(mtg.Card{Name: "Sol Ring", OracleID: "abc"})
	want := "https://api.scryfall.com/cards/search?dir=desc&order=released&q=oracleid%3Aabc+unique%3Aprints"
	if got != want {
		t.Errorf("got %s", got)
	}
}

func TestPreferPassesOverSetsNotOutYetAndTheList(t *testing.T) {
	newestFirst := []mtg.Card{
		printing("next", "expansion", func(c *mtg.Card) { c.ReleasedAt = "2026-10-02" }),
		printing("plst", "masters", func(c *mtg.Card) { c.ReleasedAt = "2026-05-01" }),
		printing("cmm", "masters", func(c *mtg.Card) { c.ReleasedAt = "2023-08-04" }),
	}
	got, ok := Prefer(newestFirst, "2026-09-29")
	if !ok || got.Set != "cmm" {
		t.Errorf("chose %q, want cmm", got.Set)
	}
}

func TestPreferPassesOverRetroFrames(t *testing.T) {
	newestFirst := []mtg.Card{
		printing("inr", "masters", func(c *mtg.Card) { c.Frame = "1997" }),
		printing("avr", "expansion", func(c *mtg.Card) { c.Frame = "2015" }),
	}
	if got, _ := Prefer(newestFirst, "2026-09-29"); got.Set != "avr" {
		t.Errorf("chose %q, want the modern frame", got.Set)
	}
}
