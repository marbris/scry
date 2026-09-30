package deck

import (
	"strings"
	"testing"

	"ttr/internal/mtg"
)

func card(name string, tags ...string) Card {
	return Card{Qty: 1, Card: mtg.Card{Name: name}, Tags: tags}
}

func tagsOf(cards []Card) string {
	var out []string
	for _, c := range cards {
		out = append(out, c.Card.Name+"["+strings.Join(c.Tags, ",")+"]")
	}
	return strings.Join(out, " ")
}

func TestTransferTagsOnlyTagsWhatIsThere(t *testing.T) {
	dst := []Card{card("Sol Ring"), card("Forest", "land")}
	src := []Card{card("sol ring", "ramp"), card("Forest", "basic"), card("Rancor", "aura")}
	out, tagged, added := TransferTags(dst, src, false)
	if got := tagsOf(out); got != "Sol Ring[ramp] Forest[basic,land]" {
		t.Errorf("got %s", got)
	}
	if tagged != 2 || added != 0 {
		t.Errorf("tagged %d added %d", tagged, added)
	}
	if len(dst[0].Tags) != 0 {
		t.Error("changed the list it was given")
	}
}

func TestTransferTagsCanAddWhatIsMissing(t *testing.T) {
	dst := []Card{card("Sol Ring", "ramp")}
	src := []Card{card("Sol Ring", "ramp"), {Qty: 4, Card: mtg.Card{Name: "Rancor"}, Tags: []string{"aura"}}}
	out, tagged, added := TransferTags(dst, src, true)
	if got := tagsOf(out); got != "Sol Ring[ramp] Rancor[aura]" {
		t.Errorf("got %s", got)
	}
	if tagged != 0 || added != 1 || out[1].Qty != 1 {
		t.Errorf("tagged %d added %d qty %d", tagged, added, out[1].Qty)
	}
}

func TestMergeTagsUnionsEveryList(t *testing.T) {
	a := []Card{card("Sol Ring", "ramp"), card("Forest")}
	b := []Card{card("Sol Ring", "rock"), card("Rancor", "aura")}
	c := []Card{card("Rancor"), card("Forest", "land")}
	out, tagged := MergeTags(a, b, c)
	want := []string{"Sol Ring[ramp,rock] Forest[land]", "Sol Ring[ramp,rock] Rancor[aura]", "Rancor[aura] Forest[land]"}
	for i := range want {
		if got := tagsOf(out[i]); got != want[i] {
			t.Errorf("list %d: got %s, want %s", i, got, want[i])
		}
	}
	if tagged[0] != 2 || tagged[1] != 1 || tagged[2] != 1 {
		t.Errorf("tagged %v", tagged)
	}
}
