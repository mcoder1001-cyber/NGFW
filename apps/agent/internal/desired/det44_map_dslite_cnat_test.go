package desired_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/desired"
)

func TestDet44Projection(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"det44": {"enabled": true, "inside": ["host-w9l0"], "outside": ["host-w9w0"], "insideVrf": "cust",
	  "mappings": [{"inside": "10.9.0.0/24", "outside": "10.9.200.0/30"}], "timeouts": {"udp": 600}}}`), vrfID)
	want := "det44.enable/global,det44.interface/host-w9l0/inside,det44.interface/host-w9w0/outside,det44.map/10.9.0.0/24/10.9.200.0/30,det44.timeouts/global"
	if got := strings.Join(s.keys(), ","); got != want || len(s.errs) != 0 {
		t.Fatalf("keys %s errs %v", got, s.errs)
	}
	if desired.Det44PortsPerHost(24, 30) != 1008 {
		t.Fatalf("ports per host /24→/30 = %d", desired.Det44PortsPerHost(24, 30))
	}
	// disabled: nothing; errors carry pointers
	s = newSink()
	desired.Nat(s, natDoc(t, `{"det44": {"enabled": false, "mappings": [{"inside": "10.9.0.0/24", "outside": "10.9.200.0/30"}]}}`), vrfID)
	if len(s.kvs) != 0 {
		t.Fatalf("disabled projected %v", s.keys())
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{"det44": {"enabled": true, "mappings": [
	  {"inside": "10.9.0.1/24", "outside": "10.9.200.0/30"},
	  {"inside": "10.9.0.0/30", "outside": "10.9.200.0/24"},
	  {"inside": "10.0.0.0/8", "outside": "10.9.200.0/32"},
	  {"inside": "10.9.1.0/24", "outside": "10.9.201.0/30"},
	  {"inside": "10.9.1.0/25", "outside": "10.9.202.0/30"}]}}`), vrfID)
	want = "/nat/det44/mappings/0/inside nat.prefixes-are-networks,/nat/det44/mappings/1/outside nat.det44-valid,/nat/det44/mappings/2/outside nat.det44-valid,/nat/det44/mappings/4/inside nat.det44-valid"
	if got := strings.Join(s.errs, ","); got != want {
		t.Fatalf("errs %s", got)
	}
}

func TestDsliteProjection(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"dslite": {"enabled": true, "aftr": {"ipv6": "fd00:9::1", "ipv4": "192.0.0.1"}, "pools": [{"range": "10.9.5.1-10.9.5.4"}]}}`), vrfID)
	if got := strings.Join(s.keys(), ","); got != "dslite.aftr/global,dslite.pool/10.9.5.1-10.9.5.4" || len(s.errs) != 0 {
		t.Fatalf("keys %s errs %v", got, s.errs)
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{"dslite": {"enabled": true, "b4": {"ipv6": "10.0.0.1"}}}`), vrfID)
	if strings.Join(s.errs, ",") != "/nat/dslite/b4/ipv6 nat.dslite-valid" {
		t.Fatalf("errs %v", s.errs)
	}
}

func TestMapProjection(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"map": {
	  "interfaces": [{"interface": "host-w9w0", "mode": "map-t"}],
	  "domains": [
	    {"name": "mt", "mode": "map-t", "ipv4Prefix": "10.9.64.0/24", "ipv6Prefix": "fd00:9:64::/48", "ipv6Source": "fd00:9:ff::/64", "eaBitsLength": 16, "psidOffset": 6, "psidLength": 8},
	    {"name": "lw", "mode": "lw4o6", "ipv4Prefix": "10.9.65.0/24", "ipv6Prefix": "fd00:9:65::/64", "ipv6Source": "fd00:9::1/128", "psidLength": 4,
	     "rules": [{"psid": 1, "ipv6Destination": "fd00:9:65::1"}, {"psid": 2, "ipv6Destination": "fd00:9:65::2"}]}],
	  "parameters": {"tcpMss": 1400}}}`), vrfID)
	want := "map.domain/lw,map.domain/mt,map.interface/host-w9w0/map-t,map.rule/lw/1,map.rule/lw/2"
	if got := strings.Join(s.keys(), ","); got != want || len(s.errs) != 0 {
		t.Fatalf("keys %s errs %v", got, s.errs)
	}
	if strings.Join(s.warns, ",") != "/nat/map/parameters/tcpMss agent.unsupported-field" {
		t.Fatalf("warns %v", s.warns)
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{"map": {"domains": [
	  {"name": "a", "mode": "map-t", "ipv4Prefix": "10.9.64.0/24", "ipv6Prefix": "fd00:9:64::/48", "ipv6Source": "fd00:9::1/128"},
	  {"name": "b", "mode": "lw4o6", "ipv4Prefix": "10.9.64.0/24", "ipv6Prefix": "fd00:9:64::/48", "ipv6Source": "fd00:9::1/128", "eaBitsLength": 8},
	  {"name": "c", "mode": "map-e", "ipv4Prefix": "10.9.64.0/24", "ipv6Prefix": "fd00:9:64::/48", "ipv6Source": "fd00:9::1/128", "psidLength": 2, "rules": [{"psid": 4, "ipv6Destination": "fd00:9::4"}]}]}}`), vrfID)
	want = "/nat/map/domains/0/ipv6Source nat.map-valid,/nat/map/domains/1/eaBitsLength nat.map-valid,/nat/map/domains/2/rules/0/psid nat.map-valid"
	if got := strings.Join(s.errs, ","); got != want {
		t.Fatalf("errs %s", got)
	}
}

func TestCnatProjection(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"cnat": {
	  "translations": [{"name": "web", "protocol": "tcp", "vip": {"ip": "10.9.2.100", "port": 80}, "backends": [{"ip": "10.9.2.2", "port": 8080}, {"ip": "10.9.2.3", "port": 8080}]}],
	  "snat": {"policy": "interface", "addresses": {"ipv4": "10.9.2.1"}, "interfaces": [{"interface": "host-w9w0", "table": "include-v4"}], "excludePrefixes": ["10.9.0.0/16"]}}}`), vrfID)
	want := "cnat.interface-feature/host-w9w0,cnat.snat-addresses/global,cnat.snat-exclude-prefix/10.9.0.0/16,cnat.snat-interface/host-w9w0/include-v4,cnat.snat-policy/global,cnat.translation/10.9.2.100/tcp/80"
	if got := strings.Join(s.keys(), ","); got != want || len(s.errs) != 0 {
		t.Fatalf("keys %s errs %v", got, s.errs)
	}
	// V10 / acceptance: a policy without an SNAT address is refused with a pointer
	s = newSink()
	desired.Nat(s, natDoc(t, `{"cnat": {"snat": {"policy": "interface"}}}`), vrfID)
	if strings.Join(s.errs, ",") != "/nat/cnat/snat/addresses "+desired.RuleCnatSnatAddress {
		t.Fatalf("errs %v", s.errs)
	}
	// CNAT and NAT44-ED on one interface: refused
	s = newSink()
	desired.Nat(s, natDoc(t, `{"inside": ["host-w9w0"], "cnat": {"snat": {"policy": "interface", "addresses": {"ipv4": "10.9.2.1"}, "interfaces": [{"interface": "host-w9w0", "table": "include-v4"}]}}}`), vrfID)
	if strings.Join(s.errs, ",") != "/nat/cnat/snat/interfaces/0/interface "+desired.RuleCnatNat44 {
		t.Fatalf("errs %v", s.errs)
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{"cnat": {"translations": [{"name": "x", "protocol": "udp", "vip": {"ip": "10.9.2.100", "port": 53}, "backends": [{"ip": "fd00:9::1", "port": 53}]}]}}`), vrfID)
	if strings.Join(s.errs, ",") != "/nat/cnat/translations/0/backends/0/ip nat.cnat-valid" {
		t.Fatalf("errs %v", s.errs)
	}
}

// TestCgnatRoundTrip: project → AssembleNat gives back the canonical document for the retrievable leaves (the
// globals owner's view; write-only det44.enable / cnat policy leaves are covered by `enabled` / not retrieved).
func TestCgnatRoundTrip(t *testing.T) {
	in := `{"det44": {"enabled": true, "inside": ["a"], "outside": ["b"], "mappings": [{"inside": "10.9.0.0/24", "outside": "10.9.200.0/30"}], "timeouts": {"udp": 600, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60}},
	  "dslite": {"enabled": true, "aftr": {"ipv6": "fd00:9::1"}, "pools": [{"range": "10.9.5.1-10.9.5.4"}]},
	  "map": {"interfaces": [{"interface": "b", "mode": "map-e"}], "domains": [{"name": "lw", "mode": "lw4o6", "ipv4Prefix": "10.9.65.0/24", "ipv6Prefix": "fd00:9:65::/64", "ipv6Source": "fd00:9::1/128", "eaBitsLength": 0, "psidOffset": 0, "psidLength": 4, "rules": [{"psid": 1, "ipv6Destination": "fd00:9:65::1"}]}]},
	  "cnat": {"translations": [{"protocol": "tcp", "vip": {"ip": "10.9.2.100", "port": 80}, "backends": [{"ip": "10.9.2.2", "port": 8080}], "lbType": "default"}], "snat": {"addresses": {"ipv4": "10.9.2.1"}}}}`
	s := newSink()
	desired.Nat(s, natDoc(t, in), vrfID)
	if len(s.errs) != 0 {
		t.Fatalf("errs %v", s.errs)
	}
	got := desired.AssembleNat(s.kvs, tableName)
	want := natDoc(t, in)
	if !proto.Equal(got, want) {
		gj, _ := protojson.Marshal(got)
		wj, _ := protojson.Marshal(want)
		t.Fatalf("round trip\n got %s\nwant %s", gj, wj)
	}
}
