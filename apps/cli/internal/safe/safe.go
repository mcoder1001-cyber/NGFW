// Package safe keeps server-supplied text from driving the operator's terminal (review H1). Every human-mode byte
// the CLI prints goes through Writer (or String): C0 controls except newline and tab, DEL, C1 controls (U+0080–U+009F,
// also as stray bytes), invalid UTF-8 and the bidi overrides/isolates (U+202A–U+202E, U+2066–U+2069) are rendered
// visibly as `\x1b`, `\x9b`, `\u202e` … so ESC/CSI/OSC sequences, BEL, carriage returns and direction tricks
// cannot erase lines, set the title, write the clipboard or disguise text. `--json` output is not filtered (JSON
// already escapes controls).
package safe

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// dangerous reports whether r must be escaped (newline and tab are allowed only when keepLayout).
func dangerous(r rune, keepLayout bool) bool {
	switch {
	case r == '\n' || r == '\t':
		return !keepLayout
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

func escape(b *strings.Builder, r rune) {
	if r <= 0xff {
		fmt.Fprintf(b, `\x%02x`, r)
	} else {
		fmt.Fprintf(b, `\u%04x`, r)
	}
}

func clean(s string, keepLayout bool) string {
	ok := true
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && n == 1) || dangerous(r, keepLayout) {
			ok = false
			break
		}
		i += n
	}
	if ok {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case dangerous(r, keepLayout):
			escape(&b, r)
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

// String escapes every control character including newline and tab: for one-line fields (names, comments, table
// cells, prompts, completion candidates).
func String(s string) string { return clean(s, false) }

// Text escapes control characters but keeps newlines and tabs (multi-line output built by the CLI itself).
func Text(s string) string { return clean(s, true) }

// Writer filters everything written to W with Text. Incomplete UTF-8 sequences at the end of a write are held back
// until the next write.
type Writer struct {
	W       io.Writer
	pending []byte
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	buf := append(w.pending, p...)
	w.pending = nil
	// hold back a trailing, possibly incomplete UTF-8 sequence (at most 3 bytes)
	cut := len(buf)
	for i := 1; i <= 3 && i <= len(buf); i++ {
		c := buf[len(buf)-i]
		if c < 0x80 {
			break
		}
		if c >= 0xc0 { // a lead byte: complete only if its sequence fits
			need := 2
			if c >= 0xf0 {
				need = 4
			} else if c >= 0xe0 {
				need = 3
			}
			if need > i {
				cut = len(buf) - i
			}
			break
		}
	}
	w.pending = append([]byte{}, buf[cut:]...)
	if _, err := io.WriteString(w.W, Text(string(buf[:cut]))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush writes held-back bytes (escaped).
func (w *Writer) Flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	_, err := io.WriteString(w.W, Text(string(w.pending)))
	w.pending = nil
	return err
}
