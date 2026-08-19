package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
)

// Small helpers with no home of their own.

// padTo pads a string out to a rune width, for lining columns up by hand
// where a lipgloss Width() would also repaint the colour.
func padTo(s string, width int) string {
	if n := runeLen(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// stripStyle removes ANSI colour, so text can be recoloured wholesale — a
// nested style would otherwise end at the first reset the inner one wrote.
func stripStyle(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// truncate cuts to a rune count, not a byte count — an em dash in a type
// line is three bytes, and slicing through one renders as a replacement
// character.
func truncate(s string, maxLen int) string {
	if runeLen(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatInt(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteString(",")
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
func stdoutWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		w = 80
	}
	w -= 2
	if w > 100 {
		w = 100 // long lines are hard to read however wide the terminal is
	}
	if w < 30 {
		w = 30
	}
	return w
}

// plural is the same six lines as in package deck and package moxfield.
// Three copies beats a package for one grammar rule, or exporting it from
// somewhere it doesn't belong.
func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
