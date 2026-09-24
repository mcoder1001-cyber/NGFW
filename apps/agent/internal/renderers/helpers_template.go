package renderers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
)

// ErrUnsafe is wrapped by every escaping helper that rejects a value.
var ErrUnsafe = errors.New("renderers: unsafe value")

// MaxIdentLen bounds identifiers (names of interfaces, VRFs, peers, tunnels, ...).
const MaxIdentLen = 128

// Strict template escaping. Daemon config files are line oriented, so the attack that
// matters is a user string that adds a line ("desc\nno shutdown") or a quote that ends the
// field ("desc" ; rm -rf /). Every user string must pass through exactly one of these
// helpers in the template (Funcs exposes them), and Execute checks the final output once
// more (CheckRendered) as a backstop against a raw {{.Field}}.

// Ident accepts an identifier: 1..MaxIdentLen bytes of ASCII letters, digits and "_.:/@-",
// starting with a letter or digit. Anything else (spaces, quotes, control characters,
// non-ASCII) is rejected. Use it for names that daemons parse as tokens.
func Ident(s string) (string, error) {
	if s == "" || len(s) > MaxIdentLen {
		return "", fmt.Errorf("%w: identifier length %d not in 1..%d", ErrUnsafe, len(s), MaxIdentLen)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if i == 0 && !isAlnum {
			return "", fmt.Errorf("%w: identifier %q must start with a letter or digit", ErrUnsafe, s)
		}
		if !isAlnum && !strings.ContainsRune("_.:/@-", rune(c)) {
			return "", fmt.Errorf("%w: identifier %q contains %q", ErrUnsafe, s, c)
		}
	}
	return s, nil
}

// Line accepts free text that will occupy the rest of one line (descriptions, comments):
// valid UTF-8 with no control characters at all (so no CR, LF, NUL or TAB) and no Unicode
// line or paragraph separators. The text is returned unchanged.
func Line(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%w: text is not valid UTF-8", ErrUnsafe)
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\u0085' {
			return "", fmt.Errorf("%w: text contains control character %U", ErrUnsafe, r)
		}
	}
	return s, nil
}

// Quoted returns s as a double-quoted string with backslash and double quote escaped, after
// the Line check. Use it where the daemon syntax takes a quoted string with C-like escapes
// (FRR, strongSwan swanctl, Unbound, chrony).
func Quoted(s string) (string, error) {
	if _, err := Line(s); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' || s[i] == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')
	return b.String(), nil
}

// JSONString returns s as a JSON string literal (for Kea and other JSON configs). Any valid
// UTF-8 string is representable; control characters are escaped, not rejected.
func JSONString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%w: text is not valid UTF-8", ErrUnsafe)
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnsafe, err)
	}
	return string(b), nil
}

// Addr parses an IPv4/IPv6 address and returns its canonical text (lower-case, compressed
// IPv6). Zones are rejected.
func Addr(s string) (string, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("%w: address %q: %v", ErrUnsafe, s, err)
	}
	if a.Zone() != "" {
		return "", fmt.Errorf("%w: address %q has a zone", ErrUnsafe, s)
	}
	return a.String(), nil
}

// Prefix parses "addr/len" and returns the canonical text of the address as given with its
// length (10.2.1.1/24 stays 10.2.1.1/24; use Network for the masked form).
func Prefix(s string) (string, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", fmt.Errorf("%w: prefix %q: %v", ErrUnsafe, s, err)
	}
	return p.String(), nil
}

// Network parses "addr/len" and returns the masked network (10.2.1.1/24 -> 10.2.1.0/24).
func Network(s string) (string, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", fmt.Errorf("%w: prefix %q: %v", ErrUnsafe, s, err)
	}
	return p.Masked().String(), nil
}

// Funcs returns the strict helpers as template functions: ident, line, quoted, json, addr,
// prefix, network. A helper error aborts template execution.
func Funcs() template.FuncMap {
	return template.FuncMap{
		"ident":   Ident,
		"line":    Line,
		"quoted":  Quoted,
		"json":    JSONString,
		"addr":    Addr,
		"prefix":  Prefix,
		"network": Network,
	}
}

// NewTemplate returns an empty text/template with Funcs installed and missingkey=error, so
// a typo in a field name fails rendering instead of producing an empty value. Parse your
// templates into it (ParseFS for templates/*.tmpl).
func NewTemplate(name string) *template.Template {
	return template.New(name).Funcs(Funcs()).Option("missingkey=error")
}

// Execute renders t with data and runs CheckRendered on the result.
func Execute(t *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("renderers: template %s: %w", t.Name(), err)
	}
	if err := CheckRendered(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("renderers: template %s: %w", t.Name(), err)
	}
	return buf.Bytes(), nil
}

// ExecuteTemplate renders the named associated template and runs CheckRendered.
func ExecuteTemplate(t *template.Template, name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("renderers: template %s: %w", name, err)
	}
	if err := CheckRendered(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("renderers: template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// CheckRendered is the backstop for rendered text configs: valid UTF-8, and no control
// characters other than "\n" and "\t" (so no CR, NUL, ESC or other C0/C1 codes) and no
// Unicode line or paragraph separators. JSON output never trips it because JSON escapes
// control characters.
func CheckRendered(b []byte) error {
	if !utf8.Valid(b) {
		return fmt.Errorf("%w: rendered output is not valid UTF-8", ErrUnsafe)
	}
	line := 1
	for _, r := range string(b) {
		switch {
		case r == '\n':
			line++
		case r == '\t':
		case unicode.IsControl(r) || r == '\u2028' || r == '\u2029':
			return fmt.Errorf("%w: control character %U in rendered output at line %d", ErrUnsafe, r, line)
		}
	}
	return nil
}
