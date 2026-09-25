package desired_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/nat44_ei"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/nat66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/npt66"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// eiRegistry registers F-nat44-ei-64-66-nptv6's four families on v for owner w4 (globals owner or not, D-071).
func eiRegistry(v *coretest.VPP, globalsOwner bool) *scheduler.MapRegistry {
	reg := scheduler.NewRegistry()
	o := natcommon.WithGlobalsOwner(globalsOwner)
	nat44ei.Register(reg, v, "w4", o)
	nat64.Register(reg, v, "w4", o)
	nat66.Register(reg, v, "w4", o)
	npt66.Register(reg, v, "w4", o)
	return reg
}

// applyOrdered creates the projected objects in registration order (each family registers its enable first), the
// order the scheduler's tie breaker uses among objects without a dependency between them.
func applyOrdered(t *testing.T, reg *scheduler.MapRegistry, kvs []scheduler.KV) {
	t.Helper()
	rank := map[string]int{}
	for i, n := range reg.Names() {
		rank[n] = i
	}
	ordered := append([]scheduler.KV{}, kvs...)
	sort.SliceStable(ordered, func(a, b int) bool { return rank[ordered[a].Key.Descriptor()] < rank[ordered[b].Key.Descriptor()] })
	for _, kv := range ordered {
		d, ok := reg.Get(kv.Key.Descriptor())
		if !ok {
			t.Fatalf("no descriptor for %s", kv.Key)
		}
		if _, err := d.Create(context.Background(), kv.Value); err != nil {
			t.Fatalf("create %s: %v", kv.Key, err)
		}
	}
}

// retrieveAll is what the scheduler's Retrieve collects: every descriptor, write-only ones skipped.
func retrieveAll(t *testing.T, reg *scheduler.MapRegistry) []scheduler.KV {
	t.Helper()
	var out []scheduler.KV
	for _, n := range reg.Names() {
		d, _ := reg.Get(n)
		kvs, err := d.Retrieve(context.Background())
		if scheduler.IsRetrieveUnsupported(err) {
			continue
		}
		if err != nil {
			t.Fatalf("retrieve %s: %v", n, err)
		}
		out = append(out, kvs...)
	}
	return out
}

// checkKeys asserts that the builder's key of every object is its descriptor's KeyOf.
func checkKeys(t *testing.T, reg *scheduler.MapRegistry, kvs []scheduler.KV) {
	t.Helper()
	for _, kv := range kvs {
		d, ok := reg.Get(kv.Key.Descriptor())
		if !ok {
			t.Fatalf("no descriptor for %s", kv.Key)
		}
		if got := d.KeyOf(kv.Value); got != kv.Key {
			t.Errorf("builder key %s, descriptor KeyOf %s", kv.Key, got)
		}
	}
}

// Every NAT44 leaf that exists in EI: outbound PAT, 1:1, port forward, identity mappings, interface pool.
const eiDoc = `{
  "mode": "ei",
  "inside": ["host-w4l0"],
  "outside": ["host-w4w0"],
  "outputFeature": ["loop401"],
  "insideVrf": "cust",
  "forwarding": true,
  "staticMappingOnly": false,
  "connectionTracking": true,
  "timeouts": {"udp": 120, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60},
  "pools": [
    {"name": "out", "description": "outbound PAT", "range": "10.4.2.100-10.4.2.103", "twiceNat": false},
    {"name": "tenant", "range": "10.4.3.1-10.4.3.2", "vrf": "cust"},
    {"name": "wan", "interface": "host-w4w0"}
  ],
  "staticMappings": [
    {"name": "web", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.103", "port": 8080}},
    {"name": "one2one", "local": {"ip": "10.4.1.3"}, "external": {"ip": "10.4.2.111"}, "description": "1:1"},
    {"name": "viapool", "protocol": "udp", "local": {"ip": "10.4.1.4", "port": 53}, "external": {"pool": "out", "port": 5353}},
    {"name": "viaif", "protocol": "tcp", "local": {"ip": "10.4.1.5", "port": 22}, "external": {"pool": "wan", "port": 2222}, "vrf": "cust"}
  ],
  "identityMappings": [
    {"ip": "10.4.2.112", "protocol": "udp", "port": 500},
    {"interface": "host-w4w0"}
  ]
}`

const eiCanonicalSlot = `{
  "mode": "ei",
  "inside": ["host-w4l0"],
  "outside": ["host-w4w0"],
  "outputFeature": ["loop401"],
  "pools": [
    {"range": "10.4.2.100-10.4.2.103", "twiceNat": false},
    {"range": "10.4.3.1-10.4.3.2", "vrf": "cust", "twiceNat": false},
    {"interface": "host-w4w0", "twiceNat": false}
  ],
  "staticMappings": [
    {"name": "one2one", "local": {"ip": "10.4.1.3"}, "external": {"ip": "10.4.2.111"}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false},
    {"name": "viaif", "protocol": "tcp", "local": {"ip": "10.4.1.5", "port": 22}, "external": {"interface": "host-w4w0", "port": 2222}, "vrf": "cust", "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false},
    {"name": "viapool", "protocol": "udp", "local": {"ip": "10.4.1.4", "port": 53}, "external": {"ip": "10.4.2.100", "port": 5353}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false},
    {"name": "web", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.103", "port": 8080}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false}
  ],
  "identityMappings": [
    {"ip": "10.4.2.112", "protocol": "udp", "port": 500},
    {"interface": "host-w4w0"}
  ]
}`

func TestNat44EIBuilderKeysAndPointers(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, eiDoc), vrfID)
	if len(s.errs)+len(s.warns) != 0 {
		t.Fatalf("errors %v warnings %v", s.errs, s.warns)
	}
	im1 := desired.IdentityMappingName(nat44ed.IdentityMappingSpec{IP: "10.4.2.112", Protocol: "udp", Port: 500})
	im2 := desired.IdentityMappingName(nat44ed.IdentityMappingSpec{Interface: "host-w4w0", Protocol: "any", AddrOnly: true})
	want := map[string]string{
		"nat44-ei.enable/global":                        "/nat",
		"nat44-ei.timeouts/global":                      "/nat/timeouts",
		"nat44-ei.forwarding/global":                    "/nat/forwarding",
		"nat44-ei.interface-feature/host-w4l0/inside":   "/nat/inside/0",
		"nat44-ei.interface-feature/host-w4w0/outside":  "/nat/outside/0",
		"nat44-ei.output-feature/loop401":               "/nat/outputFeature/0",
		"nat44-ei.address-pool/10.4.2.100-10.4.2.103/0": "/nat/pools/0",
		"nat44-ei.address-pool/10.4.3.1-10.4.3.2/4001":  "/nat/pools/1",
		"nat44-ei.interface-address/host-w4w0":          "/nat/pools/2",
		"nat44-ei.static-mapping/web":                   "/nat/staticMappings/0",
		"nat44-ei.static-mapping/one2one":               "/nat/staticMappings/1",
		"nat44-ei.static-mapping/viapool":               "/nat/staticMappings/2",
		"nat44-ei.static-mapping/viaif":                 "/nat/staticMappings/3",
		"nat44-ei.identity-mapping/" + im1:              "/nat/identityMappings/0",
		"nat44-ei.identity-mapping/" + im2:              "/nat/identityMappings/1",
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
	for _, kv := range s.kvs {
		switch kv.Key {
		case "nat44-ei.enable/global":
			e, _ := natcommon.Decode[nat44ei.EnableSpec](kv.Value)
			if e != (nat44ei.EnableSpec{InsideVRF: 4001, ConnectionTracking: true}) {
				t.Errorf("enable %+v", e)
			}
		case "nat44-ei.static-mapping/viaif":
			m, _ := natcommon.Decode[nat44ei.StaticMappingSpec](kv.Value)
			if m.External.Interface != "host-w4w0" || m.External.IP != "" || m.VRF != 4001 {
				t.Errorf("viaif %+v", m)
			}
		}
	}
}

func TestNat44EIBuilderEDOnlyAndWarnings(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"mode": "ei", "inside": ["host-w4l0"], "sessionLimit": 20000,
	  "pools": [{"name": "tn", "range": "10.4.2.120", "twiceNat": true}],
	  "staticMappings": [
	    {"name": "a", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.9", "port": 80}, "twiceNat": true},
	    {"name": "b", "local": {"ip": "10.4.1.3"}, "external": {"ip": "10.4.2.10"}, "selfTwiceNat": true, "out2inOnly": true}
	  ],
	  "loadBalancedMappings": [{"name": "lb", "protocol": "tcp", "external": {"ip": "10.4.2.113", "port": 443}, "locals": [{"ip": "10.4.1.10", "port": 8443}]}]
	}`), vrfID)
	want := []string{
		"/nat/loadBalancedMappings/0 nat.mode-ed-features",
		"/nat/pools/0/twiceNat nat.mode-ed-features",
		"/nat/staticMappings/0/twiceNat nat.mode-ed-features",
		"/nat/staticMappings/1/out2inOnly nat.mode-ed-features",
		"/nat/staticMappings/1/selfTwiceNat nat.mode-ed-features",
	}
	sort.Strings(s.errs)
	if strings.Join(s.errs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(s.errs, "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(s.warns, ",") != "/nat/sessionLimit agent.unsupported-field" {
		t.Fatalf("warnings %v", s.warns)
	}
	// a port forward outside every pool is refused (nat44-ei reserves the port on a pool address), not with an
	// interface pool (unknown address) or in static-mapping-only mode
	s = newSink()
	desired.Nat(s, natDoc(t, `{"mode": "ei", "inside": ["host-w4l0"], "pools": [{"name": "p", "range": "10.4.2.100-10.4.2.101"}],
	  "staticMappings": [{"name": "a", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.110", "port": 80}},
	    {"name": "b", "protocol": "tcp", "local": {"ip": "10.4.1.3", "port": 80}, "external": {"ip": "10.4.2.101", "port": 80}},
	    {"name": "c", "local": {"ip": "10.4.1.4"}, "external": {"ip": "10.4.2.120"}}]}`), vrfID)
	if strings.Join(s.errs, ",") != "/nat/staticMappings/0/external nat.ei-port-forward-pool" {
		t.Fatalf("port forward outside the pools: %v", s.errs)
	}
	for _, extra := range []string{`"staticMappingOnly": true,`, `"pools": [{"name": "w", "interface": "host-w4w0"}],`} {
		s = newSink()
		desired.Nat(s, natDoc(t, `{"mode": "ei", "inside": ["host-w4l0"], `+extra+`
		  "staticMappings": [{"name": "a", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.110", "port": 80}}],
		  "identityMappings": [{"ip": "10.4.2.111", "protocol": "tcp", "port": 22}]}`), vrfID)
		if len(s.errs) != 0 {
			t.Fatalf("%s: %v", extra, s.errs)
		}
	}
	// review L2: an identity mapping with a port reserves it on its own address, the same rule; without a port it
	// reserves nothing
	s = newSink()
	desired.Nat(s, natDoc(t, `{"mode": "ei", "inside": ["host-w4l0"], "pools": [{"name": "p", "range": "10.4.2.100-10.4.2.101"}],
	  "identityMappings": [{"ip": "10.4.2.111", "protocol": "tcp", "port": 22}, {"ip": "10.4.2.100", "protocol": "tcp", "port": 22},
	    {"ip": "10.4.2.112"}, {"interface": "host-w4w0", "protocol": "udp", "port": 53}]}`), vrfID)
	if strings.Join(s.errs, ",") != "/nat/identityMappings/0/ip nat.ei-port-forward-pool" {
		t.Fatalf("identity mapping with a port outside the pools: %v", s.errs)
	}
	// enabled:false keeps the EI configuration and projects nothing (D-062)
	s = newSink()
	desired.Nat(s, natDoc(t, `{"mode": "ei", "enabled": false, "inside": ["host-w4l0"]}`), vrfID)
	if len(s.kvs)+len(s.errs)+len(s.warns) != 0 {
		t.Fatalf("disabled EI projected %v %v %v", s.keys(), s.errs, s.warns)
	}
}

func TestNat44EIRoundTripNonOwner(t *testing.T) {
	v := fakeWithRig(t)
	n := v.Nat44EI()
	// the plugin is a test fixture on the shared host, enabled with what the document requires
	n.Lock()
	n.Enabled, n.Cfg.InsideVrf, n.Cfg.Flags = true, 4001, nat44_ei.NAT44_EI_CONNECTION_TRACKING
	n.Timeouts.UDP, n.Timeouts.TCPEstablished, n.Timeouts.TCPTransitory, n.Timeouts.ICMP = 120, 7440, 240, 60
	n.Forwarding = true
	n.Unlock()
	reg := eiRegistry(v, false)
	s := newSink()
	desired.Nat(s, natDoc(t, eiDoc), vrfID)
	applyOrdered(t, reg, s.kvs)
	got := desired.AssembleNat(retrieveAll(t, reg), tableName)
	want := natDoc(t, eiCanonicalSlot)
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	// the canonical document projects to the same objects (no drift on re-apply)
	canon := natDoc(t, eiCanonicalSlot)
	canon.InsideVrf, canon.Forwarding, canon.ConnectionTracking = proto.String("cust"), proto.Bool(true), proto.Bool(true)
	canon.Timeouts = &vrxv1.NatTimeouts{Udp: proto.Uint32(120)}
	s2 := newSink()
	desired.Nat(s2, canon, vrfID)
	if len(s2.errs)+len(s2.warns) != 0 || strings.Join(s2.keys(), ",") != strings.Join(s.keys(), ",") {
		t.Fatalf("canonical document projects differently:\n%v\n%v\n%v %v", s2.keys(), s.keys(), s2.errs, s2.warns)
	}
	// a slot never set the globals: enable/timeouts/forwarding were requirements only (D-071)
	n.Lock()
	addrs := len(n.Addrs)
	n.Unlock()
	if addrs != 6 {
		t.Fatalf("fake holds %d pool addresses", addrs)
	}
	// a mismatching global is refused, never changed
	n.Lock()
	n.Forwarding = false
	n.Unlock()
	d, _ := reg.Get(nat44ei.NameForwarding)
	if _, err := d.Create(context.Background(), natcommon.MustEncode(&nat44ei.ForwardingSpec{})); err == nil {
		t.Fatal("a slot accepted forwarding off")
	}
}

func TestNat44EIRoundTripGlobalsOwner(t *testing.T) {
	v := fakeWithRig(t)
	reg := eiRegistry(v, true)
	s := newSink()
	desired.Nat(s, natDoc(t, eiDoc), vrfID)
	applyOrdered(t, reg, s.kvs)
	got := desired.AssembleNat(retrieveAll(t, reg), tableName)
	want := natDoc(t, eiCanonicalSlot)
	want.Enabled, want.InsideVrf, want.Forwarding = proto.Bool(true), proto.String("cust"), proto.Bool(true)
	want.StaticMappingOnly, want.ConnectionTracking = proto.Bool(false), proto.Bool(true)
	want.Timeouts = &vrxv1.NatTimeouts{Udp: proto.Uint32(120), TcpEstablished: proto.Uint32(7440), TcpTransitory: proto.Uint32(240), Icmp: proto.Uint32(60)}
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if !v.Nat44EI().Enabled {
		t.Fatal("the globals owner did not enable nat44-ei")
	}
	// ED and EI are exclusive: with EI on, the globals owner's ED enable is refused
	edReg := scheduler.NewRegistry()
	nat44ed.Register(edReg, v, "w4", natcommon.WithGlobalsOwner(true))
	d, _ := edReg.Get(nat44ed.NameEnable)
	if _, err := d.Create(context.Background(), natcommon.MustEncode(&nat44ed.EnableSpec{})); err == nil {
		t.Fatal("nat44-ed enabled next to nat44-ei")
	}
}
