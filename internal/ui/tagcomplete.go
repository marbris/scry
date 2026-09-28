package ui

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Tab completion in the tag prompt.
//
// Tagging is the one prompt whose answers repeat: the same dozen tags go on
// card after card, and a tag typed slightly differently — "remova", "Ramp " —
// is a new tag rather than a typo, silently splitting a category in two. So
// tab finishes the word being typed from the tags that already exist, the
// way a shell finishes a filename:
//
//   - one tag fits: it is filled in, with a space after it for the next;
//   - several fit: the word grows to what they have in common, and they are
//     listed on the notice line;
//   - nothing more in common: tab walks through them, shift+tab back.
//
// A leading - (take the tag off) is kept, and the completion is of the tag
// after it.

// tagCompletion is a walk through the tags that fit, while tab is being
// pressed. Any other key ends it.
type tagCompletion struct {
	before string   // the input up to the word being completed, and its "-"
	fits   []string // the tags that fit, in order
	at     int      // which one is showing; -1 before the walk starts
	shown  string   // what the input held after the last tab
}

// knownTags is every tag in the decks on screen, the editing deck's among
// them, in order.
func (m Model) knownTags() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range m.ws.panels {
		l := p.cardsView()
		if l == nil || l.deck == nil {
			continue
		}
		for _, c := range l.all {
			for _, t := range c.Tags {
				if !seen[t] {
					seen[t] = true
					out = append(out, t)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// completeTag is tab (delta 1) or shift+tab (delta -1) in the tag prompt.
func (m *Model) completeTag(p *panel, delta int) {
	value := p.askInput.Value()

	// Still walking: the input is what the last tab left, so step on.
	if c := p.tagComp; c != nil && c.shown == value && len(c.fits) > 1 {
		switch {
		case c.at < 0 && delta > 0:
			c.at = 0
		case c.at < 0:
			c.at = len(c.fits) - 1
		default:
			c.at = (c.at + delta + len(c.fits)) % len(c.fits)
		}
		p.setAsk(c.before + c.fits[c.at])
		c.shown = p.askInput.Value()
		m.notice = tagList(c.fits, c.at)
		return
	}
	p.tagComp = nil

	// The word being completed is the last one; tags are single words.
	start := strings.LastIndex(value, " ") + 1
	before, word := value[:start], value[start:]
	if strings.HasPrefix(word, "-") {
		before, word = before+"-", word[1:]
	}
	word = strings.ToLower(word)

	known := m.knownTags()
	if len(known) == 0 {
		m.notice = "no tags yet to complete from"
		return
	}
	var fits []string
	for _, t := range known {
		if strings.HasPrefix(t, word) {
			fits = append(fits, t)
		}
	}
	switch len(fits) {
	case 0:
		m.notice = `no tag starts with "` + word + `"`
		return
	case 1:
		p.setAsk(before + fits[0] + " ")
		return
	}

	c := &tagCompletion{before: before, fits: fits, at: -1}
	if common := commonPrefix(fits); len(common) > len(word) {
		p.setAsk(before + common)
	} else {
		// Nothing to add: start the walk straight away.
		c.at = 0
		if delta < 0 {
			c.at = len(fits) - 1
		}
		p.setAsk(before + fits[c.at])
	}
	c.shown = p.askInput.Value()
	p.tagComp = c
	m.notice = tagList(fits, c.at)
}

// setAsk replaces what the prompt holds, the cursor at the end of it.
func (p *panel) setAsk(s string) {
	p.askInput.SetValue(s)
	p.askInput.CursorEnd()
}

// tagList is the tags that fit, for the notice line, the one showing in
// brackets.
func tagList(tags []string, at int) string {
	shown := make([]string, len(tags))
	for i, t := range tags {
		if i == at {
			t = "[" + t + "]"
		}
		shown[i] = t
	}
	return strings.Join(shown, " · ")
}

// commonPrefix is what every string starts with.
func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	prefix := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, prefix) {
			_, size := utf8.DecodeLastRuneInString(prefix)
			prefix = prefix[:len(prefix)-size]
		}
	}
	return prefix
}
