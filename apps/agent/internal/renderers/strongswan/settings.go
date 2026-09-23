package strongswan

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// A strict parser for the subset of the strongSwan settings grammar this renderer emits.
// strongSwan has no offline checker for swanctl.conf/strongswan.conf (swanctl only talks to a
// running charon), so Validate parses the rendered files with this parser, serialises the tree
// back and requires byte equality (a canonical-form round trip), then checks the tree
// semantically. Apply converts the same tree into VICI messages exactly as swanctl does
// (src/swanctl/commands/load_conns.c, load_creds.c, load_pools.c), so the live config and the
// files the boot path (`swanctl --load-all`) reads can never differ.
//
// Accepted lines (indentation: one tab per level, nothing else):
//
//	# comment                  (free text, no control characters)
//	<name> {                   (name: [A-Za-z0-9_+-]{1,64}, never "include")
//	}
//	<key> = <bare token>       (tokenRe: no blanks, quotes, #, { or })
//	<key> = "<quoted>"         (only \\ and \" escapes)
//	(empty line)
//
// Everything the full grammar allows beyond that — section references (`a : b {`), include
// statements, multi-token values, other escapes, CR, trailing blanks — is rejected. Errors
// carry the file, line number and a reason, never the line content (the secrets file goes
// through the same parser).

// ItemKind is the kind of a settings tree item.
type ItemKind int

// Item kinds.
const (
	KindComment ItemKind = iota + 1
	KindBlank
	KindKey
	KindSection
)

// Item is one line-level element of a section body.
type Item struct {
	Kind ItemKind
	// Name is the key or section name; Text the comment text (after "# ").
	Name, Text string
	// Value is the decoded value of a key; Quoted says how it was written.
	Value  string
	Quoted bool
	// Section is the body of a KindSection item.
	Section *Section
	// Line is the 1-based source line (0 for built trees).
	Line int
}

// Section is an ordered section body.
type Section struct {
	Items []*Item
}

// Keys returns the key items in order.
func (s *Section) Keys() []*Item {
	var out []*Item
	for _, it := range s.Items {
		if it.Kind == KindKey {
			out = append(out, it)
		}
	}
	return out
}

// Sections returns the subsection items in order.
func (s *Section) Sections() []*Item {
	var out []*Item
	for _, it := range s.Items {
		if it.Kind == KindSection {
			out = append(out, it)
		}
	}
	return out
}

// Get returns the value of key and whether it exists.
func (s *Section) Get(key string) (string, bool) {
	for _, it := range s.Items {
		if it.Kind == KindKey && it.Name == key {
			return it.Value, true
		}
	}
	return "", false
}

// Sub returns the subsection name (nil when absent).
func (s *Section) Sub(name string) *Section {
	for _, it := range s.Items {
		if it.Kind == KindSection && it.Name == name {
			return it.Section
		}
	}
	return nil
}

// ErrSyntax is wrapped by every parse error.
var ErrSyntax = errors.New("strongswan: settings syntax")

// MaxSettingsSize bounds a file the parser accepts (rendered files are far smaller).
const MaxSettingsSize = 8 << 20

// maxDepth bounds section nesting (connections.<c>.children.<ch> is depth 4).
const maxDepth = 8

// ParseSettings parses src strictly. file names the source in errors.
func ParseSettings(file string, src []byte) (*Section, error) {
	fail := func(line int, format string, a ...any) error {
		return fmt.Errorf("%w: %s line %d: %s", ErrSyntax, file, line, fmt.Sprintf(format, a...))
	}
	if len(src) > MaxSettingsSize {
		return nil, fmt.Errorf("%w: %s: %d bytes exceeds %d", ErrSyntax, file, len(src), MaxSettingsSize)
	}
	if !utf8.Valid(src) {
		return nil, fmt.Errorf("%w: %s: not valid UTF-8", ErrSyntax, file)
	}
	if len(src) > 0 && src[len(src)-1] != '\n' {
		return nil, fmt.Errorf("%w: %s: missing final newline", ErrSyntax, file)
	}
	root := &Section{}
	stack := []*Section{root}
	lines := bytes.Split(src[:max(len(src)-1, 0)], []byte("\n"))
	if len(src) == 0 {
		lines = nil
	}
	for i, raw := range lines {
		n := i + 1
		line := string(raw)
		depth := len(line) - len(strings.TrimLeft(line, "\t"))
		body := line[depth:]
		cur := stack[len(stack)-1]
		if body == "" {
			if depth != 0 {
				return nil, fail(n, "indented empty line")
			}
			cur.Items = append(cur.Items, &Item{Kind: KindBlank, Line: n})
			continue
		}
		if strings.ContainsAny(body, "\r\x00") || strings.HasPrefix(body, " ") || strings.HasSuffix(body, " ") || strings.HasSuffix(body, "\t") {
			return nil, fail(n, "blank or control character at the edge of the line")
		}
		for _, r := range body {
			if r < 0x20 && r != '\t' || r == 0x7f || r == ' ' || r == ' ' || (r >= 0x80 && r < 0xa0) {
				return nil, fail(n, "control character")
			}
		}
		if body == "}" {
			if depth != len(stack)-2 || len(stack) == 1 {
				return nil, fail(n, "unbalanced or mis-indented '}'")
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if depth != len(stack)-1 {
			return nil, fail(n, "indentation %d, want %d", depth, len(stack)-1)
		}
		switch {
		case strings.HasPrefix(body, "# ") || body == "#":
			cur.Items = append(cur.Items, &Item{Kind: KindComment, Text: strings.TrimPrefix(strings.TrimPrefix(body, "#"), " "), Line: n})
		case strings.HasSuffix(body, " {"):
			name := strings.TrimSuffix(body, " {")
			if _, err := SectionName(name); err != nil {
				return nil, fail(n, "invalid section name")
			}
			if len(stack) > maxDepth {
				return nil, fail(n, "sections nested deeper than %d", maxDepth)
			}
			for _, it := range cur.Items {
				if it.Kind == KindSection && it.Name == name {
					return nil, fail(n, "duplicate section %s", name)
				}
			}
			sec := &Section{}
			cur.Items = append(cur.Items, &Item{Kind: KindSection, Name: name, Section: sec, Line: n})
			stack = append(stack, sec)
		default:
			key, val, ok := strings.Cut(body, " = ")
			if !ok {
				return nil, fail(n, "expected '<key> = <value>', '<name> {', '}' or '# comment'")
			}
			if _, err := SectionName(key); err != nil {
				return nil, fail(n, "invalid key name")
			}
			if _, dup := cur.Get(key); dup {
				return nil, fail(n, "duplicate key %s", key)
			}
			it := &Item{Kind: KindKey, Name: key, Line: n}
			if strings.HasPrefix(val, `"`) {
				v, err := Unquote(val)
				if err != nil {
					return nil, fail(n, "value of %s: %v", key, err)
				}
				it.Value, it.Quoted = v, true
			} else {
				if _, err := Token(val); err != nil {
					return nil, fail(n, "value of %s is not a bare token", key)
				}
				it.Value = val
			}
			cur.Items = append(cur.Items, it)
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("%w: %s: %d unclosed section(s) at end of file", ErrSyntax, file, len(stack)-1)
	}
	return root, nil
}

// Serialize writes the tree in canonical form (the inverse of ParseSettings).
func (s *Section) Serialize() ([]byte, error) {
	var b bytes.Buffer
	if err := s.write(&b, 0); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (s *Section) write(b *bytes.Buffer, depth int) error {
	ind := strings.Repeat("\t", depth)
	for _, it := range s.Items {
		switch it.Kind {
		case KindBlank:
			b.WriteByte('\n')
		case KindComment:
			if it.Text == "" {
				b.WriteString(ind + "#\n")
			} else {
				b.WriteString(ind + "# " + it.Text + "\n")
			}
		case KindKey:
			v := it.Value
			if it.Quoted {
				q, err := Quote(v)
				if err != nil {
					return fmt.Errorf("%w: key %s: %v", ErrSyntax, it.Name, err)
				}
				v = q
			}
			b.WriteString(ind + it.Name + " = " + v + "\n")
		case KindSection:
			b.WriteString(ind + it.Name + " {\n")
			if err := it.Section.write(b, depth+1); err != nil {
				return err
			}
			b.WriteString(ind + "}\n")
		}
	}
	return nil
}

// RoundTrip parses src and requires the canonical serialisation to equal it byte for byte.
func RoundTrip(file string, src []byte) (*Section, error) {
	tree, err := ParseSettings(file, src)
	if err != nil {
		return nil, err
	}
	out, err := tree.Serialize()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(out, src) {
		return nil, fmt.Errorf("%w: %s is not in canonical form (round trip differs at byte %d)", ErrSyntax, file, firstDiff(out, src))
	}
	return tree, nil
}

func firstDiff(a, b []byte) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// splitList splits a comma list value as swanctl does (enumerator_create_token(value, ",", " ")).
func splitList(v string) []string {
	var out []string
	for _, t := range strings.Split(v, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
