package render

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ngfw/cli/internal/cpath"
)

const doc = `{
  "system": {"hostname": "edge-1", "dns": {"servers": ["1.1.1.1", "9.9.9.9"], "searchDomains": []}},
  "interfaces": {
    "TenGigabitEthernet0/0/0": {"enabled": true, "mtu": 9000, "ipv4": ["10.0.0.1/24"], "description": "uplink to core"},
    "loop0": {"enabled": false, "subinterfaces": {}}
  },
  "routing": {"static": [{"prefix": "0.0.0.0/0", "nextHops": [{"address": "10.0.0.254"}]}]}
}`

func parse(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTextIsStableAndHierarchical(t *testing.T) {
	want := `interfaces {
    TenGigabitEthernet0/0/0 {
        description "uplink to core";
        enabled true;
        ipv4 [ 10.0.0.1/24 ];
        mtu 9000;
    }
    loop0 {
        enabled false;
        subinterfaces { }
    }
}
routing {
    static 0 {
        nextHops 0 {
            address 10.0.0.254;
        }
        prefix 0.0.0.0/0;
    }
}
system {
    dns {
        searchDomains [ ];
        servers [ 1.1.1.1 9.9.9.9 ];
    }
    hostname edge-1;
}
`
	v := parse(t, doc)
	got := Text(v)
	if got != want {
		t.Errorf("Text:\n%s\nwant:\n%s", got, want)
	}
	for i := 0; i < 20; i++ { // map order must not leak into the output
		if Text(parse(t, doc)) != got {
			t.Fatal("Text is not deterministic")
		}
	}
}

func TestSetLines(t *testing.T) {
	want := []string{
		`set interfaces TenGigabitEthernet0/0/0 description "uplink to core"`,
		`set interfaces TenGigabitEthernet0/0/0 enabled true`,
		`set interfaces TenGigabitEthernet0/0/0 ipv4 10.0.0.1/24`,
		`set interfaces TenGigabitEthernet0/0/0 mtu 9000`,
		`set interfaces loop0 enabled false`,
		`set interfaces loop0 subinterfaces {}`,
		`set routing static 0 nextHops 0 address 10.0.0.254`,
		`set routing static 0 prefix 0.0.0.0/0`,
		`set system dns searchDomains []`,
		`set system dns servers 1.1.1.1`,
		`set system dns servers 9.9.9.9`,
		`set system hostname edge-1`,
	}
	got := Set(nil, parse(t, doc))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Set:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// at a path: the prefix is part of every line
	sub := Set([]string{"system", "dns"}, parse(t, `{"servers": ["1.1.1.1"]}`))
	if !reflect.DeepEqual(sub, []string{"set system dns servers 1.1.1.1"}) {
		t.Errorf("Set at a path: %q", sub)
	}
}

// Every set line tokenizes back into path words + one value word that equals the rendered leaf.
func TestSetLinesTokenizeBack(t *testing.T) {
	v := parse(t, `{"system": {"banner": {"motd": "hello \"world\"\nline 2"}, "hostname": "a b"}}`)
	for _, l := range Set(nil, v) {
		toks, err := cpath.Tokenize(l)
		if err != nil {
			t.Fatalf("%s: %v", l, err)
		}
		words := cpath.Texts(toks)
		segs := words[1 : len(words)-1]
		leaf := v
		for _, s := range segs {
			leaf = leaf.(map[string]any)[s]
		}
		if words[len(words)-1] != leaf {
			t.Errorf("%s: value %q, want %q", l, words[len(words)-1], leaf)
		}
	}
}

func TestDiff(t *testing.T) {
	changes := []Change{
		{Op: "replace", Pointer: "/interfaces/loop301/mtu", From: 1500.0, To: 9000.0},
		{Op: "add", Pointer: "/interfaces/TenGigabitEthernet0~10~10", To: map[string]any{"enabled": true}},
		{Op: "remove", Pointer: "/system/dns/servers/1", From: "9.9.9.9"},
	}
	want := []string{
		"- set interfaces loop301 mtu 1500",
		"+ set interfaces loop301 mtu 9000",
		"+ set interfaces TenGigabitEthernet0/0/0 enabled true",
		"- set system dns servers 1 9.9.9.9",
	}
	if got := Diff(changes); !reflect.DeepEqual(got, want) {
		t.Errorf("Diff:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestScalar(t *testing.T) {
	for v, want := range map[any]string{"x y": `"x y"`, 1500.0: "1500", 0.5: "0.5", true: "true", nil: "null", "": `""`} {
		if got := Scalar(v); got != want {
			t.Errorf("Scalar(%v) = %s, want %s", v, got, want)
		}
	}
}
