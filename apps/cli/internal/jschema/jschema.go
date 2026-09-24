// Package jschema walks the configuration JSON Schema that the API publishes in its OpenAPI document
// (components.schemas.RootConfig and the domain components it references). The CLI uses it for tab completion,
// `?` help, value coercion and client-side validation before a request is sent. Nothing here is hand-written per
// domain: the schema comes from the live API (GET /api/docs-json), so new domains need no CLI change.
package jschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Doc is a loaded OpenAPI document (only components.schemas is used).
type Doc struct {
	schemas map[string]any
	rootRef string
}

// Node is one schema object inside a Doc.
type Node struct {
	d *Doc
	m map[string]any
}

// Load reads an OpenAPI 3.1 document and selects components.schemas.RootConfig as the root.
func Load(openapi []byte) (*Doc, error) {
	var top struct {
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi, &top); err != nil {
		return nil, fmt.Errorf("OpenAPI document: %w", err)
	}
	if _, ok := top.Components.Schemas["RootConfig"].(map[string]any); !ok {
		return nil, errors.New("OpenAPI document has no components.schemas.RootConfig")
	}
	return &Doc{schemas: top.Components.Schemas, rootRef: "RootConfig"}, nil
}

// Root is the schema of the whole configuration document.
func (d *Doc) Root() *Node {
	m, _ := d.schemas[d.rootRef].(map[string]any)
	return &Node{d: d, m: m}
}

// resolve follows $ref chains ("#/components/schemas/X").
func (n *Node) resolve() *Node {
	cur := n
	for i := 0; i < 16 && cur != nil; i++ {
		ref, ok := cur.m["$ref"].(string)
		if !ok {
			return cur
		}
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		m, ok := cur.d.schemas[name].(map[string]any)
		if !ok {
			return &Node{d: cur.d, m: map[string]any{}}
		}
		cur = &Node{d: cur.d, m: m}
	}
	return cur
}

func (n *Node) sub(v any) *Node {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return (&Node{d: n.d, m: m}).resolve()
}

// Raw exposes the underlying schema object (read-only use).
func (n *Node) Raw() map[string]any { return n.resolve().m }

// branches are the alternatives of anyOf/oneOf, or the node itself.
func (n *Node) branches() []*Node {
	r := n.resolve()
	var out []*Node
	for _, k := range []string{"anyOf", "oneOf"} {
		if arr, ok := r.m[k].([]any); ok {
			for _, a := range arr {
				if s := r.sub(a); s != nil {
					out = append(out, s.branches()...)
				}
			}
		}
	}
	if len(out) == 0 {
		return []*Node{r}
	}
	return out
}

// Types lists the JSON types the node allows ("object", "array", "string", "integer", "number", "boolean", "null").
func (n *Node) Types() []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, b := range n.branches() {
		switch t := b.m["type"].(type) {
		case string:
			add(t)
		case []any:
			for _, x := range t {
				if s, ok := x.(string); ok {
					add(s)
				}
			}
		default:
			if _, ok := b.m["properties"]; ok {
				add("object")
			} else if _, ok := b.m["additionalProperties"]; ok {
				add("object")
			} else if c, ok := b.m["const"]; ok {
				add(jsonType(c))
			} else if e, ok := b.m["enum"].([]any); ok && len(e) > 0 {
				add(jsonType(e[0]))
			}
		}
	}
	return out
}

// Has reports whether t is one of the node's types.
func (n *Node) Has(t string) bool {
	for _, x := range n.Types() {
		if x == t || (t == "number" && x == "integer") {
			return true
		}
	}
	return false
}

// IsContainer reports whether the node is an object or an array of objects (a path continues below it).
func (n *Node) IsContainer() bool {
	if n.Has("object") {
		return true
	}
	if n.Has("array") {
		if it := n.Items(); it != nil && it.Has("object") {
			return true
		}
	}
	return false
}

// IsLeafList reports an array whose items are scalars (addresses, names…): `set` appends, `delete … <value>` removes.
func (n *Node) IsLeafList() bool {
	if !n.Has("array") {
		return false
	}
	it := n.Items()
	return it != nil && !it.Has("object") && !it.Has("array")
}

// Items is the schema of array items (nil when the node is not an array).
func (n *Node) Items() *Node {
	for _, b := range n.branches() {
		if it := b.sub(b.m["items"]); it != nil {
			return it
		}
	}
	return nil
}

// Property is one named child of an object schema.
type Property struct {
	Name  string
	Node  *Node
	Order float64
}

// Properties lists the fixed properties of an object (UI order, then name).
func (n *Node) Properties() []Property {
	var out []Property
	seen := map[string]bool{}
	for _, b := range n.branches() {
		props, ok := b.m["properties"].(map[string]any)
		if !ok {
			continue
		}
		for name, v := range props {
			if seen[name] {
				continue
			}
			seen[name] = true
			c := b.sub(v)
			if c == nil {
				continue
			}
			order := math.MaxFloat64
			if ui, ok := c.m["x-vrx-ui"].(map[string]any); ok {
				if o, ok := ui["order"].(float64); ok {
					order = o
				}
			}
			out = append(out, Property{Name: name, Node: c, Order: order})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// MapValue is the schema of the values of a keyed map (additionalProperties with a schema), nil otherwise.
func (n *Node) MapValue() *Node {
	for _, b := range n.branches() {
		if s := b.sub(b.m["additionalProperties"]); s != nil {
			return s
		}
	}
	return nil
}

// KeyNode is the schema of map keys (propertyNames), nil when absent.
func (n *Node) KeyNode() *Node {
	for _, b := range n.branches() {
		if s := b.sub(b.m["propertyNames"]); s != nil {
			return s
		}
	}
	return nil
}

// Child is the schema of one path segment below n.
func (n *Node) Child(seg string) (*Node, error) {
	for _, b := range n.branches() {
		if props, ok := b.m["properties"].(map[string]any); ok {
			if v, ok := props[seg]; ok {
				if c := b.sub(v); c != nil {
					return c, nil
				}
			}
		}
	}
	if mv := n.MapValue(); mv != nil {
		if k := n.KeyNode(); k != nil {
			if issues := k.Validate(seg, ""); len(issues) > 0 {
				return nil, fmt.Errorf("%q is not a valid %s: %s", seg, k.Label("key"), issues[0].Message)
			}
		}
		return mv, nil
	}
	if n.Has("array") {
		if seg == "-" || isIndex(seg) {
			if it := n.Items(); it != nil {
				return it, nil
			}
		}
		return nil, fmt.Errorf("%q is not an array index (0, 1, …)", seg)
	}
	names := make([]string, 0)
	for _, p := range n.Properties() {
		names = append(names, p.Name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%q: nothing below a %s value", seg, strings.Join(n.Types(), "|"))
	}
	return nil, fmt.Errorf("%q is not valid here; expected one of: %s", seg, strings.Join(names, ", "))
}

// Walk returns the schema at segs (below n).
func (n *Node) Walk(segs []string) (*Node, error) {
	cur := n
	for i, s := range segs {
		c, err := cur.Child(s)
		if err != nil {
			return nil, fmt.Errorf("at element %d: %w", i+1, err)
		}
		cur = c
	}
	return cur, nil
}

func isIndex(s string) bool {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Title is the schema title ("" when absent).
func (n *Node) Title() string {
	for _, b := range n.branches() {
		if t, ok := b.m["title"].(string); ok {
			return t
		}
	}
	return ""
}

// Help is the UI help text or description.
func (n *Node) Help() string {
	for _, b := range n.branches() {
		if ui, ok := b.m["x-vrx-ui"].(map[string]any); ok {
			if h, ok := ui["help"].(string); ok {
				return h
			}
		}
		if d, ok := b.m["description"].(string); ok {
			return d
		}
	}
	return ""
}

// Label is the title, or fallback.
func (n *Node) Label(fallback string) string {
	if t := n.Title(); t != "" {
		return t
	}
	return fallback
}

// Enum lists the allowed literal values (enum and const across branches), as their text form.
func (n *Node) Enum() []string {
	var out []string
	seen := map[string]bool{}
	add := func(v any) {
		s := literal(v)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, b := range n.branches() {
		if e, ok := b.m["enum"].([]any); ok {
			for _, v := range e {
				add(v)
			}
		}
		if c, ok := b.m["const"]; ok {
			add(c)
		}
		if t, _ := b.m["type"].(string); t == "boolean" {
			add(true)
			add(false)
		}
	}
	return out
}

// TypeHint is a short human description of the expected value, e.g. "integer 68..9216" or "string, e.g. 10.0.0.1/24".
func (n *Node) TypeHint() string {
	var parts []string
	for _, b := range n.branches() {
		t := strings.Join((&Node{d: b.d, m: b.m}).Types(), "|")
		if t == "" {
			t = "value"
		}
		if lo, ok := b.m["minimum"].(float64); ok {
			if hi, ok := b.m["maximum"].(float64); ok && math.Abs(hi) < 1e15 && math.Abs(lo) < 1e15 {
				t += fmt.Sprintf(" %s..%s", num(lo), num(hi))
			}
		}
		if f, ok := b.m["format"].(string); ok {
			t += " (" + f + ")"
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, " or ")
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func literal(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "null"
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if x == math.Trunc(x) {
			return "integer"
		}
		return "number"
	case json.Number:
		if _, err := x.Int64(); err == nil {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

// Coerce turns a CLI word into a JSON value the node accepts. Quoted words and JSON literals ({…}, […]) are parsed
// as JSON when that yields an acceptable value; otherwise the word is tried as each allowed type in turn (integer,
// number, boolean, null, string). The first candidate that validates wins; if none does, the error names the first
// validation issue.
func (n *Node) Coerce(word string, quoted bool) (any, error) {
	var candidates []any
	if strings.HasPrefix(word, "{") || strings.HasPrefix(word, "[") || (quoted && strings.HasPrefix(word, "\"")) {
		var v any
		if err := json.Unmarshal([]byte(word), &v); err != nil {
			return nil, fmt.Errorf("invalid JSON value: %v", err)
		}
		candidates = append(candidates, v)
	} else {
		for _, t := range n.Types() {
			switch t {
			case "integer", "number":
				if f, err := strconv.ParseFloat(word, 64); err == nil && !quoted {
					candidates = append(candidates, f)
				}
			case "boolean":
				if (word == "true" || word == "false") && !quoted {
					candidates = append(candidates, word == "true")
				}
			case "null":
				if word == "null" && !quoted {
					candidates = append(candidates, nil)
				}
			case "string":
				candidates = append(candidates, word)
			case "object", "array":
				var v any
				if err := json.Unmarshal([]byte(word), &v); err == nil {
					candidates = append(candidates, v)
				}
			}
		}
		if len(candidates) == 0 {
			candidates = append(candidates, word)
		}
	}
	var first []Issue
	for _, c := range candidates {
		issues := n.Validate(c, "")
		if len(issues) == 0 {
			return c, nil
		}
		if first == nil {
			first = issues
		}
	}
	return nil, IssuesError(first)
}

// Issue is one client-side validation finding (pointer relative to the validated value).
type Issue struct {
	Pointer string
	Message string
}

// IssuesError formats issues as one error.
func IssuesError(issues []Issue) error {
	if len(issues) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(issues))
	for _, i := range issues {
		if i.Pointer != "" {
			msgs = append(msgs, i.Pointer+": "+i.Message)
		} else {
			msgs = append(msgs, i.Message)
		}
	}
	return errors.New(strings.Join(msgs, "; "))
}

var regexCache = map[string]*regexp.Regexp{}

// jsRegex translates the ECMAScript patterns Zod emits into RE2; nil when RE2 cannot express it (then unchecked —
// the API validates again).
func jsRegex(p string) *regexp.Regexp {
	if r, ok := regexCache[p]; ok {
		return r
	}
	conv := regexp.MustCompile(`\\u([0-9a-fA-F]{4})`).ReplaceAllString(p, `\x{$1}`)
	r, err := regexp.Compile(conv)
	if err != nil {
		r = nil
	}
	regexCache[p] = r
	return r
}

// Validate checks v against the node (subset of JSON Schema 2020-12 used by packages/schema: type, enum, const,
// min/max, multipleOf, length, pattern, properties, required, additionalProperties, propertyNames, items, item
// counts, anyOf/oneOf, $ref). Formats are not checked (the schema pairs them with patterns).
func (n *Node) Validate(v any, ptr string) []Issue {
	bs := n.branches()
	var best []Issue
	for _, b := range bs {
		issues := b.validateOne(v, ptr)
		if len(issues) == 0 {
			return nil
		}
		if best == nil || len(issues) < len(best) {
			best = issues
		}
	}
	return best
}

func (n *Node) validateOne(v any, ptr string) []Issue {
	m := n.m
	var out []Issue
	bad := func(format string, a ...any) {
		out = append(out, Issue{Pointer: ptr, Message: fmt.Sprintf(format, a...)})
	}
	vt := jsonType(v)
	if ts := (&Node{d: n.d, m: m}).typesOwn(); len(ts) > 0 {
		ok := false
		for _, t := range ts {
			if t == vt || (t == "number" && vt == "integer") {
				ok = true
			}
		}
		if !ok {
			bad("expected %s, got %s", strings.Join(ts, " or "), vt)
			return out
		}
	}
	if c, ok := m["const"]; ok && !equalJSON(c, v) {
		bad("must be %s", literal(c))
	}
	if e, ok := m["enum"].([]any); ok {
		found := false
		for _, x := range e {
			if equalJSON(x, v) {
				found = true
			}
		}
		if !found {
			vals := make([]string, len(e))
			for i, x := range e {
				vals[i] = literal(x)
			}
			bad("must be one of: %s", strings.Join(vals, ", "))
		}
	}
	switch x := v.(type) {
	case float64:
		if lo, ok := m["minimum"].(float64); ok && x < lo {
			bad("must be ≥ %s", num(lo))
		}
		if hi, ok := m["maximum"].(float64); ok && x > hi {
			bad("must be ≤ %s", num(hi))
		}
		if lo, ok := m["exclusiveMinimum"].(float64); ok && x <= lo {
			bad("must be > %s", num(lo))
		}
		if hi, ok := m["exclusiveMaximum"].(float64); ok && x >= hi {
			bad("must be < %s", num(hi))
		}
		if mo, ok := m["multipleOf"].(float64); ok && mo > 0 {
			if q := x / mo; math.Abs(q-math.Round(q)) > 1e-9 {
				bad("must be a multiple of %s", num(mo))
			}
		}
	case string:
		l := float64(len([]rune(x)))
		if lo, ok := m["minLength"].(float64); ok && l < lo {
			bad("must be at least %s characters", num(lo))
		}
		if hi, ok := m["maxLength"].(float64); ok && l > hi {
			bad("must be at most %s characters", num(hi))
		}
		if p, ok := m["pattern"].(string); ok {
			if r := jsRegex(p); r != nil && !r.MatchString(x) {
				hint := ""
				if h := n.Help(); h != "" {
					hint = " (" + h + ")"
				}
				bad("%q does not have the expected format%s", x, hint)
			}
		}
	case []any:
		if lo, ok := m["minItems"].(float64); ok && float64(len(x)) < lo {
			bad("needs at least %s items", num(lo))
		}
		if hi, ok := m["maxItems"].(float64); ok && float64(len(x)) > hi {
			bad("allows at most %s items", num(hi))
		}
		if it := n.sub(m["items"]); it != nil {
			for i, e := range x {
				out = append(out, it.Validate(e, ptr+"/"+strconv.Itoa(i))...)
			}
		}
	case map[string]any:
		props, _ := m["properties"].(map[string]any)
		if req, ok := m["required"].([]any); ok {
			for _, r := range req {
				if s, ok := r.(string); ok {
					if _, present := x[s]; !present {
						bad("missing required %q", s)
					}
				}
			}
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ap := m["additionalProperties"]
		for _, k := range keys {
			kp := ptr + "/" + strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
			if pv, ok := props[k]; ok {
				if c := n.sub(pv); c != nil {
					out = append(out, c.Validate(x[k], kp)...)
				}
				continue
			}
			if pn := n.sub(m["propertyNames"]); pn != nil {
				if is := pn.Validate(k, kp); len(is) > 0 {
					out = append(out, Issue{Pointer: kp, Message: "invalid name: " + is[0].Message})
				}
			}
			switch a := ap.(type) {
			case bool:
				if !a {
					names := make([]string, 0, len(props))
					for p := range props {
						names = append(names, p)
					}
					sort.Strings(names)
					out = append(out, Issue{Pointer: kp, Message: fmt.Sprintf("unknown field %q (expected: %s)", k, strings.Join(names, ", "))})
				}
			case map[string]any:
				if c := n.sub(a); c != nil {
					out = append(out, c.Validate(x[k], kp)...)
				}
			}
		}
	}
	// anyOf/oneOf never reach here: Validate expanded them through branches().
	return out
}

// typesOwn are the types declared on this very schema object (no combinator expansion).
func (n *Node) typesOwn() []string {
	switch t := n.m["type"].(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func equalJSON(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
