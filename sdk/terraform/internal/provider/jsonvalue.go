package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// decodeJSON parses a JSON document keeping numbers exact (json.Number).
func decodeJSON(s string) (any, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if d.More() {
		return nil, fmt.Errorf("trailing data after the JSON value")
	}
	return normalizeNumbers(v), nil
}

// normalizeNumbers turns json.Number / float64 into canonical json.Number so equal numbers compare equal.
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalizeNumbers(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = normalizeNumbers(e)
		}
		return x
	case json.Number:
		f, ok := new(big.Float).SetString(x.String())
		if !ok {
			return x
		}
		return json.Number(f.Text('g', -1))
	case float64:
		return json.Number(strconv.FormatFloat(x, 'g', -1, 64))
	}
	return v
}

// canonicalJSON renders v with sorted keys and no insignificant whitespace.
func canonicalJSON(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

func jsonEqual(a, b any) bool { return reflect.DeepEqual(normalizeNumbers(a), normalizeNumbers(b)) }

// jsonSubset: every member of want is present in have with a subset value; arrays must have the same length and
// element-wise subsets; scalars must be equal. Used by Read: the API fills schema defaults into what it stores (also
// inside array elements, e.g. management.users[].scope), so `have` may carry members the user never set.
func jsonSubset(want, have any) bool {
	if wl, ok := want.([]any); ok {
		hl, ok := have.([]any)
		if !ok || len(wl) != len(hl) {
			return false
		}
		for i := range wl {
			if !jsonSubset(wl[i], hl[i]) {
				return false
			}
		}
		return true
	}
	wm, ok := want.(map[string]any)
	if !ok {
		return jsonEqual(want, have)
	}
	hm, ok := have.(map[string]any)
	if !ok {
		return false
	}
	for k, wv := range wm {
		hv, ok := hm[k]
		if !ok || !jsonSubset(wv, hv) {
			return false
		}
	}
	return true
}

// mergeSensitive overlays the write-only `sensitive_value` onto `value` (both located at ptr). Objects merge
// member-wise. Arrays listed in keyedArrayPointers (x-vrx-ui.itemKey, e.g. management.users by username) merge by
// that key — a sensitive element must name its key and match exactly one element of value; other arrays must have
// the same length and merge index-wise. Anything that cannot be matched is an error: a hash must never attach to the
// wrong element (the plan cannot show a write-only value).
func mergeSensitive(dst, src any, ptr string) (any, error) {
	switch s := src.(type) {
	case map[string]any:
		d, ok := dst.(map[string]any)
		if !ok {
			if dst != nil {
				return nil, fmt.Errorf("%s: sensitive_value has an object where value has %T", orRoot(ptr), dst)
			}
			d = map[string]any{}
		}
		out := make(map[string]any, len(d)+len(s))
		for k, v := range d {
			out[k] = v
		}
		for k, v := range s {
			m, err := mergeSensitive(d[k], v, ptr+"/"+escapeToken(k))
			if err != nil {
				return nil, err
			}
			out[k] = m
		}
		return out, nil
	case []any:
		d, ok := dst.([]any)
		if !ok {
			return nil, fmt.Errorf("%s: sensitive_value has an array where value has %T", orRoot(ptr), dst)
		}
		out := append([]any(nil), d...)
		if key := keyedArrayKey(ptr); key != "" {
			for i, e := range s {
				em, _ := e.(map[string]any)
				kv, has := em[key]
				if !has {
					return nil, fmt.Errorf("%s/%d: sensitive_value elements of this array must name their %q to be matched", orRoot(ptr), i, key)
				}
				match := -1
				for j, de := range d {
					if dm, ok := de.(map[string]any); ok && jsonEqual(dm[key], kv) {
						if match >= 0 {
							return nil, fmt.Errorf("%s: two elements of value have %s = %v", orRoot(ptr), key, kv)
						}
						match = j
					}
				}
				if match < 0 {
					return nil, fmt.Errorf("%s: sensitive_value names %s = %v, which is not in value", orRoot(ptr), key, kv)
				}
				m, err := mergeSensitive(out[match], e, fmt.Sprintf("%s/%d", ptr, match))
				if err != nil {
					return nil, err
				}
				out[match] = m
			}
			return out, nil
		}
		if len(s) != len(d) {
			return nil, fmt.Errorf("%s: sensitive_value has %d elements but value has %d — it must mirror value element by element", orRoot(ptr), len(s), len(d))
		}
		for i, e := range s {
			m, err := mergeSensitive(out[i], e, fmt.Sprintf("%s/%d", ptr, i))
			if err != nil {
				return nil, err
			}
			out[i] = m
		}
		return out, nil
	}
	return src, nil
}

func keyedArrayKey(ptr string) string {
	for pat, key := range keyedArrayPointers {
		if matchPointer(pat, ptr) {
			return key
		}
	}
	return ""
}

func escapeToken(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
}

// matchPointer: `/a/*/c` matches `/a/0/c`.
func matchPointer(pattern, ptr string) bool {
	ps, xs := strings.Split(pattern, "/"), strings.Split(ptr, "/")
	if len(ps) != len(xs) {
		return false
	}
	for i := range ps {
		if ps[i] != "*" && ps[i] != xs[i] {
			return false
		}
	}
	return true
}

// writeOnlyLeaves lists the pointers of write-only leaves inside v (located at base).
func writeOnlyLeaves(v any, base string) []string {
	var out []string
	var walk func(any, string)
	walk = func(n any, p string) {
		for _, pat := range writeOnlyPointers {
			if matchPointer(pat, p) {
				out = append(out, p)
				return
			}
		}
		switch x := n.(type) {
		case map[string]any:
			for k, e := range x {
				walk(e, p+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"))
			}
		case []any:
			for i, e := range x {
				walk(e, p+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(v, base)
	return out
}

// ---- tftypes ↔ JSON for the typed resources (names: attribute → JSON member)

func tfToJSON(v tftypes.Value, names map[string]string, skip map[string]bool) (any, error) {
	if v.IsNull() {
		return nil, nil
	}
	if !v.IsKnown() {
		return nil, fmt.Errorf("value is not known yet")
	}
	t := v.Type()
	switch {
	case t.Is(tftypes.Object{}):
		var m map[string]tftypes.Value
		if err := v.As(&m); err != nil {
			return nil, err
		}
		out := map[string]any{}
		for k, e := range m {
			if skip[k] || e.IsNull() {
				continue
			}
			j, err := tfToJSON(e, names, nil)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[jsonName(k, names)] = j
		}
		return out, nil
	case t.Is(tftypes.Map{}):
		var m map[string]tftypes.Value
		if err := v.As(&m); err != nil {
			return nil, err
		}
		out := map[string]any{}
		for k, e := range m {
			j, err := tfToJSON(e, names, nil)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = j
		}
		return out, nil
	case t.Is(tftypes.List{}), t.Is(tftypes.Set{}), t.Is(tftypes.Tuple{}):
		var l []tftypes.Value
		if err := v.As(&l); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(l))
		for i, e := range l {
			j, err := tfToJSON(e, names, nil)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out = append(out, j)
		}
		return out, nil
	case t.Is(tftypes.String):
		var s string
		return s, v.As(&s)
	case t.Is(tftypes.Bool):
		var b bool
		return b, v.As(&b)
	case t.Is(tftypes.Number):
		var f big.Float
		if err := v.As(&f); err != nil {
			return nil, err
		}
		return json.Number(f.Text('g', -1)), nil
	}
	return nil, fmt.Errorf("unsupported type %s", t)
}

func jsonToTF(t tftypes.Type, j any, names map[string]string) (tftypes.Value, error) {
	if j == nil {
		return tftypes.NewValue(t, nil), nil
	}
	switch {
	case t.Is(tftypes.Object{}):
		ot := t.(tftypes.Object)
		m, ok := j.(map[string]any)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("expected an object, got %T", j)
		}
		vals := map[string]tftypes.Value{}
		for k, at := range ot.AttributeTypes {
			v, err := jsonToTF(at, m[jsonName(k, names)], names)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("%s: %w", k, err)
			}
			vals[k] = v
		}
		return tftypes.NewValue(t, vals), nil
	case t.Is(tftypes.Map{}):
		et := t.(tftypes.Map).ElementType
		m, ok := j.(map[string]any)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("expected an object, got %T", j)
		}
		vals := map[string]tftypes.Value{}
		for k, e := range m {
			v, err := jsonToTF(et, e, names)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("%s: %w", k, err)
			}
			vals[k] = v
		}
		return tftypes.NewValue(t, vals), nil
	case t.Is(tftypes.List{}):
		et := t.(tftypes.List).ElementType
		l, ok := j.([]any)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("expected an array, got %T", j)
		}
		vals := make([]tftypes.Value, 0, len(l))
		for i, e := range l {
			v, err := jsonToTF(et, e, names)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("[%d]: %w", i, err)
			}
			vals = append(vals, v)
		}
		return tftypes.NewValue(t, vals), nil
	case t.Is(tftypes.String):
		s, ok := j.(string)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("expected a string, got %T", j)
		}
		return tftypes.NewValue(t, s), nil
	case t.Is(tftypes.Bool):
		b, ok := j.(bool)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("expected a boolean, got %T", j)
		}
		return tftypes.NewValue(t, b), nil
	case t.Is(tftypes.Number):
		var s string
		switch n := j.(type) {
		case json.Number:
			s = n.String()
		case float64:
			s = strconv.FormatFloat(n, 'g', -1, 64)
		default:
			return tftypes.Value{}, fmt.Errorf("expected a number, got %T", j)
		}
		f, ok := new(big.Float).SetString(s)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("bad number %q", s)
		}
		return tftypes.NewValue(t, f), nil
	}
	return tftypes.Value{}, fmt.Errorf("unsupported type %s", t)
}

func jsonName(attr string, names map[string]string) string {
	if n, ok := names[attr]; ok {
		return n
	}
	return attr
}
