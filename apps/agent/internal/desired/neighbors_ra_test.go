package desired

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/arp"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	ipneighbor "ngfw/agent/internal/descriptors/ip_neighbor"
	"ngfw/agent/internal/scheduler"
)

type sink struct {
	kvs    []scheduler.KV
	ptrs   map[scheduler.Key]string
	issues []string
}

func (s *sink) Add(k scheduler.Key, v proto.Message, pointer string) {
	if s.ptrs == nil {
		s.ptrs = map[scheduler.Key]string{}
	}
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
	s.ptrs[k] = pointer
}
func (s *sink) Errorf(pointer, rule, format string, a ...any) {
	s.issues = append(s.issues, "E "+pointer+" "+rule+" "+fmt.Sprintf(format, a...))
}
func (s *sink) Warnf(pointer, rule, format string, a ...any) {
	s.issues = append(s.issues, "W "+pointer+" "+rule+" "+fmt.Sprintf(format, a...))
}

func (s *sink) keys() []string {
	var out []string
	for _, kv := range s.kvs {
		out = append(out, string(kv.Key)+" @"+s.ptrs[kv.Key])
	}
	sort.Strings(out)
	return out
}

func parse(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

const nraDoc = `{
  "vrfs": {"red": {"id": 9001, "proxyArpRanges": [{"low": "10.9.3.20", "high": "10.9.3.29"}, {"low": "10.9.3.1", "high": "10.9.3.9"}]}},
  "interfaces": {
    "loop901": {"ipv6": ["2001:db8:9:1::1/64"], "proxyArp": true, "proxyNd": ["2001:db8:9:1::99"],
      "ipv6Ra": {"suppress": false, "managed": false, "other": true, "lifetimeSec": 1800, "maxIntervalSec": 600, "minIntervalSec": 200,
        "prefixes": {"2001:db8:9:1::/64": {"validSec": 86400, "preferredSec": 14400, "offLink": false, "noAutoconfig": false}}}},
    "host-w9l0": {"ipv6": ["2001:db8:9:2::1/64"], "proxyArp": false,
      "ipv6Ra": {"suppress": true, "managed": false, "other": false, "lifetimeSec": 600, "maxIntervalSec": 200, "minIntervalSec": 150},
      "subinterfaces": {"100": {"vlanId": 100, "ipv6": ["2001:db8:9:100::1/64"], "ipv6Ra": {"suppress": false}}}}
  },
  "routing": {"neighbors": {
    "static": [{"interface": "loop901", "ip": "2001:DB8:9:1::0050", "mac": "02:AA:00:00:09:50", "noFibEntry": true},
               {"interface": "host-w9l0", "ip": "10.9.2.50", "mac": "02:00:00:00:92:50"}],
    "ipv4Limits": {"maxNumber": 100000, "maxAgeSec": 300, "recycle": true},
    "dad": {"transmits": 2, "delayMs": 500}}}
}`

var all = map[string]bool{"interfaces": true, "vrfs": true, "routing": true}

func vrfID(name string) (uint32, bool) {
	switch name {
	case "", "default":
		return 0, true
	case "red":
		return 9001, true
	}
	return 0, false
}

func TestNeighborsRaProjectionNonOwner(t *testing.T) {
	ConfigureNeighborsRa(NeighborsRaOptions{})
	s := &sink{}
	NeighborsRa(s, parse(t, nraDoc), all, vrfID)
	want := []string{
		"arp.proxy-interface/loop901 @/interfaces/loop901/proxyArp",
		"arp.proxy-range/9001/10.9.3.1-10.9.3.9 @/vrfs/red/proxyArpRanges/1",
		"arp.proxy-range/9001/10.9.3.20-10.9.3.29 @/vrfs/red/proxyArpRanges/0",
		"ip-neighbor.neighbor/host-w9l0/10.9.2.50 @/routing/neighbors/static/1",
		"ip-neighbor.neighbor/loop901/2001:db8:9:1::50 @/routing/neighbors/static/0",
		"ip6-nd.ra-config/host-w9l0.100 @/interfaces/host-w9l0/subinterfaces/100/ipv6Ra",
		"ip6-nd.ra-config/loop901 @/interfaces/loop901/ipv6Ra",
		"ip6-nd.ra-prefix/loop901/2001:db8:9:1::/64 @/interfaces/loop901/ipv6Ra/prefixes/2001:db8:9:1::~164",
	}
	if got := s.keys(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("keys\n%s", strings.Join(got, "\n"))
	}
	// the host-w9l0 RA object holds only defaults = VPP's fresh state: no object (DF-2 Retrieve omits it)
	wantIssues := []string{
		"W /interfaces/loop901/proxyNd agent.unsupported-field proxy ND is experimental and off in this agent (start it with VRX_DF2_PROXY_ND=1; VPP V12): not applied",
		"W /routing/neighbors/ipv4Limits agent.unsupported-field neighbour-table limits are VPP-wide: only the globals owner applies them (D-071); not applied by this agent",
		"W /routing/neighbors/dad agent.unsupported-field duplicate address detection is VPP-wide: only the globals owner applies it (D-071); not applied by this agent",
	}
	if strings.Join(s.issues, "\n") != strings.Join(wantIssues, "\n") {
		t.Fatalf("issues\n%s", strings.Join(s.issues, "\n"))
	}
	for _, kv := range s.kvs {
		switch v := kv.Value.(type) {
		case *ip6nd.RaConfig:
			if v.GetInterface() == "loop901" && !proto.Equal(v, &ip6nd.RaConfig{Interface: "loop901", Other: true, RouterLifetime: 1800, MaxInterval: 600, MinInterval: 200, InitialCount: 3, InitialInterval: 16}) {
				t.Fatalf("ra-config %v", v)
			}
			if v.GetInterface() == "host-w9l0.100" && (v.GetSuppress() || v.GetRouterLifetime() != 600 || v.GetMinInterval() != 150) {
				t.Fatalf("sub-interface ra-config %v", v)
			}
		case *ipneighbor.Neighbor:
			if v.GetInterface() == "loop901" && !proto.Equal(v, &ipneighbor.Neighbor{Interface: "loop901", IpAddress: "2001:db8:9:1::50", MacAddress: "02:aa:00:00:09:50", NoFibEntry: true}) {
				t.Fatalf("canonical neighbour %v", v)
			}
		case *ip6nd.RaPrefix:
			if !proto.Equal(v, &ip6nd.RaPrefix{Interface: "loop901", Prefix: "2001:db8:9:1::/64", ValidLifetime: 86400, PreferredLifetime: 14400}) {
				t.Fatalf("prefix %v", v)
			}
		}
	}
}

func TestNeighborsRaProjectionGlobalsOwnerAndProxyNd(t *testing.T) {
	ConfigureNeighborsRa(NeighborsRaOptions{GlobalsOwner: true, ProxyNd: true})
	t.Cleanup(func() { ConfigureNeighborsRa(NeighborsRaOptions{}) })
	s := &sink{}
	NeighborsRa(s, parse(t, nraDoc), map[string]bool{"routing": true, "interfaces": true}, vrfID)
	if len(s.issues) != 0 {
		t.Fatalf("issues %v", s.issues)
	}
	byKey := map[string]proto.Message{}
	for _, kv := range s.kvs {
		byKey[string(kv.Key)] = kv.Value
	}
	if v := byKey["ip-neighbor.config/ipv4"]; !proto.Equal(v, &ipneighbor.Config{MaxNumber: 100000, MaxAge: 300, Recycle: true}) {
		t.Fatalf("ipv4 limits %v", v)
	}
	// unset ipv6Limits: the VPP defaults, so the always-retrieved singleton converges
	if v := byKey["ip-neighbor.config/ipv6"]; !proto.Equal(v, &ipneighbor.Config{Af: 1, MaxNumber: 50000}) {
		t.Fatalf("ipv6 limits %v", v)
	}
	if v := byKey["ip6-nd.dad/global"]; !proto.Equal(v, &ip6nd.Dad{Transmits: 2, RetransmitDelay: 0.5}) {
		t.Fatalf("dad %v", v)
	}
	if v := byKey["ip6-nd.proxy/loop901/2001:db8:9:1::99"]; !proto.Equal(v, &ip6nd.ProxyNd{Interface: "loop901", Address: "2001:db8:9:1::99"}) {
		t.Fatalf("proxy nd %v", v)
	}
	if _, ok := byKey["arp.proxy-range/9001/10.9.3.1-10.9.3.9"]; ok {
		t.Fatal("vrfs not in scope")
	}
	// a document without routing.neighbors still pins both families to the defaults (globals owner)
	s = &sink{}
	NeighborsRa(s, parse(t, `{"routing": {}}`), map[string]bool{"routing": true}, vrfID)
	if got := s.keys(); strings.Join(got, ",") != "ip-neighbor.config/ipv4 @/routing,ip-neighbor.config/ipv6 @/routing" {
		t.Fatalf("defaults %v", got)
	}
}

// Assemble(project(doc)) is the canonical document: every leaf the projection emitted comes back in the document's
// form, the stored "off" leaves are reported in their off state, lists are sorted.
func TestNeighborsRaAssembleRoundTrip(t *testing.T) {
	ConfigureNeighborsRa(NeighborsRaOptions{GlobalsOwner: true})
	t.Cleanup(func() { ConfigureNeighborsRa(NeighborsRaOptions{}) })
	in := parse(t, nraDoc)
	s := &sink{}
	NeighborsRa(s, in, all, vrfID)
	ds := &vrxv1.DesiredState{
		Vrfs: map[string]*vrxv1.Vrf{"red": {Id: proto.Uint32(9001)}},
		Interfaces: map[string]*vrxv1.Interface{
			"loop901":   {Enabled: proto.Bool(false)},
			"host-w9l0": {Enabled: proto.Bool(false), Subinterfaces: map[string]*vrxv1.Subinterface{"100": {VlanId: proto.Uint32(100)}}},
		},
		Routing: &vrxv1.RoutingConfig{},
	}
	names := func(id uint32) string {
		if id == 9001 {
			return "red"
		}
		return "default"
	}
	AssembleNeighborsRa(ds, s.kvs, all, in.GetInterfaces(), names)
	want := parse(t, `{
  "vrfs": {"red": {"id": 9001, "proxyArpRanges": [{"low": "10.9.3.1", "high": "10.9.3.9"}, {"low": "10.9.3.20", "high": "10.9.3.29"}]}},
  "interfaces": {
    "loop901": {"enabled": false, "proxyArp": true,
      "ipv6Ra": {"suppress": false, "managed": false, "other": true, "lifetimeSec": 1800, "maxIntervalSec": 600, "minIntervalSec": 200,
        "prefixes": {"2001:db8:9:1::/64": {"validSec": 86400, "preferredSec": 14400, "offLink": false, "noAutoconfig": false}}}},
    "host-w9l0": {"enabled": false, "proxyArp": false,
      "ipv6Ra": {"suppress": true, "managed": false, "other": false, "lifetimeSec": 600, "maxIntervalSec": 200, "minIntervalSec": 150},
      "subinterfaces": {"100": {"vlanId": 100,
        "ipv6Ra": {"suppress": false, "managed": false, "other": false, "lifetimeSec": 600, "maxIntervalSec": 200, "minIntervalSec": 150}}}}
  },
  "routing": {"neighbors": {
    "static": [{"interface": "host-w9l0", "ip": "10.9.2.50", "mac": "02:00:00:00:92:50", "noFibEntry": false},
               {"interface": "loop901", "ip": "2001:db8:9:1::50", "mac": "02:aa:00:00:09:50", "noFibEntry": true}],
    "ipv4Limits": {"maxNumber": 100000, "maxAgeSec": 300, "recycle": true},
    "dad": {"transmits": 2, "delayMs": 500}}}
}`)
	if !proto.Equal(ds, want) {
		t.Fatalf("assembled\n%s", protojson.Format(ds))
	}
	// nothing retrieved and nothing stored: nothing added (existing documents keep their exact shape)
	empty := &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{"loop1": {Enabled: proto.Bool(false)}}, Routing: &vrxv1.RoutingConfig{}}
	AssembleNeighborsRa(empty, nil, all, nil, names)
	if !proto.Equal(empty, &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{"loop1": {Enabled: proto.Bool(false)}}, Routing: &vrxv1.RoutingConfig{}}) {
		t.Fatalf("empty %s", protojson.Format(empty))
	}
	// VPP-default limits are "absent"
	ds = &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{}}
	AssembleNeighborsRa(ds, []scheduler.KV{{Key: "ip-neighbor.config/ipv4", Value: &ipneighbor.Config{MaxNumber: 50000}}}, all, nil, names)
	if ds.GetRouting().GetNeighbors() != nil {
		t.Fatalf("defaults reported: %v", ds.GetRouting())
	}
	_ = arp.RangeName
}

func TestIsDefaultRaConfig(t *testing.T) {
	if !IsDefaultRaConfig(RaConfigOf("x", &vrxv1.Ipv6Ra{})) {
		t.Fatal("empty ipv6Ra = VPP fresh state")
	}
	for _, ra := range []*vrxv1.Ipv6Ra{{Suppress: proto.Bool(false)}, {Managed: proto.Bool(true)}, {MaxIntervalSec: proto.Uint32(300)}, {LifetimeSec: proto.Uint32(0)}} {
		if IsDefaultRaConfig(RaConfigOf("x", ra)) {
			t.Fatalf("%v is not the default", ra)
		}
	}
}
