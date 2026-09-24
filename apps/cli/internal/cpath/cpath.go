// Package cpath maps CLI configuration paths to RFC 6901 JSON pointers and back, and tokenizes command lines.
//
// A CLI path is a list of words; every word is exactly one (unescaped) pointer segment, so the mapping is 1:1:
//
//	interfaces TenGigabitEthernet0/0/0 mtu   ⇔   /interfaces/TenGigabitEthernet0~10~10/mtu
//
// A single word starting with "/" is read as a JSON pointer itself (absolute, escapes ~0 and ~1).
package cpath

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"ngfw/cli/internal/safe"
)

// EscapeSegment escapes one pointer token (RFC 6901 §3): "~" → "~0", "/" → "~1".
func EscapeSegment(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// UnescapeSegment reverses EscapeSegment; a "~" not followed by 0 or 1 is an error.
func UnescapeSegment(s string) (string, error) {
	if !strings.Contains(s, "~") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) || (s[i+1] != '0' && s[i+1] != '1') {
			return "", fmt.Errorf("invalid JSON pointer escape in %q", s)
		}
		if s[i+1] == '0' {
			b.WriteByte('~')
		} else {
			b.WriteByte('/')
		}
		i++
	}
	return b.String(), nil
}

// ParsePointer splits a JSON pointer into unescaped segments. "" is the whole document.
func ParsePointer(p string) ([]string, error) {
	if p == "" {
		return []string{}, nil
	}
	if !strings.HasPrefix(p, "/") {
		return nil, fmt.Errorf("a JSON pointer starts with '/': %q", p)
	}
	raw := strings.Split(p[1:], "/")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		s, err := UnescapeSegment(r)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Pointer joins unescaped segments into a JSON pointer.
func Pointer(segs []string) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteByte('/')
		b.WriteString(EscapeSegment(s))
	}
	return b.String()
}

// Resolve turns path words into absolute segments. base is the current edit level. An empty word list is the edit
// level itself; a single word starting with "/" is an absolute JSON pointer; ".." climbs one level.
func Resolve(base []string, words []string) ([]string, error) {
	if len(words) == 1 && strings.HasPrefix(words[0], "/") {
		return ParsePointer(words[0])
	}
	out := append([]string{}, base...)
	for _, w := range words {
		if w == ".." {
			if len(out) == 0 {
				return nil, errors.New("already at the top of the configuration")
			}
			out = out[:len(out)-1]
			continue
		}
		if w == "" {
			return nil, errors.New("empty path element")
		}
		out = append(out, w)
	}
	return out, nil
}

// Words renders segments as a CLI path (quoted where a word would not survive Tokenize).
func Words(segs []string) string {
	q := make([]string, len(segs))
	for i, s := range segs {
		q[i] = Quote(s)
	}
	return strings.Join(q, " ")
}

// NeedsQuote reports whether s must be quoted to come back from Tokenize as one identical token.
func NeedsQuote(s string) bool {
	if s == "" {
		return true
	}
	switch s[0] {
	case '{', '[', '"', '\'', '#', '/':
		return true
	}
	if strings.ContainsAny(s, " \t\r\n\"'\\") {
		return true
	}
	return safe.String(s) != s // control characters, bidi overrides, invalid UTF-8
}

// Quote returns s as a token: unchanged when safe, else double-quoted with \-escapes.
func Quote(s string) string {
	if !NeedsQuote(s) {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for i, r := range s {
		if r == utf8.RuneError {
			if _, n := utf8.DecodeRuneInString(s[i:]); n == 1 {
				fmt.Fprintf(&b, `\x%02x`, s[i])
				continue
			}
		}
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			switch {
			case r >= 0x80 && r <= 0x9f: // C1 rune: \u so that Tokenize gives the rune back, not a byte
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				b.WriteString(safe.String(string(r))) // \xNN (C0, DEL) / \uNNNN (bidi) — Tokenize reads them back
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Token is one word of a command line.
type Token struct {
	Text   string // the value after unquoting
	Start  int    // byte offset of the token in the line
	Quoted bool   // the token was quoted (or is a JSON literal) — never a keyword
}

// ErrUnterminated is returned when the line ends inside quotes or a JSON literal.
var ErrUnterminated = errors.New("unterminated quote or JSON value")

// Tokenize splits a command line. Words are separated by blanks; "…" allows \" \\ \n \t escapes; '…' is literal;
// a word starting with '{' or '[' runs to its balanced closing bracket (JSON strings respected) so JSON values need
// no quoting. It returns the tokens found so far together with ErrUnterminated for an unfinished last token
// (completion uses that).
func Tokenize(line string) ([]Token, error) {
	var toks []Token
	i := 0
	for {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			return toks, nil
		}
		start := i
		switch line[i] {
		case '"':
			var b strings.Builder
			i++
			closed := false
			for i < len(line) {
				c := line[i]
				if c == '\\' && i+1 < len(line) {
					switch line[i+1] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					case 'r':
						b.WriteByte('\r')
					case 'x':
						if v, err := strconv.ParseUint(safeSlice(line, i+2, 2), 16, 8); err == nil {
							b.WriteByte(byte(v))
							i += 4
							continue
						}
						b.WriteByte('x')
					case 'u':
						if v, err := strconv.ParseUint(safeSlice(line, i+2, 4), 16, 16); err == nil {
							b.WriteRune(rune(uint16(v))) //nolint:gosec // 4 hex digits: fits
							i += 6
							continue
						}
						b.WriteByte('u')
					default:
						b.WriteByte(line[i+1])
					}
					i += 2
					continue
				}
				if c == '"' {
					closed = true
					i++
					break
				}
				b.WriteByte(c)
				i++
			}
			toks = append(toks, Token{Text: b.String(), Start: start, Quoted: true})
			if !closed {
				return toks, ErrUnterminated
			}
		case '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				toks = append(toks, Token{Text: line[i+1:], Start: start, Quoted: true})
				return toks, ErrUnterminated
			}
			toks = append(toks, Token{Text: line[i+1 : i+1+end], Start: start, Quoted: true})
			i += end + 2
		case '{', '[':
			end, ok := jsonEnd(line, i)
			toks = append(toks, Token{Text: line[i:end], Start: start, Quoted: true})
			i = end
			if !ok {
				return toks, ErrUnterminated
			}
		default:
			for i < len(line) && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			toks = append(toks, Token{Text: line[start:i], Start: start})
		}
	}
}

func safeSlice(s string, from, n int) string {
	if from+n > len(s) {
		return ""
	}
	return s[from : from+n]
}

// jsonEnd finds the end (exclusive) of the bracketed JSON value starting at line[i].
func jsonEnd(line string, i int) (int, bool) {
	depth := 0
	inStr := false
	for j := i; j < len(line); j++ {
		c := line[j]
		if inStr {
			switch c {
			case '\\':
				j++
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return j + 1, true
			}
		}
	}
	return len(line), false
}

// Texts returns the token values.
func Texts(toks []Token) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.Text
	}
	return out
}
