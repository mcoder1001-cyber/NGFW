package jschema

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// testdata: RootConfig (interfaces + system only), InterfacesConfig and SystemConfig as the API publishes them.
func load(t *testing.T) *Doc {
	t.Helper()
	b, err := os.ReadFile("../testdata/openapi-min.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Load(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWalkPropertiesMapsAndArrays(t *testing.T) {
	d := load(t)
	root := d.Root()
	var top []string
	for _, p := range root.Properties() {
		top = append(top, p.Name)
	}
	if !reflect.DeepEqual(top, []string{"system", "dataplane", "interfaces"}) { // x-vrx-ui order of the domain components
		t.Errorf("root properties %q", top)
	}
	ifs, err := root.Walk([]string{"interfaces"})
	if err != nil || ifs.MapValue() == nil || ifs.KeyNode() == nil {
		t.Fatalf("interfaces is a keyed map: %v", err)
	}
	mtu, err := root.Walk([]string{"interfaces", "TenGigabitEthernet0/0/0", "mtu"})
	if err != nil {
		t.Fatal(err)
	}
	if mtu.IsContainer() || !mtu.Has("integer") || mtu.TypeHint() != "integer 68..9216" {
		t.Errorf("mtu: types %v hint %q", mtu.Types(), mtu.TypeHint())
	}
	ipv4, _ := root.Walk([]string{"interfaces", "loop0", "ipv4"})
	if !ipv4.IsLeafList() {
		t.Error("ipv4 is a list of scalars")
	}
	if _, err := root.Walk([]string{"interfaces", "loop0", "ipv4", "0"}); err != nil {
		t.Errorf("array index: %v", err)
	}
	// errors name the valid alternatives / the key rule
	_, err = root.Walk([]string{"interfaces", "loop0", "speed"})
	if err == nil || !strings.Contains(err.Error(), "expected one of: enabled, description, mtu") {
		t.Errorf("unknown property: %v", err)
	}
	_, err = root.Walk([]string{"interfaces", "9bad", "mtu"})
	if err == nil || !strings.Contains(err.Error(), "not a valid Interface") {
		t.Errorf("bad map key: %v", err)
	}
	_, err = root.Walk([]string{"interfaces", "loop0", "ipv4", "x"})
	if err == nil {
		t.Error("non-numeric array index must fail")
	}
}

func TestPropertiesFollowUIOrder(t *testing.T) {
	iface, _ := load(t).Root().Walk([]string{"interfaces", "loop0"})
	var names []string
	for _, p := range iface.Properties() {
		names = append(names, p.Name)
	}
	if len(names) < 3 || names[0] != "enabled" || names[1] != "description" || names[2] != "mtu" {
		t.Errorf("order %q", names)
	}
}

func TestCoerce(t *testing.T) {
	root := load(t).Root()
	node := func(p ...string) *Node {
		n, err := root.Walk(p)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	cases := []struct {
		n      *Node
		word   string
		quoted bool
		want   any
	}{
		{node("interfaces", "l0", "mtu"), "9000", false, 9000.0},
		{node("interfaces", "l0", "enabled"), "true", false, true},
		{node("interfaces", "l0", "rxMode"), "interrupt", false, "interrupt"},
		{node("interfaces", "l0", "description"), "1500", false, "1500"}, // a string field keeps digits as text
		{node("interfaces", "l0", "ipv4"), `["10.0.0.1/24"]`, true, []any{"10.0.0.1/24"}},
		{node("interfaces", "l0", "ipv4").Items(), "10.0.0.1/24", false, "10.0.0.1/24"},
		{node("system", "hostname"), "edge-1", false, "edge-1"},
	}
	for _, c := range cases {
		got, err := c.n.Coerce(c.word, c.quoted)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Coerce(%q) = %#v, %v; want %#v", c.word, got, err, c.want)
		}
	}
	bad := []struct {
		n    *Node
		word string
		msg  string
	}{
		{node("interfaces", "l0", "mtu"), "70000", "must be ≤ 9216"},
		{node("interfaces", "l0", "mtu"), "big", "expected integer"},
		{node("interfaces", "l0", "mtu"), "1500.5", "expected integer"},
		{node("interfaces", "l0", "enabled"), "yes", "expected boolean"},
		{node("interfaces", "l0", "rxMode"), "turbo", "must be one of: polling, interrupt, adaptive"},
		{node("interfaces", "l0", "ipv4").Items(), "10.0.0.300/24", "does not have the expected format"},
		{node("interfaces", "l0", "description"), "bell\x07", "does not have the expected format"}, // \u escapes in JS patterns
		{node("interfaces", "l0", "description"), strings.Repeat("x", 256), "at most 255 characters"},
	}
	for _, c := range bad {
		if _, err := c.n.Coerce(c.word, false); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("Coerce(%q): want %q, got %v", c.word, c.msg, err)
		}
	}
}

func TestValidateObjects(t *testing.T) {
	iface, _ := load(t).Root().Walk([]string{"interfaces", "loop0"})
	if issues := iface.Validate(map[string]any{"enabled": true, "mtu": 1500.0, "ipv4": []any{"10.0.0.1/24"}}, ""); len(issues) != 0 {
		t.Errorf("valid interface: %v", issues)
	}
	issues := iface.Validate(map[string]any{"mtu": 10.0, "colour": "red", "ipv4": []any{"nope"}}, "/interfaces/loop0")
	msgs := IssuesError(issues).Error()
	for _, want := range []string{"/interfaces/loop0/mtu: must be ≥ 68", `/interfaces/loop0/colour: unknown field "colour"`, "/interfaces/loop0/ipv4/0:"} {
		if !strings.Contains(msgs, want) {
			t.Errorf("missing %q in %s", want, msgs)
		}
	}
	ifs, _ := load(t).Root().Walk([]string{"interfaces"})
	if is := ifs.Validate(map[string]any{"bad name!": map[string]any{}}, ""); len(is) == 0 {
		t.Error("an invalid map key must be reported (propertyNames)")
	}
}

func TestEnumAndHelp(t *testing.T) {
	rx, _ := load(t).Root().Walk([]string{"interfaces", "l0", "rxMode"})
	if got := rx.Enum(); !reflect.DeepEqual(got, []string{"polling", "interrupt", "adaptive"}) {
		t.Errorf("enum %q", got)
	}
	en, _ := load(t).Root().Walk([]string{"interfaces", "l0", "enabled"})
	if got := en.Enum(); !reflect.DeepEqual(got, []string{"true", "false"}) {
		t.Errorf("boolean values %q", got)
	}
	if !strings.Contains(rx.Help(), "polling") || rx.Title() != "RX mode" {
		t.Errorf("help/title: %q %q", rx.Help(), rx.Title())
	}
}

func TestLoadNeedsRootConfig(t *testing.T) {
	if _, err := Load([]byte(`{"components":{"schemas":{}}}`)); err == nil {
		t.Error("want an error without RootConfig")
	}
}
