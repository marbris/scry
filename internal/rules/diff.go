package rules

import "sort"

// A diff of two releases of the rules, rule by rule rather than line by line.
//
// A line diff of the comprehensive rules is useless: the file reflows, rules
// renumber, and the whole thing reads as changed. What a reader actually wants
// is "which rules are different, and how" — so both releases are parsed and
// compared by rule number, and each rule that appeared, vanished or had its
// wording change comes out as one entry.

type ChangeKind int

const (
	Changed ChangeKind = iota // same number, different text
	Added                     // a number that wasn't there before
	Removed                   // a number that is gone
)

// RuleChange is one rule that differs between the two releases.
type RuleChange struct {
	Number string
	Kind   ChangeKind
	Old    string // the previous text; empty for an Added rule
	New    string // the current text; empty for a Removed rule
}

// DiffRules compares two rules texts and returns the rules that differ, in
// rule-number order. A rule whose text is unchanged is left out.
func DiffRules(oldText, newText string) []RuleChange {
	old := ruleText(Parse(oldText))
	nw := ruleText(Parse(newText))

	seen := map[string]bool{}
	var nums []string
	for n := range old {
		seen[n] = true
		nums = append(nums, n)
	}
	for n := range nw {
		if !seen[n] {
			nums = append(nums, n)
		}
	}
	sort.Slice(nums, func(i, j int) bool { return lessRuleNumber(nums[i], nums[j]) })

	var out []RuleChange
	for _, n := range nums {
		o, hadOld := old[n]
		w, hasNew := nw[n]
		switch {
		case hadOld && hasNew:
			if o != w {
				out = append(out, RuleChange{Number: n, Kind: Changed, Old: o, New: w})
			}
		case hasNew:
			out = append(out, RuleChange{Number: n, Kind: Added, New: w})
		default:
			out = append(out, RuleChange{Number: n, Kind: Removed, Old: o})
		}
	}
	return out
}

// ruleText indexes a parsed release by rule number, which is what the diff
// compares — the number is the identity, the text is what may have moved under
// it.
func ruleText(d Data) map[string]string {
	m := make(map[string]string, len(d.Rules))
	for _, r := range d.Rules {
		m[r.Number] = r.Text
	}
	return m
}

// lessRuleNumber orders 100.1a before 100.2 before 101.1, which a plain string
// compare does not: "100.10" would sort before "100.2".
func lessRuleNumber(a, b string) bool {
	as, bs := splitRuleNumber(a), splitRuleNumber(b)
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		if nx, ny := leadingNumber(x), leadingNumber(y); nx != ny {
			return nx < ny
		}
		if x != y {
			return x < y
		}
	}
	return len(as) < len(bs)
}

func splitRuleNumber(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

func leadingNumber(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
