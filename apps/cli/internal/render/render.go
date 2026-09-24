// Package render prints configuration values in the CLI's text formats. Both are stable (object keys sorted,
// arrays in order) and diff-friendly (one leaf per line):
//
//   - Text: a brace hierarchy, one leaf per line;
//   - Set:  one `set <path> <value>` command per leaf — the exact commands that rebuild the value in config mode.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ngfw/cli/internal/cpath"
)

// Scalar renders a leaf value as one CLI word (strings quoted only when needed).
func Scalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return cpath.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	}
	return true
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Text renders v as an indented brace hierarchy. Scalars print as `name value;`, arrays of scalars as
// `name [ a b ];`, objects as `name { … }`, array items of objects as `name <index> { … }`.
func Text(v any) string {
	var b strings.Builder
	switch x := v.(type) {
	case map[string]any:
		writeObject(&b, x, 0)
	case []any:
		writeArrayItems(&b, "", x, 0)
	default:
		b.WriteString(Scalar(x))
		b.WriteByte('\n')
	}
	return b.String()
}

func indent(b *strings.Builder, n int) {
	for i := 0; i < n; i++ {
		b.WriteString("    ")
	}
}

func writeObject(b *strings.Builder, m map[string]any, depth int) {
	for _, k := range sortedKeys(m) {
		writeMember(b, cpath.Quote(k), m[k], depth)
	}
}

func writeMember(b *strings.Builder, name string, v any, depth int) {
	switch x := v.(type) {
	case map[string]any:
		indent(b, depth)
		if len(x) == 0 {
			fmt.Fprintf(b, "%s { }\n", name)
			return
		}
		fmt.Fprintf(b, "%s {\n", name)
		writeObject(b, x, depth+1)
		indent(b, depth)
		b.WriteString("}\n")
	case []any:
		allScalar := true
		for _, e := range x {
			if !isScalar(e) {
				allScalar = false
			}
		}
		if allScalar {
			indent(b, depth)
			words := make([]string, len(x))
			for i, e := range x {
				words[i] = Scalar(e)
			}
			if len(words) == 0 {
				fmt.Fprintf(b, "%s [ ];\n", name)
			} else {
				fmt.Fprintf(b, "%s [ %s ];\n", name, strings.Join(words, " "))
			}
			return
		}
		writeArrayItems(b, name, x, depth)
	default:
		indent(b, depth)
		fmt.Fprintf(b, "%s %s;\n", name, Scalar(x))
	}
}

func writeArrayItems(b *strings.Builder, name string, arr []any, depth int) {
	for i, e := range arr {
		label := strconv.Itoa(i)
		if name != "" {
			label = name + " " + label
		}
		writeMember(b, label, e, depth)
	}
}

// Set renders v (found at the path segs) as `set` commands, one per leaf. Arrays of scalars give one line per item
// (config-mode `set` appends to such lists); empty objects/arrays are written as `{}` / `[]` so the result is
// complete.
func Set(segs []string, v any) []string {
	var out []string
	var walk func(path []string, v any)
	walk = func(path []string, v any) {
		switch x := v.(type) {
		case map[string]any:
			if len(x) == 0 {
				out = append(out, line(path, "{}"))
				return
			}
			for _, k := range sortedKeys(x) {
				walk(append(append([]string{}, path...), k), x[k])
			}
		case []any:
			if len(x) == 0 {
				out = append(out, line(path, "[]"))
				return
			}
			for i, e := range x {
				if isScalar(e) {
					out = append(out, line(path, Scalar(e)))
				} else {
					walk(append(append([]string{}, path...), strconv.Itoa(i)), e)
				}
			}
		default:
			out = append(out, line(path, Scalar(x)))
		}
	}
	walk(append([]string{}, segs...), v)
	return out
}

func line(path []string, value string) string {
	if len(path) == 0 {
		return "set " + value
	}
	return "set " + cpath.Words(path) + " " + value
}

// Change is one entry of GET /api/v1/config/diff.
type Change struct {
	Op      string `json:"op"`
	Pointer string `json:"pointer"`
	From    any    `json:"from,omitempty"`
	To      any    `json:"to,omitempty"`
}

// Diff renders diff changes as `- set …` / `+ set …` lines (removed state first, then added), in the API's order.
func Diff(changes []Change) []string {
	var out []string
	for _, c := range changes {
		segs, err := cpath.ParsePointer(c.Pointer)
		if err != nil {
			out = append(out, fmt.Sprintf("? %s %s", c.Op, c.Pointer))
			continue
		}
		if c.Op == "remove" || c.Op == "replace" {
			for _, l := range Set(segs, c.From) {
				out = append(out, "- "+l)
			}
		}
		if c.Op == "add" || c.Op == "replace" {
			for _, l := range Set(segs, c.To) {
				out = append(out, "+ "+l)
			}
		}
	}
	return out
}
