package sr_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/binapi/sr_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/scheduler"
)

type steerKey struct {
	typ   sr_types.SrSteer
	table uint32
	pfx   ip_types.Prefix
	sw    uint32
}

// fakeSR models the SRv6 API of VPP 26.06 including the conditions that crash it
// (unchecked fib_table_find results) — "crashes" must stay 0.
type fakeSR struct {
	*df6test.FakeVPP
	tables   map[[2]uint32]bool // {table id, is_ip6}
	localsid map[ip_types.IP6Address]*srapi.SrLocalsidsDetails
	policies []*srapi.SrPoliciesV2Details
	steer    map[steerKey]*srapi.SrSteeringPolDetails
	encapSrc ip_types.IP6Address
	hopLimit uint8
	crashes  int
}

func (f *fakeSR) hasTable(id uint32, ip6 bool) bool {
	return id == 0 || f.tables[[2]uint32{id, b2u(ip6)}]
}

func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func (f *fakeSR) policy(bsid ip_types.IP6Address) (int, *srapi.SrPoliciesV2Details) {
	for i, p := range f.policies {
		if p.Bsid == bsid {
			return i, p
		}
	}
	return -1, nil
}

func newFakeSR() *fakeSR {
	f := &fakeSR{FakeVPP: df6test.NewFakeVPP(), tables: map[[2]uint32]bool{}, localsid: map[ip_types.IP6Address]*srapi.SrLocalsidsDetails{}, steer: map[steerKey]*srapi.SrSteeringPolDetails{}, hopLimit: 64}
	f.On("ip_table_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for k := range f.tables {
			out = append(out, &ip.IPTableDetails{Table: ip.IPTable{TableID: k[0], IsIP6: k[1] == 1}})
		}
		return out, nil
	})
	f.On("sr_localsid_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrLocalsidAddDel)
		if !f.hasTable(r.FibTable, true) {
			if r.IsDel {
				f.crashes++
			}
			return []api.Message{&srapi.SrLocalsidAddDelReply{Retval: -3}}, nil
		}
		if r.IsDel {
			if _, ok := f.localsid[r.Localsid]; !ok {
				return []api.Message{&srapi.SrLocalsidAddDelReply{Retval: -2}}, nil
			}
			delete(f.localsid, r.Localsid)
			return []api.Message{&srapi.SrLocalsidAddDelReply{}}, nil
		}
		if _, ok := f.localsid[r.Localsid]; ok {
			return []api.Message{&srapi.SrLocalsidAddDelReply{Retval: -1}}, nil
		}
		d := &srapi.SrLocalsidsDetails{Addr: r.Localsid, EndPsp: r.EndPsp, Behavior: r.Behavior, FibTable: r.FibTable, XconnectIfaceOrVrfTable: uint32(r.SwIfIndex)}
		switch r.Behavior {
		case sr_types.SR_BEHAVIOR_API_X, sr_types.SR_BEHAVIOR_API_DX4, sr_types.SR_BEHAVIOR_API_DX6:
			d.XconnectNhAddr = r.NhAddr
		case sr_types.SR_BEHAVIOR_API_END:
			d.XconnectIfaceOrVrfTable = 0
		case sr_types.SR_BEHAVIOR_API_DT4:
			if !f.hasTable(uint32(r.SwIfIndex), false) {
				f.crashes++
			}
		case sr_types.SR_BEHAVIOR_API_T, sr_types.SR_BEHAVIOR_API_DT6:
			if !f.hasTable(uint32(r.SwIfIndex), true) {
				f.crashes++
			}
		}
		f.localsid[r.Localsid] = d
		return []api.Message{&srapi.SrLocalsidAddDelReply{}}, nil
	})
	f.On("sr_localsids_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, d := range f.localsid {
			c := *d
			out = append(out, &c)
		}
		return out, nil
	})
	f.On("sr_policy_add_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrPolicyAddV2)
		if !f.hasTable(r.FibTable, true) {
			f.crashes++
			return []api.Message{&srapi.SrPolicyAddV2Reply{Retval: -1}}, nil
		}
		if _, p := f.policy(r.BsidAddr); p != nil {
			return []api.Message{&srapi.SrPolicyAddV2Reply{Retval: -12}}, nil
		}
		src := r.EncapSrc
		if src == (ip_types.IP6Address{}) {
			src = f.encapSrc
		}
		sl := r.Sids
		sl.Weight = r.Weight
		f.policies = append(f.policies, &srapi.SrPoliciesV2Details{Bsid: r.BsidAddr, EncapSrc: src, Type: r.Type, IsEncap: r.IsEncap, FibTable: r.FibTable, SidLists: []srapi.Srv6SidList{sl}})
		return []api.Message{&srapi.SrPolicyAddV2Reply{}}, nil
	})
	f.On("sr_policy_mod_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrPolicyModV2)
		_, p := f.policy(r.BsidAddr)
		if p == nil || r.Operation != sr_types.SR_POLICY_OP_API_ADD {
			return []api.Message{&srapi.SrPolicyModV2Reply{Retval: -1}}, nil
		}
		sl := r.Sids
		sl.Weight = r.Weight
		p.SidLists = append(p.SidLists, sl)
		return []api.Message{&srapi.SrPolicyModV2Reply{}}, nil
	})
	f.On("sr_policy_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrPolicyDel)
		i, p := f.policy(r.BsidAddr)
		if p == nil {
			return []api.Message{&srapi.SrPolicyDelReply{Retval: -1}}, nil
		}
		for _, s := range f.steer {
			if s.Bsid == r.BsidAddr {
				f.crashes++ // dangling steering entry
			}
		}
		f.policies = append(f.policies[:i], f.policies[i+1:]...)
		return []api.Message{&srapi.SrPolicyDelReply{}}, nil
	})
	f.On("sr_policies_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, p := range f.policies {
			c := *p
			c.NumSidLists = uint8(len(c.SidLists)) //nolint:gosec // small
			out = append(out, &c)
		}
		return out, nil
	})
	f.On("sr_steering_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrSteeringAddDel)
		k := steerKey{typ: r.TrafficType}
		if r.TrafficType == sr_types.SR_STEER_API_L2 {
			k.sw = uint32(r.SwIfIndex)
		} else {
			k.table, k.pfx = r.TableID, r.Prefix
			if !f.hasTable(r.TableID, r.TrafficType == sr_types.SR_STEER_API_IPV6) {
				f.crashes++
				return []api.Message{&srapi.SrSteeringAddDelReply{Retval: -1}}, nil
			}
		}
		if r.IsDel {
			if _, ok := f.steer[k]; !ok {
				return []api.Message{&srapi.SrSteeringAddDelReply{Retval: -4}}, nil
			}
			delete(f.steer, k)
			return []api.Message{&srapi.SrSteeringAddDelReply{}}, nil
		}
		if _, p := f.policy(r.BsidAddr); p == nil {
			return []api.Message{&srapi.SrSteeringAddDelReply{Retval: -2}}, nil
		}
		d := &srapi.SrSteeringPolDetails{TrafficType: r.TrafficType, Bsid: r.BsidAddr, SwIfIndex: r.SwIfIndex}
		if r.TrafficType != sr_types.SR_STEER_API_L2 {
			d.FibTable, d.Prefix, d.SwIfIndex = r.TableID, r.Prefix, 0
		}
		f.steer[k] = d // add on an existing key re-points it
		return []api.Message{&srapi.SrSteeringAddDelReply{}}, nil
	})
	f.On("sr_steering_pol_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, s := range f.steer {
			c := *s
			out = append(out, &c)
		}
		return out, nil
	})
	f.On("sr_set_encap_source", func(req api.Message) ([]api.Message, error) {
		f.encapSrc = req.(*srapi.SrSetEncapSource).EncapsSource
		return []api.Message{&srapi.SrSetEncapSourceReply{}}, nil
	})
	f.On("sr_set_encap_hop_limit", func(req api.Message) ([]api.Message, error) {
		f.hopLimit = req.(*srapi.SrSetEncapHopLimit).HopLimit
		return []api.Message{&srapi.SrSetEncapHopLimitReply{}}, nil
	})
	return f
}

// owner is the claim owner of these tests; each test starts from an empty claim store.
func freshOwner(t *testing.T) string {
	o := "w11" + t.Name()
	iface.SetClaimStore(o, nil)
	return o
}

func retrieveMap(t *testing.T, d scheduler.Descriptor) map[scheduler.Key]proto.Message {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := map[scheduler.Key]proto.Message{}
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
	return m
}

func TestLocalSid(t *testing.T) {
	ctx := context.Background()
	f := newFakeSR()
	f.tables[[2]uint32{11001, 1}] = true
	f.tables[[2]uint32{11002, 0}] = true
	owner := freshOwner(t)
	loop := f.AddInterface("loop1101", owner+":loop1101")
	// Another slot's SID must stay invisible.
	f.localsid[df6test.IP6("fd03::1")] = &srapi.SrLocalsidsDetails{Addr: df6test.IP6("fd03::1"), Behavior: sr_types.SR_BEHAVIOR_API_END}

	reg := scheduler.NewRegistry()
	sr.Register(reg, f, owner)
	if reg.Len() != 5 {
		t.Fatalf("registered %d", reg.Len())
	}
	d := sr.NewLocalSid(f, owner)
	cases := []*sr.LocalSid{
		{Sid: "fd11:1::1", Behavior: sr.Behavior_END, EndPsp: true},
		{Sid: "fd11:1::2", Behavior: sr.Behavior_END_X, Interface: "loop1101", NextHop: "fd11:2::1", FibTable: 11001},
		{Sid: "fd11:1::3", Behavior: sr.Behavior_END_DX4, Interface: "loop1101", NextHop: "10.11.2.1"},
		{Sid: "fd11:1::4", Behavior: sr.Behavior_END_DT4, LookupTable: 11002},
		{Sid: "fd11:1::5", Behavior: sr.Behavior_END_DT6, LookupTable: 11001, FibTable: 11001},
		{Sid: "fd11:1::6", Behavior: sr.Behavior_END_DX2, Interface: "loop1101"},
	}
	if k := d.KeyOf(cases[1]); k != "sr.localsid/fd11:1::2" {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(cases[1])
	if len(deps) != 2 || deps[0].Key != "vrf/11001" || deps[1].Key != "interface/loop1101" {
		t.Fatalf("deps = %+v", deps)
	}
	if deps := d.Dependencies(cases[3]); len(deps) != 1 || deps[0].Key != "vrf/11002" {
		t.Fatalf("dt4 deps = %+v", deps)
	}
	for _, c := range cases {
		if _, err := d.Create(ctx, c); err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
	}
	x := f.CallsNamed("sr_localsid_add_del")[1].(*srapi.SrLocalsidAddDel)
	if uint32(x.SwIfIndex) != loop || x.Behavior != sr_types.SR_BEHAVIOR_API_X || x.FibTable != 11001 || x.NhAddr != df6test.Addr("fd11:2::1") {
		t.Fatalf("end.x request = %+v", x)
	}
	if dt4 := f.CallsNamed("sr_localsid_add_del")[3].(*srapi.SrLocalsidAddDel); uint32(dt4.SwIfIndex) != 11002 {
		t.Fatalf("dt4 request carries the lookup table in sw_if_index: %+v", dt4)
	}
	got := retrieveMap(t, d)
	if len(got) != len(cases) {
		t.Fatalf("Retrieve = %v", got)
	}
	for _, c := range cases {
		if !proto.Equal(got[d.KeyOf(c)], c) {
			t.Errorf("Retrieve %v != %v", got[d.KeyOf(c)], c)
		}
	}
	if _, err := d.Update(ctx, cases[0], cases[0], nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update = %v", err)
	}
	// Missing tables are refused before anything is sent (VPP would crash on some).
	n := len(f.CallsNamed("sr_localsid_add_del"))
	for _, b := range []*sr.LocalSid{
		{Sid: "fd11:1::9", Behavior: sr.Behavior_END, FibTable: 11999},
		{Sid: "fd11:1::9", Behavior: sr.Behavior_END_T, LookupTable: 11002}, // 11002 is IPv4 only
	} {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrNoSuchTable) {
			t.Errorf("%v: %v", b, err)
		}
	}
	for _, b := range []*sr.LocalSid{
		{Sid: "10.11.0.1", Behavior: sr.Behavior_END},
		{Sid: "fd11::9"},
		{Sid: "fd11::9", Behavior: sr.Behavior_END_X, NextHop: "fd11::1"},
		{Sid: "fd11::9", Behavior: sr.Behavior_END_DX4, Interface: "loop1101", NextHop: "fd11::1"},
		{Sid: "fd11::9", Behavior: sr.Behavior_END, LookupTable: 3},
		{Sid: "fd11::9", Behavior: sr.Behavior_END_DT4, EndPsp: true},
		{Sid: "fd11::9", Behavior: 4},
	} {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	if len(f.CallsNamed("sr_localsid_add_del")) != n {
		t.Fatal("invalid localsid reached VPP")
	}
	for _, c := range cases {
		if err := d.Delete(ctx, c, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Delete(ctx, cases[0], nil); err != nil {
		t.Fatalf("second delete = %v", err)
	}
	if got := retrieveMap(t, d); len(got) != 0 {
		t.Fatalf("after delete: %v", got)
	}
	if len(f.localsid) != 1 || f.crashes != 0 {
		t.Fatalf("other SID touched or crash: %d %d", len(f.localsid), f.crashes)
	}
}

func TestPolicyAndSteering(t *testing.T) {
	ctx := context.Background()
	f := newFakeSR()
	f.tables[[2]uint32{11001, 1}] = true
	f.tables[[2]uint32{11002, 0}] = true
	owner := freshOwner(t)
	f.AddInterface("loop1101", owner+":loop1101")
	f.encapSrc = df6test.IP6("fd99::99") // someone's global encap source
	p := sr.NewPolicy(f, owner)
	s := sr.NewSteering(f, owner)

	encap := &sr.Policy{Bsid: "fd11:b::1", Encap: true, EncapSrc: "fd11::1", FibTable: 11001, SidLists: []*sr.SidList{
		{Sids: []string{"fd11:1::1", "fd11:1::2"}, Weight: 1},
		{Sids: []string{"fd11:1::3"}, Weight: 3},
	}}
	insert := &sr.Policy{Bsid: "fd11:b::2", Type: sr.PolicyType_SPRAY, SidLists: []*sr.SidList{{Sids: []string{"fd11:1::4"}}}}
	if k := p.KeyOf(encap); k != "sr.policy/fd11:b::1" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := p.Dependencies(encap); len(deps) != 1 || deps[0].Key != "vrf/11001" {
		t.Fatalf("deps = %+v", deps)
	}
	for _, c := range []*sr.Policy{encap, insert} {
		if _, err := p.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if mods := f.CallsNamed("sr_policy_mod_v2"); len(mods) != 1 || mods[0].(*srapi.SrPolicyModV2).Weight != 3 {
		t.Fatalf("mods = %+v", mods)
	}
	got := retrieveMap(t, p)
	if len(got) != 2 || !proto.Equal(got[p.KeyOf(encap)], encap) || !proto.Equal(got[p.KeyOf(insert)], insert) {
		t.Fatalf("Retrieve = %v", got)
	}

	l3 := &sr.Steering{TrafficType: sr.SteerType_IPV4, Prefix: "10.11.9.0/24", TableId: 11002, Bsid: "fd11:b::1"}
	l3v6 := &sr.Steering{TrafficType: sr.SteerType_IPV6, Prefix: "fd11:9::/64", Bsid: "fd11:b::2"}
	l2 := &sr.Steering{TrafficType: sr.SteerType_L2, Interface: "loop1101", Bsid: "fd11:b::1"}
	if k := s.KeyOf(l3); k != "sr.steering/ipv4/11002/10.11.9.0/24" {
		t.Fatalf("KeyOf = %s", k)
	}
	if _, err := s.Create(ctx, &sr.Steering{TrafficType: sr.SteerType_IPV4, Prefix: "10.11.9.7/24", TableId: 11002, Bsid: "fd11:b::1"}); err == nil {
		t.Fatal("prefix with host bits accepted (D-149)")
	}
	deps := s.Dependencies(l3)
	if len(deps) != 2 || deps[0].Key != "sr.policy/fd11:b::1" || deps[1].Key != "vrf/11002" {
		t.Fatalf("deps = %+v", deps)
	}
	if deps := s.Dependencies(l2); len(deps) != 2 || deps[1].Key != "interface/loop1101" {
		t.Fatalf("l2 deps = %+v", deps)
	}
	for _, c := range []*sr.Steering{l3, l3v6, l2} {
		if _, err := s.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	sg := retrieveMap(t, s)
	want := proto.Clone(l3).(*sr.Steering)
	want.Prefix = "10.11.9.0/24"
	if len(sg) != 3 || !proto.Equal(sg[s.KeyOf(l3)], want) || !proto.Equal(sg[s.KeyOf(l3v6)], l3v6) || !proto.Equal(sg[s.KeyOf(l2)], l2) {
		t.Fatalf("steering Retrieve = %v", sg)
	}
	// Policy still steered into → refused.
	if err := p.Delete(ctx, encap, nil); !errors.Is(err, sr.ErrPolicyInUse) {
		t.Fatalf("delete in-use policy = %v", err)
	}
	// BSID change re-points in place; prefix change recreates.
	moved := proto.Clone(want).(*sr.Steering)
	moved.Bsid = "fd11:b::2"
	if _, err := s.Update(ctx, want, moved, nil); err != nil {
		t.Fatal(err)
	}
	if got := retrieveMap(t, s)[s.KeyOf(moved)]; !proto.Equal(got, moved) {
		t.Fatalf("after re-point: %v", got)
	}
	other := proto.Clone(moved).(*sr.Steering)
	other.Prefix = "10.11.8.0/24"
	if _, err := s.Update(ctx, moved, other, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("prefix change = %v", err)
	}
	if _, err := s.Create(ctx, &sr.Steering{TrafficType: sr.SteerType_IPV6, Prefix: "fd11:8::/64", TableId: 11002, Bsid: "fd11:b::1"}); !errors.Is(err, df6.ErrNoSuchTable) {
		t.Fatalf("v6 steering into v4-only table = %v", err)
	}
	for _, b := range []*sr.Steering{
		{TrafficType: sr.SteerType_IPV4, Prefix: "fd11::/64", Bsid: "fd11:b::1"},
		{TrafficType: sr.SteerType_L2, Bsid: "fd11:b::1"},
		{TrafficType: sr.SteerType_IPV4, Prefix: "10.11.0.0/16", Interface: "loop1101", Bsid: "fd11:b::1"},
		{Prefix: "10.11.0.0/16", Bsid: "fd11:b::1"},
	} {
		if _, err := s.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	for _, b := range []*sr.Policy{
		{Bsid: "fd11:b::9"},
		{Bsid: "fd11:b::9", Encap: true, SidLists: []*sr.SidList{{Sids: []string{"fd11::1"}}}},
		{Bsid: "fd11:b::9", EncapSrc: "fd11::1", SidLists: []*sr.SidList{{Sids: []string{"fd11::1"}}}},
		{Bsid: "fd11:b::9", SidLists: []*sr.SidList{{}}},
		{Bsid: "fd11:b::9", SidLists: []*sr.SidList{{Sids: make([]string, 17)}}},
		{Bsid: "10.11.0.1", SidLists: []*sr.SidList{{Sids: []string{"fd11::1"}}}},
	} {
		if _, err := p.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	if _, err := p.Create(ctx, &sr.Policy{Bsid: "fd11:b::9", FibTable: 11002, SidLists: []*sr.SidList{{Sids: []string{"fd11::1"}}}}); !errors.Is(err, df6.ErrNoSuchTable) {
		t.Fatalf("policy in v4-only table = %v", err)
	}
	// Delete: dependents first, twice is fine.
	for _, kv := range func() []scheduler.KV { k, _ := s.Retrieve(ctx); return k }() {
		if err := s.Delete(ctx, kv.Value, nil); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, kv.Value, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []*sr.Policy{encap, insert, encap} {
		if err := p.Delete(ctx, c, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(retrieveMap(t, p)) != 0 || len(retrieveMap(t, s)) != 0 || f.crashes != 0 {
		t.Fatalf("leftovers or crashes (%d)", f.crashes)
	}
}

func TestGlobals(t *testing.T) {
	ctx := context.Background()
	f := newFakeSR()
	src := sr.NewEncapSource(f)
	hl := sr.NewEncapHopLimit(f)
	if k := src.KeyOf(&sr.EncapSource{}); k != "sr.encap-source/global" {
		t.Fatalf("KeyOf = %s", k)
	}
	if _, err := src.Create(ctx, &sr.EncapSource{Address: "fd11::1"}); err != nil || f.encapSrc != df6test.IP6("fd11::1") {
		t.Fatalf("set: %v", err)
	}
	if _, err := src.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	if err := src.Delete(ctx, &sr.EncapSource{Address: "fd11::1"}, nil); err != nil || f.encapSrc != (ip_types.IP6Address{}) {
		t.Fatalf("reset: %v", err)
	}
	if _, err := src.Create(ctx, &sr.EncapSource{Address: "10.0.0.1"}); !errors.Is(err, df6.ErrBadValue) {
		t.Fatalf("v4 source = %v", err)
	}
	if _, err := hl.Update(ctx, nil, &sr.EncapHopLimit{HopLimit: 9}, nil); err != nil || f.hopLimit != 9 {
		t.Fatalf("hop limit: %v", err)
	}
	if err := hl.Delete(ctx, &sr.EncapHopLimit{HopLimit: 9}, nil); err != nil || f.hopLimit != sr.DefaultEncapHopLimit {
		t.Fatalf("hop limit reset: %v %d", err, f.hopLimit)
	}
	for _, v := range []uint32{0, 256} {
		if _, err := hl.Create(ctx, &sr.EncapHopLimit{HopLimit: v}); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%d: %v", v, err)
		}
	}
}

// TestClaims (review H2/M3): untagged SR objects are ours only by claim — Retrieve never
// reports, Create never takes over and Delete never touches another owner's object; a claimed
// steering entry that now points at another BSID is not deleted; a fresh descriptor set with
// the same (persisted) claim store sees exactly our objects (agent restart).
func TestClaims(t *testing.T) {
	ctx := context.Background()
	f := newFakeSR()
	owner := freshOwner(t)
	otherOwner := owner + "x"
	iface.SetClaimStore(otherOwner, nil)
	p := sr.NewPolicy(f, owner)
	s := sr.NewSteering(f, owner)
	po := sr.NewPolicy(f, otherOwner)
	so := sr.NewSteering(f, otherOwner)

	mine := &sr.Policy{Bsid: "fd11:b::1", SidLists: []*sr.SidList{{Sids: []string{"fd11:1::1"}, Weight: 1}}}
	theirs := &sr.Policy{Bsid: "fd11:b::2", SidLists: []*sr.SidList{{Sids: []string{"fd11:1::2"}, Weight: 1}}}
	if _, err := p.Create(ctx, mine); err != nil {
		t.Fatal(err)
	}
	if _, err := po.Create(ctx, theirs); err != nil {
		t.Fatal(err)
	}
	if got := retrieveMap(t, p); len(got) != 1 || got[p.KeyOf(mine)] == nil {
		t.Fatalf("Retrieve = %v, want only ours", got)
	}
	// re-apply (resync / restart) of ours is a no-op; taking over theirs is refused
	n := len(f.CallsNamed("sr_policy_add_v2"))
	if _, err := p.Create(ctx, mine); err != nil || len(f.CallsNamed("sr_policy_add_v2")) != n {
		t.Fatalf("re-apply: %v", err)
	}
	if _, err := p.Create(ctx, theirs); !errors.Is(err, df6.ErrNotOurs) {
		t.Fatalf("take-over = %v, want ErrNotOurs", err)
	}
	if err := p.Delete(ctx, theirs, nil); err != nil || len(f.policies) != 2 {
		t.Fatalf("delete of theirs: %v (policies %d)", err, len(f.policies))
	}
	// steering: ours points at our policy; someone re-points it at theirs → never deleted
	st := &sr.Steering{TrafficType: sr.SteerType_IPV6, Prefix: "fd11:9::/64", Bsid: "fd11:b::1"}
	if _, err := s.Create(ctx, st); err != nil {
		t.Fatal(err)
	}
	if _, err := so.Create(ctx, &sr.Steering{TrafficType: sr.SteerType_IPV6, Prefix: "fd11:9::/64", Bsid: "fd11:b::2"}); !errors.Is(err, df6.ErrNotOurs) {
		t.Fatalf("foreign take-over of steering = %v", err)
	}
	for _, e := range f.steer {
		e.Bsid = df6test.IP6("fd11:b::2")
	}
	if err := s.Delete(ctx, st, nil); !errors.Is(err, df6.ErrNotOurs) {
		t.Fatalf("delete of re-pointed steering = %v, want ErrNotOurs", err)
	}
	if len(f.steer) != 1 {
		t.Fatal("re-pointed steering entry deleted")
	}
	// agent restart: fresh descriptors, same claim store
	p2 := sr.NewPolicy(f, owner)
	if got := retrieveMap(t, p2); len(got) != 1 || got[p2.KeyOf(mine)] == nil {
		t.Fatalf("after restart Retrieve = %v", got)
	}
	// a claimed object VPP lost is absent (→ recreated by the scheduler)
	f.policies = f.policies[1:]
	if got := retrieveMap(t, p2); len(got) != 0 {
		t.Fatalf("lost policy still retrieved: %v", got)
	}
	if _, err := p2.Create(ctx, mine); err != nil || len(retrieveMap(t, p2)) != 1 {
		t.Fatalf("recreate: %v", err)
	}
}

// TestStaleClaimAfterVPPRestart (fix round 2, N3, D-080): our policy's claim expires with the
// VPP instance; a foreign policy that reuses the BSID after the restart is never reported,
// taken over or deleted.
func TestStaleClaimAfterVPPRestart(t *testing.T) {
	ctx := context.Background()
	f := newFakeSR()
	owner := freshOwner(t)
	p := sr.NewPolicy(f, owner)
	mine := &sr.Policy{Bsid: "fd11:b::7", SidLists: []*sr.SidList{{Sids: []string{"fd11:1::1"}, Weight: 1}}}
	f.SetBoot(900)
	if _, err := p.Create(ctx, mine); err != nil {
		t.Fatal(err)
	}
	// VPP restarts (state lost); before we reconcile, someone else creates the same BSID.
	f.SetBoot(901)
	f.policies = nil
	iface.SetClaimStore(owner+"op", nil)
	foreign := &sr.Policy{Bsid: "fd11:b::7", SidLists: []*sr.SidList{{Sids: []string{"fd11:1::9"}, Weight: 1}}}
	if _, err := sr.NewPolicy(f, owner+"op").Create(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	if got := retrieveMap(t, p); len(got) != 0 {
		t.Fatalf("foreign policy reported as ours: %v", got)
	}
	if _, err := p.Create(ctx, mine); !errors.Is(err, df6.ErrNotOurs) {
		t.Fatalf("re-apply over the foreign policy = %v, want ErrNotOurs", err)
	}
	if err := p.Delete(ctx, mine, nil); err != nil || len(f.policies) != 1 || df6.IP6String(f.policies[0].SidLists[0].Sids[0]) != "fd11:1::9" {
		t.Fatalf("delete touched the foreign policy: %v %v", err, f.policies)
	}
}
