package ui

import (
	"reflect"
	"strings"
	"testing"

	"ttr/internal/mtg"
)

func TestPrintingFactsSplitPrintingFromWorth(t *testing.T) {
	c := mtg.Card{SetName: "Universes Within", Rarity: "rare", EDHRECRank: 10619,
		Prices: mtg.Prices{USD: "2.53"}}
	want := []string{"Universes Within · rare", "edhrec #10619 · $2.53"}
	if got := printingFacts(c); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintingFactsLeaveOutWhatACardLacks(t *testing.T) {
	// No price at all is left out, not shown as a dash; a foil-only price
	// still counts.
	cases := []struct {
		card mtg.Card
		want []string
	}{
		{mtg.Card{SetName: "Alpha", EDHRECRank: 5}, []string{"Alpha", "edhrec #5"}},
		{mtg.Card{Rarity: "rare"}, []string{"rare"}},
		{mtg.Card{Prices: mtg.Prices{USDFoil: "10"}}, []string{"$10.00"}},
		{mtg.Card{}, nil},
	}
	for _, tc := range cases {
		if got := printingFacts(tc.card); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%+v: got %q, want %q", tc.card, got, tc.want)
		}
	}
}

func TestStatsRowSitsFlushRight(t *testing.T) {
	row := stripANSI(statsRow(mtg.Card{Power: "7", Toughness: "4"}, 20))
	if row != strings.Repeat(" ", 17)+"7/4" {
		t.Errorf("got %q", row)
	}
	if statsRow(mtg.Card{TypeLine: "Instant"}, 20) != "" {
		t.Error("an instant has no stats row")
	}
}
