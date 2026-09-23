package vppstartup

import (
	"fmt"
	"slices"
	"strings"
)

// Semantic comparison of startup.conf files. VPP's parser (unformat) is whitespace-agnostic and
// ignores the order of sections and of most entries inside a section, and '#' starts a comment.
// Parse reduces a file to its sections and entries; Canonical flattens that to sorted lines such
// as "dpdk > blacklist 0000:0b:00.0" or "plugins > plugin npt66_plugin.so > enable", so two files
// that VPP reads the same way compare equal regardless of comments, indentation and ordering.
//
// Entries are taken per line (the file layout VPP ships and we render: one entry per line,
// `name { … }` opening a section). It is a comparison aid, not a VPP parser.

// Section is one `name { … }` block (the root has Name "").
type Section struct {
	Name     string
	Entries  []string
	Sections []*Section
}

// Parse parses a startup.conf into its section tree. Unbalanced braces are an error.
func Parse(b []byte) (*Section, error) {
	root := &Section{}
	stack := []*Section{root}
	for ln, raw := range strings.Split(string(b), "\n") {
		line := stripComment(raw)
		var words []string
		flush := func() {
			if len(words) > 0 {
				cur := stack[len(stack)-1]
				cur.Entries = append(cur.Entries, strings.Join(words, " "))
				words = nil
			}
		}
		for _, tok := range tokens(line) {
			switch tok {
			case "{":
				if len(words) == 0 {
					return nil, fmt.Errorf("line %d: '{' without a section name", ln+1)
				}
				s := &Section{Name: strings.Join(words, " ")}
				words = nil
				cur := stack[len(stack)-1]
				cur.Sections = append(cur.Sections, s)
				stack = append(stack, s)
			case "}":
				flush()
				if len(stack) == 1 {
					return nil, fmt.Errorf("line %d: unmatched '}'", ln+1)
				}
				stack = stack[:len(stack)-1]
			default:
				words = append(words, tok)
			}
		}
		flush()
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("unterminated section %q", stack[len(stack)-1].Name)
	}
	return root, nil
}

// stripComment drops everything from a '#' that starts a token.
func stripComment(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t' || line[i-1] == '{' || line[i-1] == '}') {
			return line[:i]
		}
	}
	return line
}

// tokens splits on whitespace and makes '{' and '}' separate tokens.
func tokens(line string) []string {
	line = strings.ReplaceAll(line, "{", " { ")
	line = strings.ReplaceAll(line, "}", " } ")
	return strings.Fields(line)
}

// Canonical flattens the tree to sorted lines: "<path> {}" for every section (so an empty
// `cpu { }` still counts) and "<path> > <entry>" for every entry.
func (s *Section) Canonical() []string {
	var out []string
	var walk func(prefix string, sec *Section)
	walk = func(prefix string, sec *Section) {
		for _, e := range sec.Entries {
			out = append(out, join(prefix, e))
		}
		for _, c := range sec.Sections {
			p := join(prefix, c.Name)
			out = append(out, p+" {}")
			walk(p, c)
		}
	}
	walk("", s)
	slices.Sort(out)
	return out
}

func join(prefix, s string) string {
	if prefix == "" {
		return s
	}
	return prefix + " > " + s
}

// SemanticDiff parses both files and returns the canonical lines only in a and only in b
// (multiset difference, each sorted).
func SemanticDiff(a, b []byte) (onlyA, onlyB []string, err error) {
	pa, err := Parse(a)
	if err != nil {
		return nil, nil, fmt.Errorf("first file: %w", err)
	}
	pb, err := Parse(b)
	if err != nil {
		return nil, nil, fmt.Errorf("second file: %w", err)
	}
	ca, cb := pa.Canonical(), pb.Canonical()
	count := map[string]int{}
	for _, l := range cb {
		count[l]++
	}
	for _, l := range ca {
		if count[l] > 0 {
			count[l]--
			continue
		}
		onlyA = append(onlyA, l)
	}
	count = map[string]int{}
	for _, l := range ca {
		count[l]++
	}
	for _, l := range cb {
		if count[l] > 0 {
			count[l]--
			continue
		}
		onlyB = append(onlyB, l)
	}
	return onlyA, onlyB, nil
}
