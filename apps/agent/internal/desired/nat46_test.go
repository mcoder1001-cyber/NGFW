package desired_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/desired"
)

const nat46Doc = `{"nat46": {"clientPrefix": "64:ff9b::/96", "interfaces": ["loop460", "loop461"],
  "mappings": [{"name": "dns", "ipv4": "198.51.100.53", "ipv6": "2001:db8:46::53"},
               {"name": "web", "ipv4": "198.51.100.10", "ipv6": "2001:db8:46::10", "mtu": 1500}]}}`

func TestNat46Projection(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, nat46Doc), vrfID)
	want := "map.domain/nat46-dns,map.domain/nat46-web,map.interface/loop460/map-t,map.interface/loop461/map-t"
	if got := strings.Join(s.keys(), ","); got != want || len(s.errs) != 0 {
		t.Fatalf("keys %s errs %v", got, s.errs)
	}
}

// A map-t interface listed by nat.map and nat.nat46 is one object: nat.map emits it, NAT46 does not (no duplicate key).
func TestNat46SharedInterface(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"map": {"interfaces": [{"interface": "loop460", "mode": "map-t"}],
	  "domains": [{"name": "mt", "mode": "map-t", "ipv4Prefix": "203.0.113.0/24", "ipv6Prefix": "2001:db8:99::/48", "ipv6Source": "2001:db8:ff::/96", "eaBitsLength": 8}]},
	  "nat46": {"interfaces": ["loop460"], "mappings": [{"name": "web", "ipv4": "198.51.100.10", "ipv6": "2001:db8:46::10"}]}}`), vrfID)
	want := "map.domain/mt,map.domain/nat46-web,map.interface/loop460/map-t"
	if got := strings.Join(s.keys(), ","); got != want || len(s.errs) != 0 {
		t.Fatalf("keys %s errs %v", got, s.errs)
	}
}

func TestNat46Refusals(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"map": {"interfaces": [{"interface": "loop460", "mode": "map-e"}],
	  "domains": [{"name": "e", "mode": "map-e", "ipv4Prefix": "198.51.100.0/24", "ipv6Prefix": "2001:db8:99::/48", "ipv6Source": "2001:db8:ff::1/128", "eaBitsLength": 8}]},
	  "nat46": {"clientPrefix": "64:ff9b::/96", "interfaces": ["loop460"], "mappings": [{"name": "web", "ipv4": "198.51.100.10", "ipv6": "2001:db8:46::10"}]}}`), vrfID)
	wantErr := "/nat/nat46/mappings/0/ipv4 " + desired.RuleNat46 + ",/nat/nat46/interfaces/0 " + desired.RuleNat46
	if got := strings.Join(s.errs, ","); got != wantErr {
		t.Fatalf("errs\n got %s\nwant %s", got, wantErr)
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{"nat46": {"clientPrefix": "64:ff9b::/64", "interfaces": ["loop460"],
	  "mappings": [{"name": "a", "ipv4": "198.51.100.10", "ipv6": "2001:db8:46::10"}, {"name": "b", "ipv4": "198.51.100.10", "ipv6": "2001:db8:46::11"}]}}`), vrfID)
	wantErr = "/nat/nat46/clientPrefix " + desired.RuleNat46 + ",/nat/nat46/mappings/1/ipv4 " + desired.RuleNat46
	if got := strings.Join(s.errs, ","); got != wantErr {
		t.Fatalf("errs\n got %s\nwant %s", got, wantErr)
	}
	for _, k := range s.keys() {
		if strings.HasPrefix(k, "map.") {
			t.Fatalf("refused NAT46 still emitted %s", k)
		}
	}
}

// TestNat46RoundTrip: with the stored owners recorded, Assemble(Project(doc)) == doc, also when an interface is
// shared with nat.map (it appears under both, no false drift) — and without owners a NAT46 interface stays nat.map
// drift (never silently claimed).
func TestNat46RoundTrip(t *testing.T) {
	in := `{"map": {"interfaces": [{"interface": "loop460", "mode": "map-t"}],
	  "domains": [{"name": "mt", "mode": "map-t", "ipv4Prefix": "203.0.113.0/24", "ipv6Prefix": "2001:db8:99::/48", "ipv6Source": "2001:db8:ff::/96", "eaBitsLength": 8, "psidOffset": 0, "psidLength": 0}]},
	  "nat46": {"clientPrefix": "64:ff9b::/96", "interfaces": ["loop460", "loop461"],
	    "mappings": [{"name": "dns", "ipv4": "198.51.100.53", "ipv6": "2001:db8:46::53"}, {"name": "web", "ipv4": "198.51.100.10", "ipv6": "2001:db8:46::10", "mtu": 1500}]}}`
	want := natDoc(t, in)
	desired.SetNat46Owners(want)
	defer desired.SetNat46Owners(nil)
	s := newSink()
	desired.Nat(s, want, vrfID)
	if len(s.errs) != 0 {
		t.Fatalf("errs %v", s.errs)
	}
	if got := desired.AssembleNat(s.kvs, tableName); !proto.Equal(got, want) {
		t.Fatalf("round trip\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}

	only := natDoc(t, nat46Doc)
	desired.SetNat46Owners(only)
	s = newSink()
	desired.Nat(s, only, vrfID)
	got := desired.AssembleNat(s.kvs, tableName)
	if got.GetMap() != nil || !proto.Equal(got.GetNat46(), only.GetNat46()) {
		t.Fatalf("nat46 only\n got %s\nwant %s", protojson.Format(got), protojson.Format(only))
	}
	desired.SetNat46Owners(nil)
	got = desired.AssembleNat(s.kvs, tableName)
	if len(got.GetMap().GetInterfaces()) != 2 || len(got.GetNat46().GetInterfaces()) != 0 || len(got.GetNat46().GetMappings()) != 2 {
		t.Fatalf("without owners: %s", protojson.Format(got))
	}
}
