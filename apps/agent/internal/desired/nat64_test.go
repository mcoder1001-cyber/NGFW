package desired_test

import (
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// NAT64 (with the slot's /96 in a slot VRF, the DF-3 host-test pattern), NAT66 and NPTv6 in one document. The product
// default 64:ff9b::/96 appears in TestNat64ProductDefaultPrefix.
const v6Doc = `{
  "nat64": {
    "enabled": true,
    "inside": ["host-w4l0"],
    "outside": ["host-w4w0"],
    "prefixes": [{"prefix": "fd00:4:64::/96", "vrf": "cust"}, {"prefix": "fd00:4:6400::/96"}],
    "pools": [{"range": "10.4.64.1-10.4.64.2"}],
    "staticBibs": [{"description": "web", "protocol": "tcp", "inside": {"ip": "fd00:4::80", "port": 80}, "outside": {"ip": "10.4.64.1", "port": 8080}}],
    "timeouts": {"udp": 300, "tcpEstablished": 3600, "tcpTransitory": 240, "icmp": 60}
  },
  "nat66": {
    "enabled": true,
    "inside": ["loop401"],
    "outside": ["loop402"],
    "staticMappings": [{"description": "srv", "local": "fd00:4:1::66", "external": "fd00:4:2::66"}]
  },
  "nptv6": {
    "bindings": [{"description": "site", "interface": "host-w4w0", "internal": "fd00:4:10::/48", "external": "fd00:4:20::/48"}]
  }
}`

func v6Rig(t *testing.T) *coretest.VPP {
	t.Helper()
	v := fakeWithRig(t)
	v.AddInterface("loop402", "Loopback", "w4:loop402")
	return v
}

func TestNat64Nat66Nptv6BuilderKeysAndPointers(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, v6Doc), vrfID)
	if len(s.errs)+len(s.warns) != 0 {
		t.Fatalf("errors %v warnings %v", s.errs, s.warns)
	}
	want := map[string]string{
		"nat64.enable/global":                    "/nat/nat64",
		"nat64.timeouts/global":                  "/nat/nat64/timeouts",
		"nat64.interface/host-w4l0/inside":       "/nat/nat64/inside/0",
		"nat64.interface/host-w4w0/outside":      "/nat/nat64/outside/0",
		"nat64.prefix/fd00:4:64::/96/4001":       "/nat/nat64/prefixes/0",
		"nat64.prefix/fd00:4:6400::/96/0":        "/nat/nat64/prefixes/1",
		"nat64.pool/10.4.64.1-10.4.64.2/0":       "/nat/nat64/pools/0",
		"nat64.static-bib/tcp/fd00:4::80/80/0":   "/nat/nat64/staticBibs/0",
		"nat66.enable/global":                    "/nat/nat66",
		"nat66.interface/loop401":                "/nat/nat66/inside/0",
		"nat66.interface/loop402":                "/nat/nat66/outside/0",
		"nat66.static-mapping/fd00:4:1::66/0":    "/nat/nat66/staticMappings/0",
		"npt66.binding/host-w4w0/fd00:4:10::/48": "/nat/nptv6/bindings/0",
	}
	if len(s.kvs) != len(want) {
		t.Fatalf("keys %v", s.keys())
	}
	for k, ptr := range want {
		if got := s.pointers[scheduler.Key(k)]; got != ptr {
			t.Errorf("%s at %q, want %q (keys %v)", k, got, ptr, s.keys())
		}
	}
	checkKeys(t, eiRegistry(coretest.New(), false), s.kvs)
	// a disabled translator keeps its configuration and programs nothing (the schema default is enabled: false)
	s = newSink()
	desired.Nat(s, natDoc(t, `{"nat64": {"enabled": false, "inside": ["host-w4l0"], "prefixes": [{"prefix": "64:ff9b::/96"}]},
	  "nat66": {"inside": ["loop401"]}}`), vrfID)
	if len(s.kvs)+len(s.errs)+len(s.warns) != 0 {
		t.Fatalf("disabled translators projected %v %v %v", s.keys(), s.errs, s.warns)
	}
}

func TestNat64ProductDefaultPrefix(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"nat64": {"enabled": true, "prefixes": [{"prefix": "64:ff9b::/96"}], "pools": [{"range": "198.51.100.1-198.51.100.14"}]}}`), vrfID)
	if strings.Join(s.keys(), ",") != "nat64.enable/global,nat64.pool/198.51.100.1-198.51.100.14/0,nat64.prefix/64:ff9b::/96/0" {
		t.Fatalf("keys %v (errors %v)", s.keys(), s.errs)
	}
}

func TestNat64Nat66Nptv6BuilderErrors(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{
	  "nat64": {"enabled": true,
	    "prefixes": [{"prefix": "fd00:4:64::/80"}, {"prefix": "10.4.0.0/16"}, {"prefix": "fd00:4:64::1/96"}, {"prefix": "fd00:4:65::/96", "vrf": "nope"}],
	    "pools": [{"range": "10.4.64.9-10.4.64.1"}],
	    "staticBibs": [{"protocol": "tcp", "inside": {"ip": "10.4.1.1", "port": 1}, "outside": {"ip": "10.4.64.1", "port": 1}},
	                   {"inside": {"ip": "fd00:4::1", "port": 1}, "outside": {"ip": "10.4.64.1", "port": 1}}]},
	  "nat66": {"enabled": true, "staticMappings": [{"local": "10.4.1.1", "external": "fd00:4::1"}]},
	  "nptv6": {"bindings": [
	    {"interface": "host-w4w0", "internal": "fd00:4:10::/48", "external": "fd00:4:20::/56"},
	    {"interface": "host-w4w0", "internal": "fd00:4:10::/80", "external": "fd00:4:20::/80"},
	    {"interface": "host-w4w0", "internal": "fd00:4:10::1/48", "external": "fd00:4:20::/48"},
	    {"interface": "host-w4l0", "internal": "fd00:4:11::/48", "external": "fd00:4:21::/48"},
	    {"interface": "host-w4l0", "internal": "fd00:4:12::/48", "external": "fd00:4:22::/48"}]}
	}`), vrfID)
	want := []string{
		"/nat/nat64/pools/0/range nat.pools-valid",
		"/nat/nat64/prefixes/0/prefix nat.nat64-valid",
		"/nat/nat64/prefixes/1/prefix nat.nat64-valid",
		"/nat/nat64/prefixes/2/prefix nat.prefixes-are-networks",
		"/nat/nat64/prefixes/3/vrf nat.vrfs-exist",
		"/nat/nat64/staticBibs/0/inside/ip nat.nat64-valid",
		"/nat/nat64/staticBibs/1/protocol nat.nat64-valid",
		"/nat/nat66/staticMappings/0/local nat.nat66-valid",
		"/nat/nptv6/bindings/0/external nat.nptv6-valid",
		"/nat/nptv6/bindings/1/internal nat.nptv6-valid",
		"/nat/nptv6/bindings/2/internal nat.prefixes-are-networks",
		"/nat/nptv6/bindings/4/interface nat.nptv6-valid",
	}
	sort.Strings(s.errs)
	sort.Strings(want)
	if strings.Join(s.errs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(s.errs, "\n"), strings.Join(want, "\n"))
	}
}

const v6CanonicalSlot = `{
  "nat64": {
    "enabled": true,
    "inside": ["host-w4l0"],
    "outside": ["host-w4w0"],
    "prefixes": [{"prefix": "fd00:4:6400::/96"}, {"prefix": "fd00:4:64::/96", "vrf": "cust"}],
    "pools": [{"range": "10.4.64.1-10.4.64.2"}],
    "staticBibs": [{"protocol": "tcp", "inside": {"ip": "fd00:4::80", "port": 80}, "outside": {"ip": "10.4.64.1", "port": 8080}}]
  },
  "nat66": {
    "enabled": true,
    "inside": ["loop401"],
    "outside": ["loop402"],
    "staticMappings": [{"local": "fd00:4:1::66", "external": "fd00:4:2::66"}]
  }
}`

// A slot (non-owner): the enables are write-only requirements, the nat64 timeouts a requirement; NPTv6 is write-only
// and never retrieved (D-063).
func TestNat64Nat66RoundTripNonOwner(t *testing.T) {
	v := v6Rig(t)
	v.Nat64().NatEnable()
	v.Nat66().NatEnable()
	n64 := v.Nat64()
	n64.Lock()
	n64.Timeouts.TCPEstablished = 3600
	n64.Unlock()
	reg := eiRegistry(v, false)
	s := newSink()
	desired.Nat(s, natDoc(t, v6Doc), vrfID)
	applyOrdered(t, reg, s.kvs)
	got := desired.AssembleNat(retrieveAll(t, reg), tableName)
	want := natDoc(t, v6CanonicalSlot)
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if b, ok := v.NPT66().Binding("host-w4w0"); !ok || b.Internal.String() != "fd00:4:10::/48" || b.External.String() != "fd00:4:20::/48" {
		t.Fatalf("npt66 binding %v %+v", ok, b)
	}
	// the canonical document (+ the write-only NPTv6 binding, the requirement timeouts) projects to the same objects
	canon := natDoc(t, v6CanonicalSlot)
	canon.Nat64.Timeouts = &vrxv1.NatTimeouts{TcpEstablished: proto.Uint32(3600)}
	canon.Nptv6 = natDoc(t, v6Doc).GetNptv6()
	s2 := newSink()
	desired.Nat(s2, canon, vrfID)
	if len(s2.errs)+len(s2.warns) != 0 || strings.Join(s2.keys(), ",") != strings.Join(s.keys(), ",") {
		t.Fatalf("canonical document projects differently:\n%v\n%v", s2.keys(), s.keys())
	}
}

func TestNat64Nat66RoundTripGlobalsOwner(t *testing.T) {
	v := v6Rig(t)
	reg := eiRegistry(v, true)
	s := newSink()
	desired.Nat(s, natDoc(t, v6Doc), vrfID)
	applyOrdered(t, reg, s.kvs)
	got := desired.AssembleNat(retrieveAll(t, reg), tableName)
	want := natDoc(t, v6CanonicalSlot)
	want.Nat64.Timeouts = &vrxv1.NatTimeouts{Udp: proto.Uint32(300), TcpEstablished: proto.Uint32(3600), TcpTransitory: proto.Uint32(240), Icmp: proto.Uint32(60)}
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if !v.Nat64().Enabled || !v.Nat66().Enabled {
		t.Fatal("the globals owner did not enable nat64 / nat66")
	}
}

func TestAssembleV6EmptyAndPureRoundTrip(t *testing.T) {
	if got := desired.AssembleNat(nil, tableName); proto.Size(got) != 0 {
		t.Fatalf("no objects: %v", got)
	}
	// project → assemble → project is stable for every leaf this task projects (incl. the write-only objects)
	s := newSink()
	desired.Nat(s, natDoc(t, v6Doc), vrfID)
	again := newSink()
	desired.Nat(again, desired.AssembleNat(s.kvs, tableName), vrfID)
	if strings.Join(again.keys(), ",") != strings.Join(s.keys(), ",") {
		t.Fatalf("pure round trip:\n%v\n%v", again.keys(), s.keys())
	}
}
