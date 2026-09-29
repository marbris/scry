package prints

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"ttr/internal/fetch"
	"ttr/internal/mtg"
)

// The printing gx shows.
//
// "The card" as a picture is a question with a hundred answers for a staple,
// and most of them are not what you picture: promos, borderless showcases,
// Secret Lair oddities. What you picture is the card as it was most recently
// printed in an ordinary set — so that is the one chosen, and the newest
// printing of any kind only when there is no ordinary one to be had.

// ordinarySets are the kinds of set whose printings look like the card.
var ordinarySets = map[string]bool{
	"core": true, "expansion": true, "commander": true, "masters": true,
	"draft_innovation": true, "starter": true,
}

// oddSets are sets whose type says ordinary but whose printings aren't: The
// List's reprints carry a mark in the corner, and the Mystery Boosters are
// playtest oddities and reprints of every kind.
var oddSets = map[string]bool{
	"plst": true, "mb1": true, "mb2": true, "fmb1": true, "pmei": true,
}

// specialFrames are the treatments that make a printing not the card as you
// picture it.
var specialFrames = map[string]bool{
	"showcase": true, "extendedart": true, "etched": true, "inverted": true,
	"textless": true, "fullart": true,
}

// Ordinary reports whether a printing looks like the card: in paper, in
// English, in an ordinary set, with an ordinary frame.
func Ordinary(c mtg.Card) bool {
	if c.Digital || (c.Lang != "" && c.Lang != "en") {
		return false
	}
	if c.Promo || c.FullArt || c.Variation {
		return false
	}
	if c.BorderColor == "borderless" || c.BorderColor == "gold" || c.BorderColor == "silver" {
		return false
	}
	for _, f := range c.FrameEffects {
		if specialFrames[f] {
			return false
		}
	}
	return ordinarySets[c.SetType] && !oddSets[c.Set]
}

// Prefer picks from printings listed newest first: the first ordinary one,
// or else the first one with a picture at all. A printing from a set not
// out yet by today — "2006-01-02" — is a preview, not the card as printed,
// and is passed over while there is anything else.
func Prefer(printings []mtg.Card, today string) (mtg.Card, bool) {
	out := func(c mtg.Card) bool { return c.ReleasedAt == "" || c.ReleasedAt <= today }
	for _, want := range []func(mtg.Card) bool{
		func(c mtg.Card) bool { return out(c) && Ordinary(c) },
		out,
		func(mtg.Card) bool { return true },
	} {
		for _, c := range printings {
			if want(c) && c.Image("normal") != "" {
				return c, true
			}
		}
	}
	return mtg.Card{}, false
}

// printsQuery is the search for every printing of a card, newest first: by
// its oracle id where it has one, which is exact, otherwise by exact name.
func printsQuery(c mtg.Card) string {
	q := fmt.Sprintf("!%q", c.Name)
	if c.OracleID != "" {
		q = "oracleid:" + c.OracleID
	}
	v := url.Values{}
	v.Set("q", q+" unique:prints")
	v.Set("order", "released")
	v.Set("dir", "desc")
	return "https://api.scryfall.com/cards/search?" + v.Encode()
}

// Preferred finds the printing gx should show for a card. A couple of pages
// is plenty: the newest printings come first, and an ordinary one turns up
// long before the old ones would.
func Preferred(c mtg.Card) (mtg.Card, error) {
	today := time.Now().Format("2006-01-02")
	var all []mtg.Card
	for page, next := 0, printsQuery(c); next != "" && page < 2; page++ {
		body, err := fetch.Get(next)
		if err != nil {
			return mtg.Card{}, err
		}
		var sr mtg.SearchResponse
		if err := json.Unmarshal(body, &sr); err != nil {
			return mtg.Card{}, err
		}
		all = append(all, sr.Data...)
		if best, ok := Prefer(all, today); ok && Ordinary(best) {
			return best, nil
		}
		if !sr.HasMore {
			break
		}
		next = sr.NextPage
	}
	if best, ok := Prefer(all, today); ok {
		return best, nil
	}
	return mtg.Card{}, fmt.Errorf("no picture of %s", c.Name)
}
