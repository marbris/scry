package deck

import (
	"sort"
	"strings"
)

// applyTagEdits returns the tags with the additions and removals applied,
// normalised the way the file stores them.
func ApplyTagEdits(tags, add, remove []string) []string {
	set := map[string]bool{}
	for _, t := range tags {
		set[t] = true
	}
	for _, t := range add {
		set[t] = true
	}
	for _, t := range remove {
		delete(set, t)
	}
	if len(set) == 0 {
		return nil
	}

	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ── Moving tags between lists ───────────────────────────────────
//
// These are joins on the card's name. Tags only ever arrive: nothing here
// takes a tag off a card.

func nameKey(name string) string { return strings.ToLower(name) }

// TransferTags gives the cards in dst the tags the same cards carry in src
// — an UPDATE … JOIN. With addMissing, the cards of src that dst lacks are
// added too, one copy each with their tags: an upsert, for growing a tag
// list. It returns the new list, how many cards in it gained a tag, and how
// many were added.
func TransferTags(dst, src []Card, addMissing bool) (out []Card, tagged, added int) {
	out = append([]Card(nil), dst...)
	at := map[string]int{}
	for i, c := range out {
		at[nameKey(c.Card.Name)] = i
	}
	for _, s := range src {
		i, ok := at[nameKey(s.Card.Name)]
		if !ok {
			if !addMissing {
				continue
			}
			out = append(out, Card{Card: s.Card, Qty: 1, Tags: append([]string(nil), s.Tags...)})
			at[nameKey(s.Card.Name)] = len(out) - 1
			added++
			continue
		}
		before := out[i].Tags
		out[i].Tags = ApplyTagEdits(before, s.Tags, nil)
		if len(out[i].Tags) != len(before) {
			tagged++
		}
	}
	return out, tagged, added
}

// MergeTags gives every card, in every list, the tags that card has in any
// of them — a union across the lists, one card at a time. The lists come
// back in the order they went in, with how many cards in each gained a tag.
func MergeTags(lists ...[]Card) (out [][]Card, tagged []int) {
	all := map[string][]string{}
	for _, l := range lists {
		for _, c := range l {
			k := nameKey(c.Card.Name)
			all[k] = ApplyTagEdits(all[k], c.Tags, nil)
		}
	}
	for _, l := range lists {
		next := append([]Card(nil), l...)
		n := 0
		for i, c := range next {
			merged := ApplyTagEdits(c.Tags, all[nameKey(c.Card.Name)], nil)
			if len(merged) != len(c.Tags) {
				n++
			}
			next[i].Tags = merged
		}
		out = append(out, next)
		tagged = append(tagged, n)
	}
	return out, tagged
}
