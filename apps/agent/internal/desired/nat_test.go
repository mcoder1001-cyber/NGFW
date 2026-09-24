package desired_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// sink records what a builder emits (the agent's projection in miniature).
type sink struct {
	kvs      []scheduler.KV
	pointers map[scheduler.Key]string
	errs     []string // "<pointer> <rule>"
	warns    []string
}

func newSink() *sink { return &sink{pointers: map[scheduler.Key]string{}} }

func (s *sink) Add(k scheduler.Key, v proto.Message, pointer string) {
	if _, dup := s.pointers[k]; dup {
		s.errs = append(s.errs, pointer+" agent.duplicate-object")
		return
	}
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
	s.pointers[k] = pointer
}
func (s *sink) Errorf(pointer, rule, _ string, _ ...any) { s.errs = append(s.errs, pointer+" "+rule) }
func (s *sink) Warnf(pointer, rule, _ string, _ ...any)  { s.warns = append(s.warns, pointer+" "+rule) }

func (s *sink) keys() []string {
	var out []string
	for _, kv := range s.kvs {
		out = append(out, string(kv.Key))
	}
	sort.Strings(out)
	return out
}

func natDoc(t *testing.T, js string) *vrxv1.NatConfig {
	t.Helper()
	n := &vrxv1.NatConfig{}
	if err := protojson.Unmarshal([]byte(js), n); err != nil {
		t.Fatalf("nat doc: %v", err)
	}
	return n
}

var vrfs = map[string]uint32{"default": 0, "": 0, "cust": 4001}

func vrfID(name string) (uint32, bool) { id, ok := vrfs[name]; return id, ok }

func tableName(id uint32) string {
	for n, v := range vrfs {
		if v == id && n != "" {
			if id == 0 {
				return "default"
			}
			return n
		}
	}
	return fmt.Sprint(id)
}

// The three classic scenarios (outbound PAT, 1:1, port forward) plus every other NAT44-ED leaf the builder projects.
const fullDoc = `{
  "mode": "ed",
  "inside": ["host-w4l0"],
  "outside": ["host-w4w0"],
  "outputFeature": ["loop401"],
  "insideVrf": "cust",
  "sessionLimit": 20000,
  "forwarding": true,
  "timeouts": {"udp": 120, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60},
  "pools": [
    {"name": "out", "description": "outbound PAT", "range": "10.4.2.100-10.4.2.103"},
    {"name": "tn", "range": "10.4.2.120", "twiceNat": true},
    {"name": "tenant", "range": "10.4.3.1-10.4.3.2", "vrf": "cust"},
    {"name": "wan", "interface": "host-w4w0"}
  ],
  "staticMappings": [
    {"name": "web", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.110", "port": 8080}},
    {"name": "one2one", "local": {"ip": "10.4.1.3"}, "external": {"ip": "10.4.2.111"}, "description": "1:1"},
    {"name": "viapool", "protocol": "udp", "local": {"ip": "10.4.1.4", "port": 53}, "external": {"pool": "out", "port": 5353}},
    {"name": "viaif", "protocol": "tcp", "local": {"ip": "10.4.1.5", "port": 22}, "external": {"pool": "wan", "port": 2222}, "vrf": "cust", "twiceNat": true}
  ],
  "identityMappings": [
    {"ip": "10.4.2.112", "protocol": "udp", "port": 500},
    {"interface": "host-w4w0"}
  ],
  "loadBalancedMappings": [
    {"name": "lb", "protocol": "tcp", "external": {"ip": "10.4.2.113", "port": 443},
     "locals": [{"ip": "10.4.1.11", "port": 8443, "probability": 60}, {"ip": "10.4.1.10", "port": 8443, "probability": 40}], "affinity": 30}
  ]
}`

func TestNatBuilderKeysAndPointers(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, fullDoc), vrfID)
	if len(s.errs)+len(s.warns) != 0 {
		t.Fatalf("errors %v warnings %v", s.errs, s.warns)
	}
	im1 := desired.IdentityMappingName(nat44ed.IdentityMappingSpec{IP: "10.4.2.112", Protocol: "udp", Port: 500})
	im2 := desired.IdentityMappingName(nat44ed.IdentityMappingSpec{Interface: "host-w4w0", Protocol: "any", AddrOnly: true})
	want := map[string]string{
		"nat44-ed.enable/global":                                  "/nat",
		"nat44-ed.timeouts/global":                                "/nat/timeouts",
		"nat44-ed.forwarding/global":                              "/nat/forwarding",
		"nat44-ed.interface-feature/host-w4l0/inside":             "/nat/inside/0",
		"nat44-ed.interface-feature/host-w4w0/outside":            "/nat/outside/0",
		"nat44-ed.output-feature/loop401":                         "/nat/outputFeature/0",
		"nat44-ed.address-pool/10.4.2.100-10.4.2.103/0":           "/nat/pools/0",
		"nat44-ed.address-pool/10.4.2.120-10.4.2.120/0/twice-nat": "/nat/pools/1",
		"nat44-ed.address-pool/10.4.3.1-10.4.3.2/4001":            "/nat/pools/2",
		"nat44-ed.interface-address/host-w4w0":                    "/nat/pools/3",
		"nat44-ed.static-mapping/web":                             "/nat/staticMappings/0",
		"nat44-ed.static-mapping/one2one":                         "/nat/staticMappings/1",
		"nat44-ed.static-mapping/viapool":                         "/nat/staticMappings/2",
		"nat44-ed.static-mapping/viaif":                           "/nat/staticMappings/3",
		"nat44-ed.identity-mapping/" + im1:                        "/nat/identityMappings/0",
		"nat44-ed.identity-mapping/" + im2:                        "/nat/identityMappings/1",
		"nat44-ed.lb-static-mapping/lb":                           "/nat/loadBalancedMappings/0",
	}
	if len(s.kvs) != len(want) {
		t.Fatalf("keys %v", s.keys())
	}
	for k, ptr := range want {
		if got := s.pointers[scheduler.Key(k)]; got != ptr {
			t.Errorf("%s at %q, want %q (keys %v)", k, got, ptr, s.keys())
		}
	}
	// the builder's key is the descriptor's own key for the value (KeyOf), for every object
	reg := scheduler.NewRegistry()
	nat44ed.Register(reg, coretest.New(), "w4")
	for _, kv := range s.kvs {
		d, ok := reg.Get(kv.Key.Descriptor())
		if !ok {
			t.Fatalf("no descriptor for %s", kv.Key)
		}
		if got := d.KeyOf(kv.Value); got != kv.Key {
			t.Errorf("builder key %s, descriptor KeyOf %s", kv.Key, got)
		}
	}
	// external.pool resolves to the range pool's start address or the interface pool's interface
	for _, kv := range s.kvs {
		switch kv.Key {
		case "nat44-ed.static-mapping/viapool":
			m, _ := natcommon.Decode[nat44ed.StaticMappingSpec](kv.Value)
			if m.External.IP != "10.4.2.100" || m.External.Interface != "" || m.External.Port != 5353 {
				t.Errorf("viapool external %+v", m.External)
			}
		case "nat44-ed.static-mapping/viaif":
			m, _ := natcommon.Decode[nat44ed.StaticMappingSpec](kv.Value)
			if m.External.Interface != "host-w4w0" || m.External.IP != "" || m.VRF != 4001 || !m.TwiceNAT {
				t.Errorf("viaif %+v", m)
			}
		case "nat44-ed.enable/global":
			e, _ := natcommon.Decode[nat44ed.EnableSpec](kv.Value)
			if e != (nat44ed.EnableSpec{Sessions: 20000, InsideVRF: 4001}) {
				t.Errorf("enable %+v", e)
			}
		}
	}
}

func TestNatBuilderOffDefaultsAndSiblings(t *testing.T) {
	// explicit enabled:false keeps the configuration but projects nothing of NAT44 (D-062)
	s := newSink()
	desired.Nat(s, natDoc(t, `{"enabled": false, "inside": ["host-w4l0"], "pools": [{"name": "p", "range": "10.4.2.100"}]}`), vrfID)
	if len(s.kvs)+len(s.errs)+len(s.warns) != 0 {
		t.Fatalf("enabled:false projected %v %v %v", s.keys(), s.errs, s.warns)
	}
	// absent enabled + nothing configured = off; default timeouts and no forwarding are no objects
	s = newSink()
	desired.Nat(s, natDoc(t, `{"mode": "ed", "forwarding": false, "timeouts": {"udp": 300, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60}}`), vrfID)
	if len(s.kvs) != 0 {
		t.Fatalf("an unconfigured nat projected %v", s.keys())
	}
	// enabled with nothing else: only the plugin requirement
	s = newSink()
	desired.Nat(s, natDoc(t, `{"enabled": true}`), vrfID)
	if strings.Join(s.keys(), ",") != "nat44-ed.enable/global" {
		t.Fatalf("enabled only: %v", s.keys())
	}
	// the CGNAT siblings and ipfix are warnings, never objects; mode ei and disabled nat64/nat66 are
	// F-nat44-ei-64-66-nptv6's (nat44-ei objects; nothing for a disabled translator); UNSUPPORTED VPP flags too
	s = newSink()
	desired.Nat(s, natDoc(t, `{"mode": "ei", "inside": ["host-w4l0"],
	  "ipfix": {"enabled": false}, "nat64": {"enabled": false}, "nat66": {"inside": ["x"]}, "nptv6": {"bindings": []},
	  "det44": {"enabled": true}, "dslite": {"enabled": false}, "map": {"domains": []}, "cnat": {"translations": []}}`), vrfID)
	if strings.Join(s.keys(), ",") != "nat44-ei.enable/global,nat44-ei.interface-feature/host-w4l0/inside" {
		t.Fatalf("mode ei projected %v", s.keys())
	}
	want := []string{
		"/nat/ipfix agent.unsupported-field", "/nat/det44 agent.unsupported-field", "/nat/dslite agent.unsupported-field",
	}
	sort.Strings(want)
	sort.Strings(s.warns)
	if strings.Join(s.warns, ",") != strings.Join(want, ",") { // empty messages (nptv6 {}, map {}, cnat {}) warn nothing
		t.Fatalf("warnings %v, want %v", s.warns, want)
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{"inside": ["host-w4l0"], "staticMappingOnly": true, "connectionTracking": true,
	  "staticMappings": [{"name": "a", "protocol": "tcp", "local": {"ip": "10.4.1.2"}, "external": {"ip": "10.4.2.9"}}]}`), vrfID)
	sort.Strings(s.warns)
	if strings.Join(s.warns, ",") != "/nat/connectionTracking agent.unsupported-field,/nat/staticMappingOnly agent.unsupported-field,/nat/staticMappings/0/protocol agent.unsupported-field" {
		t.Fatalf("warnings %v", s.warns)
	}
}

func TestNatBuilderErrors(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{
	  "mode": "xx"}`), vrfID)
	if strings.Join(s.errs, ",") != "/nat/mode nat.mode" {
		t.Fatalf("mode: %v", s.errs)
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{
	  "insideVrf": "nope",
	  "pools": [
	    {"name": "big", "range": "10.4.0.0-10.4.4.0"},
	    {"name": "rev", "range": "10.4.2.9-10.4.2.1"},
	    {"name": "badvrf", "range": "10.4.2.20", "vrf": "nope"},
	    {"name": "both", "range": "10.4.2.30", "interface": "host-w4w0"},
	    {"name": "dup1", "range": "10.4.2.40"},
	    {"name": "dup2", "range": "10.4.2.40"}
	  ],
	  "staticMappings": [
	    {"name": "noport", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.9"}},
	    {"name": "noproto", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.9", "port": 80}},
	    {"name": "nopool", "local": {"ip": "10.4.1.2"}, "external": {"pool": "missing"}},
	    {"name": "twoext", "local": {"ip": "10.4.1.2"}, "external": {"ip": "10.4.2.9", "interface": "host-w4w0"}}
	  ],
	  "identityMappings": [{"port": 22, "ip": "10.4.2.9"}, {}]
	}`), vrfID)
	want := []string{
		"/nat/insideVrf nat.vrfs-exist",
		"/nat/pools/0/range nat.pool-size",
		"/nat/pools/1/range nat.pools-valid",
		"/nat/pools/2/vrf nat.vrfs-exist",
		"/nat/pools/3 nat.pools-valid",
		"/nat/pools/5 agent.duplicate-object",
		"/nat/staticMappings/0 nat.static-mappings",
		"/nat/staticMappings/1/protocol nat.static-mappings",
		"/nat/staticMappings/2/external/pool nat.static-mappings",
		"/nat/staticMappings/3/external nat.static-mappings",
		"/nat/identityMappings/0/protocol nat.identity-mappings",
		"/nat/identityMappings/1 nat.identity-mappings",
	}
	sort.Strings(s.errs)
	sort.Strings(want)
	if strings.Join(s.errs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(s.errs, "\n"), strings.Join(want, "\n"))
	}
}

// apply creates the projected objects on the fake in dependency order (enable first) through the DF-3 descriptors.
func apply(t *testing.T, reg *scheduler.MapRegistry, kvs []scheduler.KV) {
	t.Helper()
	ordered := append([]scheduler.KV{}, kvs...)
	sort.SliceStable(ordered, func(a, b int) bool {
		return ordered[a].Key.Descriptor() == nat44ed.NameEnable && ordered[b].Key.Descriptor() != nat44ed.NameEnable
	})
	for _, kv := range ordered {
		d, _ := reg.Get(kv.Key.Descriptor())
		if _, err := d.Create(context.Background(), kv.Value); err != nil {
			t.Fatalf("create %s: %v", kv.Key, err)
		}
	}
}

func retrieve(t *testing.T, reg *scheduler.MapRegistry) []scheduler.KV {
	t.Helper()
	var out []scheduler.KV
	for _, n := range []string{nat44ed.NameEnable, nat44ed.NameTimeouts, nat44ed.NameForwarding, nat44ed.NameInterfaceFeature, nat44ed.NameOutputFeature,
		nat44ed.NameAddressPool, nat44ed.NameInterfaceAddress, nat44ed.NameStaticMapping, nat44ed.NameIdentityMapping, nat44ed.NameLBStaticMapping} {
		d, _ := reg.Get(n)
		kvs, err := d.Retrieve(context.Background())
		if scheduler.IsRetrieveUnsupported(err) {
			continue // a non-owner cannot retrieve VPP globals (D-071)
		}
		if err != nil {
			t.Fatalf("retrieve %s: %v", n, err)
		}
		out = append(out, kvs...)
	}
	return out
}

func fakeWithRig(t *testing.T) *coretest.VPP {
	t.Helper()
	v := coretest.New()
	v.AddInterface("host-w4l0", "af_packet", "w4:host-w4l0")
	v.AddInterface("host-w4w0", "af_packet", "w4:host-w4w0")
	v.AddInterface("loop401", "Loopback", "w4:loop401")
	v.SetIPv4("host-w4w0", "10.4.2.1/24")
	return v
}

// canonicalFull is what Retrieve reports for fullDoc: lists sorted, pool names / descriptions never invented,
// `external.pool` as the address / interface it stands for, 1:1 without protocol, explicit flags, the globals only for
// the globals owner.
const canonicalFullSlot = `{
  "mode": "ed",
  "forwarding": true,
  "inside": ["host-w4l0"],
  "outside": ["host-w4w0"],
  "outputFeature": ["loop401"],
  "pools": [
    {"range": "10.4.2.100-10.4.2.103", "twiceNat": false},
    {"range": "10.4.3.1-10.4.3.2", "vrf": "cust", "twiceNat": false},
    {"range": "10.4.2.120", "twiceNat": true},
    {"interface": "host-w4w0", "twiceNat": false}
  ],
  "staticMappings": [
    {"name": "one2one", "local": {"ip": "10.4.1.3"}, "external": {"ip": "10.4.2.111"}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false},
    {"name": "viaif", "protocol": "tcp", "local": {"ip": "10.4.1.5", "port": 22}, "external": {"interface": "host-w4w0", "port": 2222}, "vrf": "cust", "twiceNat": true, "selfTwiceNat": false, "out2inOnly": false},
    {"name": "viapool", "protocol": "udp", "local": {"ip": "10.4.1.4", "port": 53}, "external": {"ip": "10.4.2.100", "port": 5353}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false},
    {"name": "web", "protocol": "tcp", "local": {"ip": "10.4.1.2", "port": 80}, "external": {"ip": "10.4.2.110", "port": 8080}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false}
  ],
  "identityMappings": [
    {"ip": "10.4.2.112", "protocol": "udp", "port": 500},
    {"interface": "host-w4w0"}
  ],
  "loadBalancedMappings": [
    {"name": "lb", "protocol": "tcp", "external": {"ip": "10.4.2.113", "port": 443}, "affinity": 30, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false,
     "locals": [{"ip": "10.4.1.10", "port": 8443, "probability": 40}, {"ip": "10.4.1.11", "port": 8443, "probability": 60}]}
  ]
}`

func TestNatRoundTripNonOwner(t *testing.T) {
	v := fakeWithRig(t)
	n := v.Nat44ED()
	// the plugin is a test fixture on the shared host: enabled with the configuration the document requires
	n.Lock()
	n.Enabled, n.Cfg.Sessions, n.Cfg.InsideVrf = true, 20000, 4001
	n.Timeouts.UDP, n.Timeouts.TCPEstablished, n.Timeouts.TCPTransitory, n.Timeouts.ICMP = 120, 7440, 240, 60
	n.Forwarding = true
	n.Unlock()
	reg := scheduler.NewRegistry()
	nat44ed.Register(reg, v, "w4") // not the globals owner (D-071): enable/timeouts/forwarding are requirements
	s := newSink()
	desired.Nat(s, natDoc(t, fullDoc), vrfID)
	apply(t, reg, s.kvs)
	got := desired.AssembleNat(retrieve(t, reg), tableName)
	// forwarding is visible to a non-owner only as "set" (its presence object is not retrievable): not reported
	want := natDoc(t, strings.Replace(canonicalFullSlot, `"forwarding": true,`, ``, 1))
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	// the canonical document projects to the same objects (running-vs-actual has no drift on re-apply)
	s2 := newSink()
	canon := natDoc(t, canonicalFullSlot)
	canon.Enabled, canon.InsideVrf, canon.SessionLimit = proto.Bool(true), proto.String("cust"), proto.Uint32(20000)
	canon.Timeouts = &vrxv1.NatTimeouts{Udp: proto.Uint32(120)}
	desired.Nat(s2, canon, vrfID)
	if len(s2.errs)+len(s2.warns) != 0 || strings.Join(s2.keys(), ",") != strings.Join(s.keys(), ",") {
		t.Fatalf("canonical document projects differently:\n%v\n%v\n%v %v", s2.keys(), s.keys(), s2.errs, s2.warns)
	}
	for i := range s.kvs {
		for j := range s2.kvs {
			if s.kvs[i].Key == s2.kvs[j].Key && !proto.Equal(s.kvs[i].Value, s2.kvs[j].Value) {
				t.Errorf("%s: %v vs %v", s.kvs[i].Key, s.kvs[i].Value, s2.kvs[j].Value)
			}
		}
	}
	// VPP holds the pool addresses one by one, the interface-bound mapping twice: Retrieve merges and collapses them
	n.Lock()
	addrs, statics := len(n.Addrs), len(n.Statics)
	n.Unlock()
	if addrs != 7 || statics != 4 {
		t.Fatalf("fake holds %d pool addresses and %d static mappings", addrs, statics)
	}
}

func TestNatRoundTripGlobalsOwner(t *testing.T) {
	v := fakeWithRig(t)
	reg := scheduler.NewRegistry()
	nat44ed.Register(reg, v, "w4", natcommon.WithGlobalsOwner(true)) // the product agent on a real box
	s := newSink()
	desired.Nat(s, natDoc(t, fullDoc), vrfID)
	apply(t, reg, s.kvs)
	got := desired.AssembleNat(retrieve(t, reg), tableName)
	want := natDoc(t, canonicalFullSlot)
	want.Enabled, want.InsideVrf, want.SessionLimit = proto.Bool(true), proto.String("cust"), proto.Uint32(20000)
	want.Timeouts = &vrxv1.NatTimeouts{Udp: proto.Uint32(120), TcpEstablished: proto.Uint32(7440), TcpTransitory: proto.Uint32(240), Icmp: proto.Uint32(60)}
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if !v.Nat44ED().Enabled {
		t.Fatal("the globals owner did not enable the plugin")
	}
}

func TestAssembleNatEmptyAndDefaults(t *testing.T) {
	if got := desired.AssembleNat(nil, tableName); proto.Size(got) != 0 {
		t.Fatalf("no objects: %v", got)
	}
	// the globals owner with default timeouts, no forwarding, default sessions reports the defaults explicitly
	en := natcommon.MustEncode(&nat44ed.EnableSpec{})
	got := desired.AssembleNat([]scheduler.KV{{Key: nat44ed.EnableKey, Value: en}}, tableName)
	want := natDoc(t, `{"enabled": true, "mode": "ed", "forwarding": false, "timeouts": {"udp": 300, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60}}`)
	if !proto.Equal(got, want) {
		t.Fatalf("defaults: %s", protojson.Format(got))
	}
}

func TestNat44EnabledMirrorsSchema(t *testing.T) {
	for js, want := range map[string]bool{
		`{}`:                                  false,
		`{"enabled": true}`:                   true,
		`{"enabled": false, "inside": ["a"]}`: false,
		`{"outputFeature": ["a"]}`:            true,
		`{"pools": [{"name": "p", "range": "10.4.2.1"}]}`: true,
		`{"identityMappings": [{"ip": "10.4.2.1"}]}`:      true,
		`{"nat64": {"enabled": true}}`:                    false,
	} {
		if got := desired.Nat44Enabled(natDoc(t, js)); got != want {
			t.Errorf("%s: %v, want %v", js, got, want)
		}
	}
}
