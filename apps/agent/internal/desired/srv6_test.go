package desired

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/scheduler"
)

// srv6Sink records what the SRv6 builder emits.
type srv6Sink struct {
	kvs   map[scheduler.Key]proto.Message
	ptrs  map[scheduler.Key]string
	errs  []string // "<pointer> <rule>"
	warns []string
}

func newSrv6Sink() *srv6Sink {
	return &srv6Sink{kvs: map[scheduler.Key]proto.Message{}, ptrs: map[scheduler.Key]string{}}
}

func (s *srv6Sink) Add(k scheduler.Key, v proto.Message, pointer string) {
	s.kvs[k], s.ptrs[k] = v, pointer
}
func (s *srv6Sink) Errorf(pointer, rule, _ string, _ ...any) {
	s.errs = append(s.errs, pointer+" "+rule)
}
func (s *srv6Sink) Warnf(pointer, rule, _ string, _ ...any) {
	s.warns = append(s.warns, pointer+" "+rule)
}

func srv6DS(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

var srv6VRFs = func(name string) (uint32, bool) {
	switch name {
	case "default":
		return 0, true
	case "cust":
		return 4001, true
	}
	return 0, false
}

var routingOnly = map[string]bool{"routing": true}

const srv6BuildDoc = `{"routing": {"srv6": {
  "encapSource": "fd00:4::1", "encapHopLimit": 40,
  "localSids": {
    "fd00:4:ff::1": {"behavior": "end", "psp": true},
    "fd00:4:ff::2": {"behavior": "end.x", "vrf": "cust", "interface": "host-w4l0", "nextHop": "fd00:4:1::2"},
    "fd00:4:ff::3": {"behavior": "end.dx4", "interface": "host-w4l1", "nextHop": "10.4.1.2"},
    "fd00:4:ff::a": {"behavior": "end.dt4", "lookupVrf": "cust"}
  },
  "policies": {
    "fd00:4:bb::1": {"sidLists": [{"sids": ["fd00:4:ee::1", "fd00:4:ee::2"]}, {"sids": ["fd00:4:ee::3"], "weight": 5}]},
    "fd00:4:bb::2": {"type": "spray", "encap": false, "vrf": "cust", "sidLists": [{"sids": ["fd00:4:ee::4"], "weight": 1}]},
    "fd00:4:bb::3": {"type": "tef", "encapSource": "fd00:4::3", "sidLists": [{"sids": ["fd00:4:ee::5"]}]}
  },
  "steering": [
    {"type": "l2", "interface": "host-w4l1", "bsid": "fd00:4:bb::1"},
    {"type": "l3", "prefix": "10.4.100.0/24", "vrf": "cust", "bsid": "fd00:4:bb::1"},
    {"type": "l3", "prefix": "fd00:4:100::/48", "bsid": "fd00:4:bb::2"}
  ]
}}}`

func TestSrv6Builder(t *testing.T) {
	s := newSrv6Sink()
	Srv6(s, srv6DS(t, srv6BuildDoc), routingOnly, srv6VRFs)
	if len(s.errs) != 0 {
		t.Fatalf("errors %v", s.errs)
	}
	want := map[scheduler.Key]proto.Message{
		"sr.encap-source/global":    &sr.EncapSource{Address: "fd00:4::1"},
		"sr.encap-hop-limit/global": &sr.EncapHopLimit{HopLimit: 40},
		"sr.localsid/fd00:4:ff::1":  &sr.LocalSid{Sid: "fd00:4:ff::1", Behavior: sr.Behavior_END, EndPsp: true},
		"sr.localsid/fd00:4:ff::2":  &sr.LocalSid{Sid: "fd00:4:ff::2", Behavior: sr.Behavior_END_X, FibTable: 4001, Interface: "host-w4l0", NextHop: "fd00:4:1::2"},
		"sr.localsid/fd00:4:ff::3":  &sr.LocalSid{Sid: "fd00:4:ff::3", Behavior: sr.Behavior_END_DX4, Interface: "host-w4l1", NextHop: "10.4.1.2"},
		"sr.localsid/fd00:4:ff::a":  &sr.LocalSid{Sid: "fd00:4:ff::a", Behavior: sr.Behavior_END_DT4, LookupTable: 4001},
		// the global source fills an encap policy that sets none (D-074)
		"sr.policy/fd00:4:bb::1": &sr.Policy{Bsid: "fd00:4:bb::1", Type: sr.PolicyType_DEFAULT, Encap: true, EncapSrc: "fd00:4::1",
			SidLists: []*sr.SidList{{Sids: []string{"fd00:4:ee::1", "fd00:4:ee::2"}, Weight: 1}, {Sids: []string{"fd00:4:ee::3"}, Weight: 5}}},
		"sr.policy/fd00:4:bb::2":              &sr.Policy{Bsid: "fd00:4:bb::2", Type: sr.PolicyType_SPRAY, FibTable: 4001, SidLists: []*sr.SidList{{Sids: []string{"fd00:4:ee::4"}, Weight: 1}}},
		"sr.policy/fd00:4:bb::3":              &sr.Policy{Bsid: "fd00:4:bb::3", Type: sr.PolicyType_TEF, Encap: true, EncapSrc: "fd00:4::3", SidLists: []*sr.SidList{{Sids: []string{"fd00:4:ee::5"}, Weight: 1}}},
		"sr.steering/l2/host-w4l1":            &sr.Steering{TrafficType: sr.SteerType_L2, Interface: "host-w4l1", Bsid: "fd00:4:bb::1"},
		"sr.steering/ipv4/4001/10.4.100.0/24": &sr.Steering{TrafficType: sr.SteerType_IPV4, Prefix: "10.4.100.0/24", TableId: 4001, Bsid: "fd00:4:bb::1"},
		"sr.steering/ipv6/0/fd00:4:100::/48":  &sr.Steering{TrafficType: sr.SteerType_IPV6, Prefix: "fd00:4:100::/48", Bsid: "fd00:4:bb::2"},
	}
	if len(s.kvs) != len(want) {
		t.Fatalf("got %d objects, want %d: %v", len(s.kvs), len(want), s.kvs)
	}
	for k, w := range want {
		if !proto.Equal(s.kvs[k], w) {
			t.Errorf("%s:\n got %v\nwant %v", k, s.kvs[k], w)
		}
	}
	if s.ptrs["sr.steering/ipv4/4001/10.4.100.0/24"] != "/routing/srv6/steering/1" || s.ptrs["sr.localsid/fd00:4:ff::a"] != "/routing/srv6/localSids/fd00:4:ff::a" {
		t.Fatalf("pointers %v", s.ptrs)
	}
	if strings.Join(s.warns, ",") != "/routing/srv6/encapSource agent.unsupported-field,/routing/srv6/encapHopLimit agent.unsupported-field" {
		t.Fatalf("write-only notes %v", s.warns)
	}
	// Every KV is valid for its descriptor (the sr Canon functions) and keys match the descriptors'.
	c := struct{ l, p, st scheduler.Descriptor }{sr.NewLocalSid(nil, "t"), sr.NewPolicy(nil, "t"), sr.NewSteering(nil, "t")}
	for k, v := range s.kvs {
		var d scheduler.Descriptor
		switch v.(type) {
		case *sr.LocalSid:
			d = c.l
		case *sr.Policy:
			d = c.p
		case *sr.Steering:
			d = c.st
		default:
			continue
		}
		if d.KeyOf(v) != k {
			t.Errorf("key %s, descriptor says %s", k, d.KeyOf(v))
		}
	}
}

func TestSrv6BuilderSkipsOtherDomains(t *testing.T) {
	s := newSrv6Sink()
	Srv6(s, srv6DS(t, srv6BuildDoc), map[string]bool{"interfaces": true}, srv6VRFs)
	Srv6(s, srv6DS(t, `{"routing": {}}`), routingOnly, srv6VRFs)
	if len(s.kvs)+len(s.errs)+len(s.warns) != 0 {
		t.Fatalf("%v %v %v", s.kvs, s.errs, s.warns)
	}
}

func TestSrv6BuilderErrors(t *testing.T) {
	cases := map[string]struct{ doc, want string }{
		"encap without any source": {`{"routing": {"srv6": {"policies": {"fd00:4:bb::1": {"sidLists": [{"sids": ["fd00:4:ee::1"]}]}}}}}`,
			"/routing/srv6/policies/fd00:4:bb::1/encapSource routing.srv6-encap-source"},
		"insert with a source": {`{"routing": {"srv6": {"policies": {"fd00:4:bb::1": {"encap": false, "encapSource": "fd00:4::1", "sidLists": [{"sids": ["fd00:4:ee::1"]}]}}}}}`,
			"/routing/srv6/policies/fd00:4:bb::1/encapSource routing.srv6-encap-source"},
		"17 SIDs": {`{"routing": {"srv6": {"encapSource": "fd00:4::1", "policies": {"fd00:4:bb::1": {"sidLists": [{"sids": ["fd00:4:ee::1","fd00:4:ee::2","fd00:4:ee::3","fd00:4:ee::4","fd00:4:ee::5","fd00:4:ee::6","fd00:4:ee::7","fd00:4:ee::8","fd00:4:ee::9","fd00:4:ee::a","fd00:4:ee::b","fd00:4:ee::c","fd00:4:ee::d","fd00:4:ee::e","fd00:4:ee::f","fd00:4:ee::10","fd00:4:ee::11"]}]}}}}}`,
			"/routing/srv6/policies/fd00:4:bb::1/sidLists/0/sids routing.srv6"},
		"no sid list": {`{"routing": {"srv6": {"encapSource": "fd00:4::1", "policies": {"fd00:4:bb::1": {}}}}}`,
			"/routing/srv6/policies/fd00:4:bb::1/sidLists routing.srv6"},
		"non-canonical segment": {`{"routing": {"srv6": {"encapSource": "fd00:4::1", "policies": {"fd00:4:bb::1": {"sidLists": [{"sids": ["FD00:4:ee::1"]}]}}}}}`,
			"/routing/srv6/policies/fd00:4:bb::1/sidLists/0/sids/0 routing.srv6-canonical"},
		"multicast SID": {`{"routing": {"srv6": {"localSids": {"ff02::1": {"behavior": "end"}}}}}`,
			"/routing/srv6/localSids/ff02::1 routing.srv6-canonical"},
		"sid is a bsid": {`{"routing": {"srv6": {"encapSource": "fd00:4::1", "localSids": {"fd00:4:bb::1": {"behavior": "end"}}, "policies": {"fd00:4:bb::1": {"sidLists": [{"sids": ["fd00:4:ee::1"]}]}}}}}`,
			"/routing/srv6/policies/fd00:4:bb::1 routing.srv6-sid-unique"},
		"end.dx4 with an IPv6 next hop": {`{"routing": {"srv6": {"localSids": {"fd00:4:ff::3": {"behavior": "end.dx4", "interface": "host-w4l1", "nextHop": "fd00:4:1::2"}}}}}`,
			"/routing/srv6/localSids/fd00:4:ff::3/nextHop routing.srv6-behavior-fields"},
		"end with an interface": {`{"routing": {"srv6": {"localSids": {"fd00:4:ff::1": {"behavior": "end", "interface": "host-w4l1"}}}}}`,
			"/routing/srv6/localSids/fd00:4:ff::1/interface routing.srv6-behavior-fields"},
		"end.dt6 without lookup": {`{"routing": {"srv6": {"localSids": {"fd00:4:ff::b": {"behavior": "end.dt6"}}}}}`,
			"/routing/srv6/localSids/fd00:4:ff::b/lookupVrf routing.srv6-behavior-fields"},
		"psp on end.dt4": {`{"routing": {"srv6": {"localSids": {"fd00:4:ff::a": {"behavior": "end.dt4", "lookupVrf": "cust", "psp": true}}}}}`,
			"/routing/srv6/localSids/fd00:4:ff::a/psp routing.srv6-behavior-fields"},
		"unknown VRF": {`{"routing": {"srv6": {"localSids": {"fd00:4:ff::1": {"behavior": "end", "vrf": "nope"}}}}}`,
			"/routing/srv6/localSids/fd00:4:ff::1/vrf routing.srv6-vrf-exists"},
		"IPv4 steering into an insert policy": {`{"routing": {"srv6": {"policies": {"fd00:4:bb::2": {"encap": false, "sidLists": [{"sids": ["fd00:4:ee::4"]}]}}, "steering": [{"type": "l3", "prefix": "10.4.0.0/16", "bsid": "fd00:4:bb::2"}]}}}`,
			"/routing/srv6/steering/0/bsid routing.srv6-steering-encap"},
		"L2 steering into an insert policy": {`{"routing": {"srv6": {"policies": {"fd00:4:bb::2": {"encap": false, "sidLists": [{"sids": ["fd00:4:ee::4"]}]}}, "steering": [{"type": "l2", "interface": "host-w4l1", "bsid": "fd00:4:bb::2"}]}}}`,
			"/routing/srv6/steering/0/bsid routing.srv6-steering-encap"},
		"steering twice": {`{"routing": {"srv6": {"encapSource": "fd00:4::1", "policies": {"fd00:4:bb::1": {"sidLists": [{"sids": ["fd00:4:ee::1"]}]}}, "steering": [{"type": "l2", "interface": "host-w4l1", "bsid": "fd00:4:bb::1"}, {"type": "l2", "interface": "host-w4l1", "bsid": "fd00:4:bb::1"}]}}}`,
			"/routing/srv6/steering/1 routing.srv6-steering-unique"},
		"prefix with host bits": {`{"routing": {"srv6": {"encapSource": "fd00:4::1", "policies": {"fd00:4:bb::1": {"sidLists": [{"sids": ["fd00:4:ee::1"]}]}}, "steering": [{"type": "l3", "prefix": "10.4.0.1/16", "bsid": "fd00:4:bb::1"}]}}}`,
			"/routing/srv6/steering/0/prefix routing.srv6-canonical"},
		"unknown steering type": {`{"routing": {"srv6": {"encapSource": "fd00:4::1", "policies": {"fd00:4:bb::1": {"sidLists": [{"sids": ["fd00:4:ee::1"]}]}}, "steering": [{"type": "l4", "bsid": "fd00:4:bb::1"}]}}}`,
			"/routing/srv6/steering/0/type routing.srv6"},
		"hop limit 0": {`{"routing": {"srv6": {"encapHopLimit": 0}}}`, "/routing/srv6/encapHopLimit routing.srv6"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSrv6Sink()
			Srv6(s, srv6DS(t, c.doc), routingOnly, srv6VRFs)
			if !strings.Contains(strings.Join(s.errs, "\n"), c.want) {
				t.Fatalf("errors %v, want %s", s.errs, c.want)
			}
		})
	}
}

func srv6Names(id uint32) string {
	if id == 4001 {
		return "cust"
	}
	if id == 0 {
		return "default"
	}
	return "4002"
}

// TestSrv6Assemble: Retrieve's objects → routing.srv6 in canonical form; the policy that inherited the
// applied global source has no encapSource; steering sorted (L3 by VRF then prefix, then L2).
func TestSrv6Assemble(t *testing.T) {
	kvs := []scheduler.KV{
		{Key: "sr.steering/l2/host-w4l1", Value: &sr.Steering{TrafficType: sr.SteerType_L2, Interface: "host-w4l1", Bsid: "fd00:4:bb::1"}},
		{Key: "sr.steering/ipv6/0/fd00:4:100::/48", Value: &sr.Steering{TrafficType: sr.SteerType_IPV6, Prefix: "fd00:4:100::/48", Bsid: "fd00:4:bb::2"}},
		{Key: "sr.steering/ipv4/4001/10.4.100.0/24", Value: &sr.Steering{TrafficType: sr.SteerType_IPV4, Prefix: "10.4.100.0/24", TableId: 4001, Bsid: "fd00:4:bb::1"}},
		{Key: "sr.localsid/fd00:4:ff::1", Value: &sr.LocalSid{Sid: "fd00:4:ff::1", Behavior: sr.Behavior_END, EndPsp: true}},
		{Key: "sr.localsid/fd00:4:ff::2", Value: &sr.LocalSid{Sid: "fd00:4:ff::2", Behavior: sr.Behavior_END_X, FibTable: 4001, Interface: "host-w4l0", NextHop: "fd00:4:1::2"}},
		{Key: "sr.localsid/fd00:4:ff::a", Value: &sr.LocalSid{Sid: "fd00:4:ff::a", Behavior: sr.Behavior_END_DT4, LookupTable: 4001}},
		{Key: "sr.policy/fd00:4:bb::1", Value: &sr.Policy{Bsid: "fd00:4:bb::1", Encap: true, EncapSrc: "fd00:4::1", SidLists: []*sr.SidList{{Sids: []string{"fd00:4:ee::1"}, Weight: 1}}}},
		{Key: "sr.policy/fd00:4:bb::2", Value: &sr.Policy{Bsid: "fd00:4:bb::2", Type: sr.PolicyType_SPRAY, FibTable: 4001, SidLists: []*sr.SidList{{Sids: []string{"fd00:4:ee::4"}, Weight: 2}}}},
		{Key: "sr.policy/fd00:4:bb::3", Value: &sr.Policy{Bsid: "fd00:4:bb::3", Type: sr.PolicyType_TEF, Encap: true, EncapSrc: "fd00:4::3", SidLists: []*sr.SidList{{Sids: []string{"fd00:4:ee::5"}, Weight: 1}}}},
	}
	ds := &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{}}
	AssembleSrv6(ds, kvs, routingOnly, srv6Names, Srv6Env{EncapSource: func() string { return "fd00:4::1" }})
	want := srv6DS(t, `{"routing": {"srv6": {
	  "localSids": {
	    "fd00:4:ff::1": {"behavior": "end", "psp": true, "vrf": "default"},
	    "fd00:4:ff::2": {"behavior": "end.x", "psp": false, "vrf": "cust", "interface": "host-w4l0", "nextHop": "fd00:4:1::2"},
	    "fd00:4:ff::a": {"behavior": "end.dt4", "psp": false, "vrf": "default", "lookupVrf": "cust"}
	  },
	  "policies": {
	    "fd00:4:bb::1": {"type": "default", "encap": true, "vrf": "default", "sidLists": [{"sids": ["fd00:4:ee::1"], "weight": 1}]},
	    "fd00:4:bb::2": {"type": "spray", "encap": false, "vrf": "cust", "sidLists": [{"sids": ["fd00:4:ee::4"], "weight": 2}]},
	    "fd00:4:bb::3": {"type": "tef", "encap": true, "vrf": "default", "encapSource": "fd00:4::3", "sidLists": [{"sids": ["fd00:4:ee::5"], "weight": 1}]}
	  },
	  "steering": [
	    {"type": "l3", "prefix": "10.4.100.0/24", "vrf": "cust", "bsid": "fd00:4:bb::1"},
	    {"type": "l3", "prefix": "fd00:4:100::/48", "vrf": "default", "bsid": "fd00:4:bb::2"},
	    {"type": "l2", "interface": "host-w4l1", "bsid": "fd00:4:bb::1"}
	  ]}}}`)
	if !proto.Equal(ds, want) {
		t.Fatalf("assembled\n got %s\nwant %s", protojson.Format(ds), protojson.Format(want))
	}
	// Without an applied global (non-owner, or before the first resync) every source is reported.
	ds = &vrxv1.DesiredState{}
	AssembleSrv6(ds, kvs, routingOnly, srv6Names, Srv6Env{})
	if ds.GetRouting().GetSrv6().GetPolicies()["fd00:4:bb::1"].GetEncapSource() != "fd00:4::1" {
		t.Fatalf("source %s", protojson.Format(ds))
	}
	// Nothing retrieved → no srv6 key; other domains → untouched.
	ds = &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{}}
	AssembleSrv6(ds, nil, routingOnly, srv6Names, Srv6Env{})
	AssembleSrv6(ds, kvs, map[string]bool{"vrfs": true}, srv6Names, Srv6Env{})
	if ds.GetRouting().GetSrv6() != nil {
		t.Fatalf("srv6 %s", protojson.Format(ds))
	}
}

// TestSrv6RoundTrip: what the builder emits, assembled back, is the canonical document.
func TestSrv6RoundTrip(t *testing.T) {
	const canonical = `{"routing": {"srv6": {
	  "localSids": {
	    "fd00:4:ff::1": {"behavior": "end", "psp": false, "vrf": "default"},
	    "fd00:4:ff::3": {"behavior": "end.dx4", "psp": false, "vrf": "default", "interface": "host-w4l1", "nextHop": "10.4.1.2"},
	    "fd00:4:ff::b": {"behavior": "end.dt6", "psp": false, "vrf": "cust", "lookupVrf": "cust"},
	    "fd00:4:ff::c": {"behavior": "end.t", "psp": true, "vrf": "default", "lookupVrf": "cust"}
	  },
	  "policies": {"fd00:4:bb::1": {"type": "default", "encap": true, "vrf": "cust", "encapSource": "fd00:4::1", "sidLists": [{"sids": ["fd00:4:ee::1", "fd00:4:ee::2"], "weight": 7}]}},
	  "steering": [
	    {"type": "l3", "prefix": "10.4.1.0/24", "vrf": "cust", "bsid": "fd00:4:bb::1"},
	    {"type": "l3", "prefix": "10.4.2.0/24", "vrf": "cust", "bsid": "fd00:4:bb::1"},
	    {"type": "l3", "prefix": "10.4.0.0/16", "vrf": "default", "bsid": "fd00:4:bb::1"},
	    {"type": "l2", "interface": "host-w4l0", "bsid": "fd00:4:bb::1"}
	  ]}}}`
	s := newSrv6Sink()
	in := srv6DS(t, canonical)
	Srv6(s, in, routingOnly, srv6VRFs)
	if len(s.errs) != 0 {
		t.Fatal(s.errs)
	}
	var kvs []scheduler.KV
	for k, v := range s.kvs {
		kvs = append(kvs, scheduler.KV{Key: k, Value: v})
	}
	out := &vrxv1.DesiredState{}
	AssembleSrv6(out, kvs, routingOnly, srv6Names, Srv6Env{})
	if !proto.Equal(out, in) {
		t.Fatalf("round trip\n got %s\nwant %s", protojson.Format(out), protojson.Format(in))
	}
}
