package objects

import (
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// objectsDoc parses an `objects` document (protobuf JSON = the API's JSON).
func objectsDoc(t *testing.T, js string) *vrxv1.ObjectsConfig {
	t.Helper()
	d := &vrxv1.ObjectsConfig{}
	if err := protojson.Unmarshal([]byte(js), d); err != nil {
		t.Fatalf("doc: %v", err)
	}
	return d
}

func prefixes(ps []netip.Prefix) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.String()
	}
	return strings.Join(s, " ")
}

const nestedDoc = `{
  "addresses": {
    "web1":   {"type": "host", "address": "192.0.2.10"},
    "web2":   {"type": "host", "address": "192.0.2.11"},
    "web6":   {"type": "host", "address": "2001:db8::10"},
    "lan":    {"type": "network", "prefix": "10.3.1.7/24"},
    "pool":   {"type": "range", "start": "10.3.2.1", "end": "10.3.2.20"},
    "pool6":  {"type": "range", "start": "2001:db8:1::", "end": "2001:db8:1::ff"},
    "cdn":    {"type": "fqdn", "fqdn": "cdn.w3.test"},
    "ghost":  {"type": "fqdn", "fqdn": "ghost.w3.test"}
  },
  "addressGroups": {
    "web-servers": {"members": ["web1", "web2", "web6"]},
    "dmz":         {"members": ["web-servers", "lan", "web1"]},
    "all":         {"members": ["dmz", "pool", "pool6", "cdn", "ghost", "web-servers"]},
    "empty":       {"members": []},
    "outer":       {"members": ["empty"]}
  }
}`

func TestExpandNestedGroupsSplitAndDeterministic(t *testing.T) {
	doc := objectsDoc(t, nestedDoc)
	fqdn := func(name string) ([]netip.Addr, bool) {
		if name == "cdn" {
			return []netip.Addr{netip.MustParseAddr("2001:db8::53"), netip.MustParseAddr("198.51.100.7")}, true
		}
		return nil, false
	}
	cases := []struct{ ref, v4, v6, unresolved string }{
		{"web1", "192.0.2.10/32", "", ""},
		{"lan", "10.3.1.0/24", "", ""}, // host bits masked
		// 192.0.2.10 and .11 are siblings: aggregated into one /31
		{"web-servers", "192.0.2.10/31", "2001:db8::10/128", ""},
		// web1 twice (directly and through web-servers): deduplicated
		{"dmz", "10.3.1.0/24 192.0.2.10/31", "2001:db8::10/128", ""},
		{"all", "10.3.1.0/24 10.3.2.1/32 10.3.2.2/31 10.3.2.4/30 10.3.2.8/29 10.3.2.16/30 10.3.2.20/32 192.0.2.10/31 198.51.100.7/32",
			"2001:db8::10/128 2001:db8::53/128 2001:db8:1::/120", "ghost"},
		{"empty", "", "", ""},
		{"outer", "", "", ""},
	}
	for _, c := range cases {
		got, err := Expand(doc, c.ref, WithFQDN(fqdn))
		if err != nil {
			t.Fatalf("%s: %v", c.ref, err)
		}
		if prefixes(got.V4) != c.v4 || prefixes(got.V6) != c.v6 || strings.Join(got.Unresolved, " ") != c.unresolved {
			t.Errorf("%s:\n v4 %q want %q\n v6 %q want %q\n unresolved %v want %q", c.ref, prefixes(got.V4), c.v4, prefixes(got.V6), c.v6, got.Unresolved, c.unresolved)
		}
		again, _ := Expand(doc, c.ref, WithFQDN(fqdn))
		if !reflect.DeepEqual(got, again) {
			t.Errorf("%s: not deterministic", c.ref)
		}
		t.Logf("Expand(%s) v4=[%s] v6=[%s] unresolved=%v", c.ref, prefixes(got.V4), prefixes(got.V6), got.Unresolved)
	}
	// without an FQDN lookup every fqdn object is unresolved (a warning, never an error)
	got, err := Expand(doc, "all")
	if err != nil || strings.Join(got.Unresolved, ",") != "cdn,ghost" || got.Len() != 10 {
		t.Fatalf("no lookup: %v %+v", err, got)
	}
	if all := prefixes(got.All()); !strings.HasPrefix(all, "10.3.1.0/24 ") || !strings.HasSuffix(all, " 2001:db8:1::/120") {
		t.Fatalf("All() order: %s", all)
	}
}

func TestExpandErrors(t *testing.T) {
	doc := objectsDoc(t, `{
	  "addresses": {"a": {"type": "host", "address": "192.0.2.1"}, "bad": {"type": "host", "address": "fe80::1%eth0"}},
	  "addressGroups": {"g1": {"members": ["a", "g2"]}, "g2": {"members": ["g3"]}, "g3": {"members": ["g1"]},
	                    "dangling": {"members": ["nosuch"]}}
	}`)
	if _, err := Expand(doc, "nosuch"); !errors.Is(err, ErrUnknownObject) {
		t.Fatalf("unknown: %v", err)
	}
	_, err := Expand(doc, "g1")
	if !errors.Is(err, ErrCycle) || !strings.Contains(err.Error(), "g1 → g2 → g3 → g1") {
		t.Fatalf("cycle: %v", err)
	}
	t.Logf("cycle: %v", err)
	if _, err := Expand(doc, "dangling"); !errors.Is(err, ErrUnknownObject) {
		t.Fatalf("dangling member: %v", err)
	}
	if _, err := Expand(doc, "bad"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zone: %v", err)
	}
}

// The cap: a group of 10 001 distinct, non-adjacent hosts is refused; 10 000 are fine.
func TestExpandCapExceeded(t *testing.T) {
	doc := &vrxv1.ObjectsConfig{Addresses: map[string]*vrxv1.AddressObject{}, AddressGroups: map[string]*vrxv1.AddressGroup{}}
	var members []string
	for i := 0; i <= MaxEntries; i++ { // 10 001 hosts 10.<i/128>.<…>.<2(i%128)>: every other address, never siblings
		name := fmt.Sprintf("h%05d", i)
		a := netip.AddrFrom4([4]byte{10, byte(i / 32768), byte(i / 128 % 256), byte(i % 128 * 2)})
		doc.Addresses[name] = &vrxv1.AddressObject{Type: ptr("host"), Address: ptr(a.String())}
		members = append(members, name)
	}
	doc.AddressGroups["big"] = &vrxv1.AddressGroup{Members: members}
	doc.AddressGroups["fits"] = &vrxv1.AddressGroup{Members: members[:MaxEntries]}
	_, err := Expand(doc, "big")
	var le *LimitError
	if !errors.As(err, &le) || le.Count != MaxEntries+1 || le.Limit != MaxEntries || le.Ref != "big" {
		t.Fatalf("cap: %v", err)
	}
	t.Logf("cap exceeded: %v", err)
	got, err := Expand(doc, "fits")
	if err != nil || got.Len() != MaxEntries {
		t.Fatalf("10 000 entries must fit: %v %d", err, got.Len())
	}
	if _, err := Expand(doc, "fits", WithLimit(10)); !errors.As(err, &le) || le.Limit != 10 {
		t.Fatalf("WithLimit: %v", err)
	}
	if err := CheckLimit("acl web-in rule 10", 3*4000); !errors.As(err, &le) || le.Count != 12000 {
		t.Fatalf("CheckLimit: %v", err)
	}
	if CheckLimit("r", MaxEntries) != nil {
		t.Fatal("CheckLimit at the limit")
	}
}

func ptr[T any](v T) *T { return &v }

func TestRangeToPrefixes(t *testing.T) {
	cases := []struct{ start, end, want string }{
		{"10.0.0.0", "10.0.0.255", "10.0.0.0/24"},
		{"10.0.0.1", "10.0.0.1", "10.0.0.1/32"},
		{"10.0.0.1", "10.0.0.6", "10.0.0.1/32 10.0.0.2/31 10.0.0.4/31 10.0.0.6/32"},
		{"192.168.0.255", "192.168.2.0", "192.168.0.255/32 192.168.1.0/24 192.168.2.0/32"},
		{"0.0.0.0", "255.255.255.255", "0.0.0.0/0"},
		{"255.255.255.254", "255.255.255.255", "255.255.255.254/31"},
		{"2001:db8::", "2001:db8::ffff", "2001:db8::/112"},
		{"2001:db8::1", "2001:db8::3", "2001:db8::1/128 2001:db8::2/127"},
		{"::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "::/0"},
	}
	for _, c := range cases {
		got, err := RangeToPrefixes(netip.MustParseAddr(c.start), netip.MustParseAddr(c.end))
		if err != nil || prefixes(got) != c.want {
			t.Errorf("%s–%s = %s (%v), want %s", c.start, c.end, prefixes(got), err, c.want)
		}
		t.Logf("range %s–%s → %s", c.start, c.end, prefixes(got))
	}
	for _, bad := range [][2]string{{"10.0.0.2", "10.0.0.1"}, {"10.0.0.1", "2001:db8::1"}} {
		if _, err := RangeToPrefixes(netip.MustParseAddr(bad[0]), netip.MustParseAddr(bad[1])); err == nil {
			t.Errorf("%v: want an error", bad)
		}
	}
}

// Range → CIDR is exact: the prefixes cover every address of the range and nothing else (checked
// address by address on a few awkward ranges).
func TestRangeToPrefixesExact(t *testing.T) {
	for _, c := range [][2]string{{"10.1.2.3", "10.1.5.250"}, {"172.16.0.7", "172.16.1.8"}} {
		start, end := netip.MustParseAddr(c[0]), netip.MustParseAddr(c[1])
		ps, err := RangeToPrefixes(start, end)
		if err != nil {
			t.Fatal(err)
		}
		in := func(a netip.Addr) bool {
			for _, p := range ps {
				if p.Contains(a) {
					return true
				}
			}
			return false
		}
		for a := start.Prev().Prev(); a.Compare(end.Next().Next()) <= 0; a = a.Next() {
			want := a.Compare(start) >= 0 && a.Compare(end) <= 0
			if in(a) != want {
				t.Fatalf("%s–%s: %s covered=%v", c[0], c[1], a, in(a))
			}
		}
		for i := 1; i < len(ps); i++ { // minimal: no two neighbours could merge
			if _, ok := siblings(ps[i-1], ps[i]); ok {
				t.Fatalf("not minimal: %s", prefixes(ps))
			}
		}
	}
}

func TestAggregate(t *testing.T) {
	in := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/25"), netip.MustParsePrefix("10.0.0.128/25"), // → 10.0.0.0/24
		netip.MustParsePrefix("10.0.1.0/24"),   // → with the above: 10.0.0.0/23
		netip.MustParsePrefix("10.0.0.77/32"),  // covered
		netip.MustParsePrefix("10.0.4.0/24"),   // alone
		netip.MustParsePrefix("2001:db8::/48"), //
		netip.MustParsePrefix("2001:db8::1/128"),
		netip.MustParsePrefix("10.0.4.0/24"), // duplicate
	}
	if got := prefixes(aggregate(in)); got != "10.0.0.0/23 10.0.4.0/24 2001:db8::/48" {
		t.Fatalf("aggregate: %s", got)
	}
}

func TestZoneInterfaces(t *testing.T) {
	doc := objectsDoc(t, `{"zones": {"lan": {"interfaces": ["host-w3l0", "host-w3l0.100", "GigabitEthernet0/8/0"]}}}`)
	got, err := ZoneInterfaces(doc, "lan")
	if err != nil || strings.Join(got, ",") != "GigabitEthernet0/8/0,host-w3l0,host-w3l0.100" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := ZoneInterfaces(doc, "wan"); !errors.Is(err, ErrUnknownObject) {
		t.Fatal(err)
	}
}
