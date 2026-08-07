package main

// Comprehensive Rules engine — merged in from the mtg-rules project.
// Parses the official rules text, indexes keyword abilities/actions and
// glossary terms, and matches them against a card's oracle text.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ── Rules data ──────────────────────────────────────────────────

type Rule struct {
	Number   string // e.g. "100.1a"
	Text     string // full text of the rule
	Children []int  // indices of sub-rules
	Parent   int    // index of parent rule, -1 if none
	Depth    int    // 0 = section, 1 = category, 2 = rule, 3 = sub-rule
}

type GlossaryEntry struct {
	Term       string
	Definition string
}

type kwKind int

const (
	kwAbility kwKind = iota // 702.x — keyword abilities (flying, trample…)
	kwAction                // 701.x — keyword actions (destroy, scry…)
	kwWord                  // 207.2c — ability words (landfall, metalcraft…)
)

type Keyword struct {
	Name string
	Rule string
	Kind kwKind
}

type RulesData struct {
	Rules    []Rule
	Glossary []GlossaryEntry
	Index    map[string]int // rule number -> index

	keywords   map[string]Keyword       // lowercased name -> keyword
	keywordRe  *regexp.Regexp           // alternation of every keyword name
	glossary   map[string]GlossaryEntry // lowercased term -> entry
	glossaryRe *regexp.Regexp           // alternation of matchable terms
	typeRules  map[string]string        // card type -> category rule number
}

func (d RulesData) loaded() bool { return len(d.Rules) > 0 }

// ── Parsing ─────────────────────────────────────────────────────

var (
	sectionRe  = regexp.MustCompile(`^(\d)\. (.+)$`)
	categoryRe = regexp.MustCompile(`^(\d{3})\. (.+)$`)
	ruleRe     = regexp.MustCompile(`^(\d{3}\.\d+[a-z]*)\.?\s(.*)$`)
	subRuleRe  = regexp.MustCompile(`[a-z]$`)
	suffixRe   = regexp.MustCompile(`[a-z]+$`)
)

func parseRules(text string) RulesData {
	lines := strings.Split(text, "\n")
	data := RulesData{
		Index: make(map[string]int),
	}

	rulesStart := -1
	glossaryStart := -1
	creditsStart := -1

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "1. Game Concepts" {
			for j := i + 1; j < len(lines) && j < i+20; j++ {
				if ruleRe.MatchString(strings.TrimSpace(lines[j])) {
					rulesStart = i
					break
				}
			}
		}
		if line == "Glossary" {
			glossaryStart = i
		}
		if line == "Credits" {
			creditsStart = i
		}
	}

	if rulesStart == -1 {
		return data
	}
	if creditsStart == -1 {
		creditsStart = len(lines)
	}

	endOfRules := creditsStart
	if glossaryStart > 0 {
		endOfRules = glossaryStart
	}

	for i := rulesStart; i < endOfRules; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Section: "1. Game Concepts"
		if m := sectionRe.FindStringSubmatch(line); m != nil && len(m[1]) == 1 {
			idx := len(data.Rules)
			data.Rules = append(data.Rules, Rule{
				Number: m[1],
				Text:   m[2],
				Parent: -1,
				Depth:  0,
			})
			data.Index[m[1]] = idx
			continue
		}

		// Category: "100. General"
		if m := categoryRe.FindStringSubmatch(line); m != nil {
			idx := len(data.Rules)
			parentIdx := findParentSection(m[1], &data)
			data.Rules = append(data.Rules, Rule{
				Number: m[1],
				Text:   m[2],
				Parent: parentIdx,
				Depth:  1,
			})
			if parentIdx >= 0 {
				data.Rules[parentIdx].Children = append(data.Rules[parentIdx].Children, idx)
			}
			data.Index[m[1]] = idx
			continue
		}

		// Rule: "100.1. text" or "100.1a text"
		if m := ruleRe.FindStringSubmatch(line); m != nil {
			ruleNum := m[1]
			ruleText := m[2]

			// Collect continuation lines
			for i+1 < endOfRules {
				next := strings.TrimSpace(lines[i+1])
				if next == "" {
					break
				}
				if ruleRe.MatchString(next) || sectionRe.MatchString(next) || categoryRe.MatchString(next) {
					break
				}
				i++
				ruleText += " " + next
			}

			depth := 2
			if subRuleRe.MatchString(ruleNum) {
				depth = 3
			}

			parentIdx := findParentRule(ruleNum, &data)
			idx := len(data.Rules)
			data.Rules = append(data.Rules, Rule{
				Number: ruleNum,
				Text:   ruleText,
				Parent: parentIdx,
				Depth:  depth,
			})
			if parentIdx >= 0 {
				data.Rules[parentIdx].Children = append(data.Rules[parentIdx].Children, idx)
			}
			data.Index[ruleNum] = idx
			continue
		}
	}

	// Glossary
	if glossaryStart > 0 {
		var currentTerm string
		var currentDef strings.Builder

		for i := glossaryStart + 1; i < creditsStart; i++ {
			parseGlossaryLine(strings.TrimSpace(lines[i]), &data, &currentTerm, &currentDef)
		}
		if currentTerm != "" {
			data.Glossary = append(data.Glossary, GlossaryEntry{
				Term:       currentTerm,
				Definition: strings.TrimSpace(currentDef.String()),
			})
		}
	}

	buildKeywordIndex(&data)
	buildGlossaryIndex(&data)
	buildTypeIndex(&data)

	return data
}

func parseGlossaryLine(line string, data *RulesData, currentTerm *string, currentDef *strings.Builder) {
	if line == "" {
		if *currentTerm != "" {
			data.Glossary = append(data.Glossary, GlossaryEntry{
				Term:       *currentTerm,
				Definition: strings.TrimSpace(currentDef.String()),
			})
			*currentTerm = ""
			currentDef.Reset()
		}
		return
	}

	if *currentTerm == "" {
		*currentTerm = line
	} else {
		if currentDef.Len() > 0 {
			currentDef.WriteString(" ")
		}
		currentDef.WriteString(line)
	}
}

func findParentSection(catNum string, data *RulesData) int {
	if len(catNum) >= 1 {
		if idx, ok := data.Index[string(catNum[0])]; ok {
			return idx
		}
	}
	return -1
}

func findParentRule(ruleNum string, data *RulesData) int {
	// "702.9a" -> parent is "702.9"
	if suffixRe.MatchString(ruleNum) {
		parent := suffixRe.ReplaceAllString(ruleNum, "")
		if idx, ok := data.Index[parent]; ok {
			return idx
		}
	}

	// "702.9" -> parent is "702"
	parts := strings.Split(ruleNum, ".")
	if len(parts) >= 2 {
		if idx, ok := data.Index[parts[0]]; ok {
			return idx
		}
	}

	return -1
}

// ── Keyword / glossary indexes ──────────────────────────────────

var (
	// Keyword names: letters, digits, apostrophes, spaces, hyphens, "!"
	kwNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9’' !-]*$`)
	// Glossary terms worth matching against card text
	glossTermRe = regexp.MustCompile(`^[A-Za-z][A-Za-z’' -]*$`)
	// "See rule 702.9" / "See rule 207.2c"
	seeRuleRe = regexp.MustCompile(`[Ss]ee rule (\d{1,3}(?:\.\d+[a-z]*)?)`)
)

func buildKeywordIndex(d *RulesData) {
	d.keywords = make(map[string]Keyword)

	add := func(name, rule string, kind kwKind) {
		name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "."))
		if len(name) < 4 || len(name) > 40 || !kwNameRe.MatchString(name) {
			return
		}
		lower := strings.ToLower(name)
		if _, dup := d.keywords[lower]; dup {
			return
		}
		d.keywords[lower] = Keyword{Name: name, Rule: rule, Kind: kind}
	}

	for _, r := range d.Rules {
		if r.Depth != 2 {
			continue
		}
		var kind kwKind
		switch {
		case strings.HasPrefix(r.Number, "702."):
			kind = kwAbility
		case strings.HasPrefix(r.Number, "701."):
			kind = kwAction
		default:
			continue
		}
		name := r.Text
		add(name, r.Number, kind)
		// "Tap and Untap" / "Daybound and Nightbound" — index both halves too
		if parts := strings.Split(name, " and "); len(parts) == 2 {
			add(parts[0], r.Number, kind)
			add(parts[1], r.Number, kind)
		}
	}

	// Ability words are only ever listed inside rule 207.2c
	if idx, ok := d.Index["207.2c"]; ok {
		for _, w := range abilityWords(d.Rules[idx].Text) {
			add(w, "207.2c", kwWord)
		}
	}

	names := make([]string, 0, len(d.keywords))
	for _, k := range d.keywords {
		names = append(names, k.Name)
	}
	d.keywordRe = alternation(names)
}

// abilityWords pulls the comma-separated list out of rule 207.2c.
func abilityWords(text string) []string {
	const marker = "ability words are "
	i := strings.Index(text, marker)
	if i < 0 {
		return nil
	}
	list := text[i+len(marker):]
	if j := strings.Index(list, "."); j >= 0 {
		list = list[:j]
	}

	var out []string
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimSpace(strings.TrimPrefix(part, "and "))
		if part == "" {
			continue
		}
		out = append(out, titleCase(part))
	}
	return out
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		if len(r) == 0 {
			continue
		}
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func buildGlossaryIndex(d *RulesData) {
	d.glossary = make(map[string]GlossaryEntry)

	var names []string
	for _, g := range d.Glossary {
		term := strings.TrimSpace(g.Term)
		if !glossTermRe.MatchString(term) {
			continue
		}
		// Single short words are too noisy to match against card text
		if !strings.Contains(term, " ") && len(term) < 9 {
			continue
		}
		lower := strings.ToLower(term)
		if _, isKeyword := d.keywords[lower]; isKeyword {
			continue
		}
		if _, dup := d.glossary[lower]; dup {
			continue
		}
		d.glossary[lower] = g
		names = append(names, term)
	}

	d.glossaryRe = alternation(names)
}

// Card types map onto the category rules in section 3.
var cardTypeRules = map[string]string{
	"Artifact":     "301",
	"Creature":     "302",
	"Enchantment":  "303",
	"Instant":      "304",
	"Land":         "305",
	"Planeswalker": "306",
	"Sorcery":      "307",
	"Kindred":      "308",
	"Dungeon":      "309",
	"Battle":       "310",
	"Plane":        "311",
	"Phenomenon":   "312",
	"Vanguard":     "313",
	"Scheme":       "314",
	"Conspiracy":   "315",
}

// buildTypeIndex keeps only the type->rule pairs that the loaded rules
// actually confirm, so a renumbered future release degrades quietly.
func buildTypeIndex(d *RulesData) {
	d.typeRules = make(map[string]string)
	for cardType, num := range cardTypeRules {
		idx, ok := d.Index[num]
		if !ok {
			continue
		}
		// The category is named in the plural, and not always regularly
		// ("Sorceries", "Phenomena"), so compare on a shared prefix.
		need := len(cardType)
		if need > 5 {
			need = 5
		}
		if commonPrefixLen(strings.ToLower(d.Rules[idx].Text), strings.ToLower(cardType)) >= need {
			d.typeRules[cardType] = num
		}
	}
}

func commonPrefixLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// alternation builds a case-insensitive regexp matching any of the names,
// longest first so "Double Strike" wins over "Double".
func alternation(names []string) *regexp.Regexp {
	if len(names) == 0 {
		return nil
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	re, err := regexp.Compile(`(?i)` + strings.Join(quoted, "|"))
	if err != nil {
		return nil
	}
	return re
}

// ── Scanning ────────────────────────────────────────────────────

type span struct {
	start, end int
	text       string
}

// scan finds every whole-word occurrence of the alternation in s.
// Word boundaries are checked here rather than with \b so that names
// ending in punctuation ("For Mirrodin!") still match.
func scan(re *regexp.Regexp, s string) []span {
	if re == nil || s == "" {
		return nil
	}

	var out []span
	pos := 0
	for pos < len(s) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		if boundedAt(s, start, end) {
			out = append(out, span{start: start, end: end, text: s[start:end]})
			pos = end
		} else {
			pos = start + 1
		}
	}
	return out
}

func boundedAt(s string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		first, _ := utf8.DecodeRuneInString(s[start:])
		if isWordRune(before) && isWordRune(first) {
			return false
		}
	}
	if end < len(s) {
		last, _ := utf8.DecodeLastRuneInString(s[:end])
		after, _ := utf8.DecodeRuneInString(s[end:])
		if isWordRune(last) && isWordRune(after) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// ── Matching a card against the rules ───────────────────────────

type matchKind int

const (
	matchKeyword matchKind = iota
	matchGlossary
	matchType
)

type RuleMatch struct {
	Kind  matchKind
	Term  string // display name, e.g. "Flying"
	Rule  string // rule number, may be "" for glossary-only hits
	Kw    Keyword
	Entry GlossaryEntry
}

const maxGlossaryMatches = 8

// MatchCard returns the rules relevant to a card, most specific first:
// keywords in the order they appear in the oracle text, then glossary
// concepts, then the card's types.
func (d RulesData) MatchCard(c ScryfallCard) []RuleMatch {
	if !d.loaded() {
		return nil
	}

	var out []RuleMatch
	seen := make(map[string]bool)

	// Both halves of a double-faced card, so the back face's keywords
	// still turn up in the rules panel.
	oracle := c.combinedOracle()

	for _, sp := range scan(d.keywordRe, oracle) {
		kw, ok := d.keywords[strings.ToLower(sp.text)]
		if !ok {
			continue
		}
		key := kw.Rule
		if kw.Kind == kwWord {
			key = kw.Rule + "/" + kw.Name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, RuleMatch{Kind: matchKeyword, Term: kw.Name, Rule: kw.Rule, Kw: kw})
	}

	glossaryHits := 0
	for _, sp := range scan(d.glossaryRe, oracle) {
		if glossaryHits >= maxGlossaryMatches {
			break
		}
		entry, ok := d.glossary[strings.ToLower(sp.text)]
		if !ok {
			continue
		}
		key := "g:" + strings.ToLower(entry.Term)
		if seen[key] {
			continue
		}
		seen[key] = true
		glossaryHits++

		rule := ""
		if m := seeRuleRe.FindStringSubmatch(entry.Definition); m != nil {
			rule = m[1]
		}
		out = append(out, RuleMatch{Kind: matchGlossary, Term: entry.Term, Rule: rule, Entry: entry})
	}

	for _, t := range cardTypes(c.TypeLine) {
		num, ok := d.typeRules[t]
		if !ok || seen["t:"+num] {
			continue
		}
		seen["t:"+num] = true
		out = append(out, RuleMatch{Kind: matchType, Term: t, Rule: num})
	}

	return out
}

// cardTypes returns the card types on a type line, ignoring subtypes.
func cardTypes(typeLine string) []string {
	if typeLine == "" {
		return nil
	}
	// Only look at the front half of "Legendary Creature — Dragon"
	if i := strings.Index(typeLine, "—"); i >= 0 {
		typeLine = typeLine[:i]
	}
	// Double-faced type lines are joined with "//"
	typeLine = strings.ReplaceAll(typeLine, "//", " ")

	var out []string
	for _, word := range strings.Fields(typeLine) {
		word = strings.TrimSpace(word)
		if _, ok := cardTypeRules[word]; ok {
			out = append(out, word)
		}
	}
	return out
}

// ── File management ─────────────────────────────────────────────

const rulesURL = "https://media.wizards.com/2026/downloads/MagicCompRules%2020260417.txt"

func dataDir() string {
	dir := filepath.Join(os.Getenv("HOME"), ".local", "share", "scry")
	os.MkdirAll(dir, 0755)
	return dir
}

func rulesFilePath() string {
	return filepath.Join(dataDir(), "comprules.txt")
}

func downloadRules() error {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Preserve headers through redirects
			req.Header.Set("User-Agent", "scry/1.0")
			req.Header.Set("Accept", "*/*")
			return nil
		},
	}

	req, err := http.NewRequest("GET", rulesURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "scry/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d from %s", resp.StatusCode, rulesURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}

	return os.WriteFile(rulesFilePath(), body, 0644)
}

func loadRules() (RulesData, error) {
	path := rulesFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := downloadRules(); err != nil {
			return RulesData{}, err
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return RulesData{}, err
	}

	text := strings.TrimPrefix(string(content), "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	return parseRules(text), nil
}
