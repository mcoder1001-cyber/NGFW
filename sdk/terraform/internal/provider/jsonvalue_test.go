package provider

import "testing"

func TestJSONSubset(t *testing.T) {
	cases := []struct {
		want, have string
		ok         bool
	}{
		{`{"a":1}`, `{"a":1.0,"b":2}`, true},
		{`{"a":{"x":1}}`, `{"a":{"x":1,"y":[]}}`, true},
		{`[{"u":"a"}]`, `[{"u":"a","scope":"*"}]`, true},
		{`[{"u":"a"}]`, `[{"u":"a"},{"u":"b"}]`, false},
		{`["x"]`, `["x","y"]`, false},
		{`{"a":1}`, `{"a":2}`, false},
		{`{"a":1}`, `{"b":1}`, false},
		{`"s"`, `"s"`, true},
	}
	for _, c := range cases {
		w, _ := decodeJSON(c.want)
		h, _ := decodeJSON(c.have)
		if got := jsonSubset(w, h); got != c.ok {
			t.Errorf("jsonSubset(%s, %s) = %v", c.want, c.have, got)
		}
	}
}

func TestDeepMergeAndWriteOnlyLeaves(t *testing.T) {
	v, _ := decodeJSON(`[{"username":"a","role":"admin"},{"username":"b"}]`)
	sv, _ := decodeJSON(`[{"passwordHash":"x"}]`)
	m := deepMerge(v, sv)
	if canonicalJSON(m) != `[{"passwordHash":"x","role":"admin","username":"a"},{"username":"b"}]` {
		t.Fatal(canonicalJSON(m))
	}
	if got := writeOnlyLeaves(m, "/management/users"); len(got) != 1 || got[0] != "/management/users/0/passwordHash" {
		t.Fatal(got)
	}
	if got := writeOnlyLeaves(map[string]any{"passwordHash": "x"}, "/management/users/3"); len(got) != 1 {
		t.Fatal(got)
	}
	if got := writeOnlyLeaves(map[string]any{"passwordHash": "x"}, "/interfaces/loop1"); len(got) != 0 {
		t.Fatal(got)
	}
}
