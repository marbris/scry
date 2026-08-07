package main

// Oracle-text syntax highlighting.
//
// There is no language server for Magic card text, so this is a small
// tokenizer instead: mana/tap symbols, reminder text, loyalty costs, P/T
// modifiers and the card's own name are matched with regexps, while the
// keyword abilities (702.x), keyword actions (701.x) and ability words
// (207.2c) come straight out of the comprehensive rules — no hand-kept
// word list to fall behind the next set.

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

type runeStyle struct {
	col    lipgloss.Color
	bold   bool
	italic bool
	set    bool
}

var (
	plainStyle    = runeStyle{col: gruvFg, set: true}
	reminderStyle = runeStyle{col: gruvGray, italic: true, set: true}
	abilityStyle  = runeStyle{col: gruvYellow, bold: true, set: true}
	actionStyle   = runeStyle{col: gruvAqua, set: true}
	wordStyle     = runeStyle{col: gruvPurple, italic: true, set: true}
	loyaltyStyle  = runeStyle{col: gruvOrange, bold: true, set: true}
	buffStyle     = runeStyle{col: gruvGreen, set: true}
	debuffStyle   = runeStyle{col: gruvRed, set: true}
)

var (
	reminderRe = regexp.MustCompile(`\([^)]*\)?`)
	symbolRe   = regexp.MustCompile(`\{[^}]{1,12}\}`)
	loyaltyRe  = regexp.MustCompile(`(?m)^[+\x{2212}-]?[0-9X]+:`)
	ptRe       = regexp.MustCompile(`[+\x{2212}-][0-9X]+/[+\x{2212}-][0-9X]+`)
)

// highlightOracle styles a card's oracle text and wraps it to width.
// The result is fully styled — no outer Foreground should be applied to
// it, or the nested resets will strip the colors partway through.
func highlightOracle(c ScryfallCard, width int, rules RulesData) string {
	text := c.OracleText
	if text == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}

	mask := buildMask(text, c, rules)

	var b strings.Builder
	lineStart := 0
	for i := 0; i <= len(text); i++ {
		if i < len(text) && text[i] != '\n' {
			continue
		}
		if lineStart > 0 {
			b.WriteString("\n")
		}
		for j, r := range wrapRanges(text[lineStart:i], width) {
			if j > 0 {
				b.WriteString("\n")
			}
			b.WriteString(emit(text, mask, lineStart+r[0], lineStart+r[1]))
		}
		lineStart = i + 1
	}
	return b.String()
}

// highlightRuleText applies the same treatment to rules text, which has
// no card name and no loyalty costs but plenty of symbols and keywords.
func highlightRuleText(text string, width int, rules RulesData) string {
	return highlightOracle(ScryfallCard{OracleText: text}, width, rules)
}

func buildMask(text string, c ScryfallCard, rules RulesData) []runeStyle {
	mask := make([]runeStyle, len(text))

	// fill claims a byte range unless it overlaps something already styled,
	// which is what gives the patterns below their priority order.
	fill := func(start, end int, st runeStyle) {
		if start < 0 || end > len(text) || start >= end {
			return
		}
		for i := start; i < end; i++ {
			if mask[i].set {
				return
			}
		}
		for i := start; i < end; i++ {
			mask[i] = st
		}
	}

	// 1. Reminder text swallows everything inside its parens.
	for _, loc := range reminderRe.FindAllStringIndex(text, -1) {
		fill(loc[0], loc[1], reminderStyle)
	}

	// 2. Mana, tap and other symbols.
	for _, loc := range symbolRe.FindAllStringIndex(text, -1) {
		inner := text[loc[0]+1 : loc[1]-1]
		fill(loc[0], loc[1], runeStyle{col: symbolColor(inner), bold: true, set: true})
	}

	// 3. Loyalty ability costs at the start of a line.
	for _, loc := range loyaltyRe.FindAllStringIndex(text, -1) {
		fill(loc[0], loc[1], loyaltyStyle)
	}

	// 4. The card's own name, plus its short form ("Dragonlord Ojutai" -> "Ojutai").
	nameStyle := runeStyle{col: colorForCard(c.Colors), bold: true, set: true}
	for _, name := range selfNames(c.Name) {
		for _, sp := range scan(literalRe(name), text) {
			fill(sp.start, sp.end, nameStyle)
		}
	}

	// 5. Power/toughness modifiers and counters.
	for _, loc := range ptRe.FindAllStringIndex(text, -1) {
		st := debuffStyle
		if text[loc[0]] == '+' {
			st = buffStyle
		}
		fill(loc[0], loc[1], st)
	}

	// 6. Keywords straight from the comprehensive rules.
	if rules.loaded() {
		for _, sp := range scan(rules.keywordRe, text) {
			kw, ok := rules.keywords[strings.ToLower(sp.text)]
			if !ok {
				continue
			}
			switch kw.Kind {
			case kwAbility:
				fill(sp.start, sp.end, abilityStyle)
			case kwAction:
				fill(sp.start, sp.end, actionStyle)
			case kwWord:
				fill(sp.start, sp.end, wordStyle)
			}
		}
	}

	return mask
}

// selfNames returns the forms of its own name a card may refer to itself by.
func selfNames(name string) []string {
	if name == "" {
		return nil
	}
	// Split cards: "Fire // Ice"
	parts := strings.Split(name, " // ")
	out := make([]string, 0, len(parts)*2)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
		// Legendary short name: text after the comma of "Ojutai, Soul of Winter"
		// is a title; the part before it is the name used in rules text.
		if i := strings.Index(p, ","); i > 0 {
			if short := strings.TrimSpace(p[:i]); short != "" && short != p {
				out = append(out, short)
			}
		}
	}
	return out
}

var literalReCache = map[string]*regexp.Regexp{}

func literalRe(s string) *regexp.Regexp {
	if re, ok := literalReCache[s]; ok {
		return re
	}
	re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(s))
	if err != nil {
		return nil
	}
	literalReCache[s] = re
	return re
}

func symbolColor(inner string) lipgloss.Color {
	upper := strings.ToUpper(inner)

	switch upper {
	case "T", "Q":
		return gruvOrange
	case "E":
		return gruvAqua
	case "S":
		return gruvBlue
	case "C", "X", "Y", "Z":
		return gruvFgDim
	}

	colors := map[rune]lipgloss.Color{
		'W': lipgloss.Color("#fbf1c7"),
		'U': gruvBlue,
		'B': gruvPurple,
		'R': gruvRed,
		'G': gruvGreen,
	}

	found := lipgloss.Color("")
	count := 0
	for _, r := range upper {
		if col, ok := colors[r]; ok {
			count++
			found = col
		}
	}
	switch {
	case count == 1:
		return found
	case count > 1:
		return gruvYellow // hybrid
	}
	return gruvFgDim // generic / numeric
}

// ── Wrapping ────────────────────────────────────────────────────

// wrapRanges greedily word-wraps s to width and returns the byte ranges
// of each output line, so the caller can keep its per-byte styling.
func wrapRanges(s string, width int) [][2]int {
	if width < 1 {
		width = 1
	}

	words := splitWords(s, width)
	if len(words) == 0 {
		return [][2]int{{0, 0}}
	}

	var out [][2]int
	start, end, lineW := words[0][0], words[0][1], runeLen(s[words[0][0]:words[0][1]])

	for _, w := range words[1:] {
		wLen := runeLen(s[w[0]:w[1]])
		if lineW+1+wLen > width {
			out = append(out, [2]int{start, end})
			start, end, lineW = w[0], w[1], wLen
			continue
		}
		end = w[1]
		lineW += 1 + wLen
	}
	out = append(out, [2]int{start, end})
	return out
}

// splitWords returns the byte ranges of whitespace-separated words,
// hard-breaking any word too long to ever fit on a line.
func splitWords(s string, width int) [][2]int {
	var out [][2]int
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}

		if runeLen(s[start:i]) <= width {
			out = append(out, [2]int{start, i})
			continue
		}
		// Too long: chop it into width-sized pieces on rune boundaries.
		chunkStart, chunkLen := start, 0
		for pos := start; pos < i; {
			_, size := utf8.DecodeRuneInString(s[pos:])
			pos += size
			chunkLen++
			if chunkLen == width || pos == i {
				out = append(out, [2]int{chunkStart, pos})
				chunkStart, chunkLen = pos, 0
			}
		}
	}
	return out
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// emit renders text[start:end) as runs of equally styled bytes.
func emit(text string, mask []runeStyle, start, end int) string {
	if start >= end {
		return ""
	}

	var b strings.Builder
	runStart := start
	cur := styleAt(mask, start)

	for i := start + 1; i <= end; i++ {
		var next runeStyle
		if i < end {
			next = styleAt(mask, i)
		}
		if i == end || next != cur {
			b.WriteString(renderRun(text[runStart:i], cur))
			runStart = i
			cur = next
		}
	}
	return b.String()
}

func styleAt(mask []runeStyle, i int) runeStyle {
	if i < len(mask) && mask[i].set {
		return mask[i]
	}
	return plainStyle
}

func renderRun(s string, st runeStyle) string {
	return lipgloss.NewStyle().
		Foreground(st.col).
		Bold(st.bold).
		Italic(st.italic).
		Render(s)
}
