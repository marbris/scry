package main

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// The list filter. Bubbles' default is a fuzzy one, which matches a term as
// a subsequence — typing "flying" in a deck matches "Go**l**dspan Dragon
// **F**..." and a dozen other cards that never mention flying, because the
// letters happen to appear in that order somewhere across the name and the
// oracle text. Over a hundred cards of rules text almost everything matches
// something, which makes the filter useless for the one thing it's most
// wanted for: narrowing a deck to the cards that actually say a word.
//
// So the filter is literal. Terms are whole substrings, all of them have to
// appear, and results are ordered by where the first one matched — name
// before rules text, early before late.

// literalFilter matches every space-separated term as a substring, case
// insensitively. A term in quotes may contain spaces.
func literalFilter(term string, targets []string) []list.Rank {
	terms := filterTerms(term)
	if len(terms) == 0 {
		return nil
	}

	var ranks []list.Rank
	for i, target := range targets {
		lower := strings.ToLower(target)

		matched := make([]int, 0, len(terms))
		ok := true
		for _, t := range terms {
			at := strings.Index(lower, t)
			if at < 0 {
				ok = false
				break
			}
			// The matched span, so the list can highlight it.
			for j := at; j < at+len(t); j++ {
				matched = append(matched, j)
			}
		}
		if !ok {
			continue
		}
		sort.Ints(matched)
		ranks = append(ranks, list.Rank{Index: i, MatchedIndexes: matched})
	}

	// Earliest match first, so a card with the word in its name comes above
	// one that only mentions it in a reminder clause.
	sort.SliceStable(ranks, func(a, b int) bool {
		return firstIndex(ranks[a]) < firstIndex(ranks[b])
	})
	return ranks
}

func firstIndex(r list.Rank) int {
	if len(r.MatchedIndexes) == 0 {
		return 1 << 30
	}
	return r.MatchedIndexes[0]
}

// filterTerms splits a filter into the substrings that all have to match.
// Quoting keeps a phrase together: `"first strike"` is one term, where
// first strike would be two.
func filterTerms(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}

	var terms []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				terms = append(terms, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		terms = append(terms, cur.String())
	}
	return terms
}
