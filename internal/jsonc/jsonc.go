// Package jsonc lets scry's settings files carry comments.
//
// JSON has none, which makes a settings file a poor place to learn what the
// settings are. `scry init` writes each file with every default in it,
// commented out, so the file reads as its own documentation and changes
// nothing until a line is uncommented. This is what makes those files
// loadable: // to the end of the line is dropped, anywhere outside a string.
package jsonc

// Strip removes // comments, leaving everything else — strings included, and
// a // inside one — exactly as it was. Line breaks are kept, so a parse
// error still points at the right line.
func Strip(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inString, escaped := false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
			continue
		}
		out = append(out, c)
	}
	return out
}

// Empty reports whether a file holds nothing but comments and space — a
// template nobody has uncommented anything in, which means "all defaults".
func Empty(src []byte) bool {
	for _, c := range Strip(src) {
		switch c {
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return true
}
