package provider

import (
	"strings"
	"testing"
)

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

func TestMergeSensitiveByKeyNeverByIndex(t *testing.T) {
	v, _ := decodeJSON(`[{"username":"admin","role":"admin"},{"username":"ops","role":"operator"}]`)
	sv, _ := decodeJSON(`[{"username":"ops","passwordHash":"x"}]`)
	m, err := mergeSensitive(v, sv, "/management/users")
	if err != nil {
		t.Fatal(err)
	}
	if canonicalJSON(m) != `[{"role":"admin","username":"admin"},{"passwordHash":"x","role":"operator","username":"ops"}]` {
		t.Fatal(canonicalJSON(m))
	}
	for _, bad := range []struct{ sv, want string }{
		{`[{"passwordHash":"x"}]`, `must name their "username"`},
		{`[{"username":"gone","passwordHash":"x"}]`, `username = gone, which is not in value`},
	} {
		s, _ := decodeJSON(bad.sv)
		if _, err := mergeSensitive(v, s, "/management/users"); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%s: %v", bad.sv, err)
		}
	}
	// unkeyed arrays must mirror value element by element
	a, _ := decodeJSON(`{"list":[1,2]}`)
	b, _ := decodeJSON(`{"list":[{"k":1}]}`)
	if _, err := mergeSensitive(a, b, "/x"); err == nil || !strings.Contains(err.Error(), "must mirror value") {
		t.Fatal(err)
	}
}

func TestWriteOnlyLeaves(t *testing.T) {
	m, _ := decodeJSON(`[{"username":"a","passwordHash":"x"},{"username":"b"}]`)
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
