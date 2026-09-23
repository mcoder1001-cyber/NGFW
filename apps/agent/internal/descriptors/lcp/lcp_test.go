package lcp

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

type model struct {
	ns    string
	pairs map[uint32]lcp.LcpItfPairDetails
	next  uint32
}

func newFake() (*dfkittest.FakeVPP, *model) {
	f := dfkittest.NewFake(
		dfkittest.Iface{Index: 7, Name: "loop501", Tag: "w5:loop501"},
		dfkittest.Iface{Index: 9, Name: "ens192"},
		dfkittest.Iface{Index: 8, Name: "loop601", Tag: "w6:loop601"},
	)
	m := &model{pairs: map[uint32]lcp.LcpItfPairDetails{}, next: 20}
	f.On("lcp_default_ns_set", func(msg api.Message) ([]api.Message, error) {
		m.ns = msg.(*lcp.LcpDefaultNsSet).Netns
		return []api.Message{&lcp.LcpDefaultNsSetReply{}}, nil
	})
	f.On("lcp_default_ns_get", func(api.Message) ([]api.Message, error) {
		return []api.Message{&lcp.LcpDefaultNsGetReply{Netns: m.ns}}, nil
	})
	f.On("lcp_itf_pair_add_del_v3", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*lcp.LcpItfPairAddDelV3)
		idx := uint32(r.SwIfIndex)
		_, exists := m.pairs[idx]
		switch {
		case r.IsAdd && exists:
			return []api.Message{&lcp.LcpItfPairAddDelV3Reply{Retval: int32(api.VALUE_EXIST)}}, nil
		case !r.IsAdd && !exists:
			return []api.Message{&lcp.LcpItfPairAddDelV3Reply{Retval: int32(api.INVALID_SW_IF_INDEX)}}, nil
		case r.IsAdd:
			ns := r.Netns
			if ns == "" {
				ns = m.ns // VPP stores the effective namespace
			}
			m.next++
			m.pairs[idx] = lcp.LcpItfPairDetails{PhySwIfIndex: r.SwIfIndex, HostSwIfIndex: interface_types.InterfaceIndex(m.next),
				VifIndex: 100 + m.next, HostIfName: r.HostIfName, HostIfType: r.HostIfType, Netns: ns}
			return []api.Message{&lcp.LcpItfPairAddDelV3Reply{HostSwIfIndex: interface_types.InterfaceIndex(m.next), VifIndex: 100 + m.next}}, nil
		default:
			delete(m.pairs, idx)
			return []api.Message{&lcp.LcpItfPairAddDelV3Reply{}}, nil
		}
	})
	f.On("lcp_itf_pair_get", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, p := range m.pairs {
			p := p
			out = append(out, &p)
		}
		return append(out, &lcp.LcpItfPairGetReply{Cursor: ^uint32(0)}), nil
	})
	f.On("lcp_itf_pair_replace_begin", dfkittest.Retval(&lcp.LcpItfPairReplaceBeginReply{}))
	f.On("lcp_itf_pair_replace_end", dfkittest.Retval(&lcp.LcpItfPairReplaceEndReply{}))
	return f, m
}

func TestItfPair(t *testing.T) {
	f, m := newFake()
	ctx := context.Background()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	if d.KeyOf(v) != "lcp.itf-pair/loop501" {
		t.Fatal(d.KeyOf(v))
	}
	deps := d.Dependencies(v)
	if len(deps) != 2 || deps[0].Key != "interface/loop501" || deps[0].Optional || deps[1].Key != KeyDefaultNetns || !deps[1].Optional {
		t.Fatalf("deps %+v", deps)
	}
	meta, err := d.Create(ctx, v)
	if err != nil || meta != (PairMeta{PhySwIfIndex: 7, HostSwIfIndex: 21, VifIndex: 121}) {
		t.Fatalf("create %v %v", meta, err)
	}
	req := f.CallsNamed("lcp_itf_pair_add_del_v3")[0].(*lcp.LcpItfPairAddDelV3)
	if !req.IsAdd || req.SwIfIndex != 7 || req.HostIfName != "w5-lcp0" || req.HostIfType != lcp.LCP_API_ITF_HOST_TAP || req.Netns != "ns-w5" {
		t.Fatalf("request %+v", req)
	}
	// a pair on an unclaimed untagged NIC and one on another owner's interface are not ours
	m.pairs[9] = lcp.LcpItfPairDetails{PhySwIfIndex: 9, HostIfName: "e0"}
	m.pairs[8] = lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0"}
	got := dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	if got.Meta != meta || len(dfkittest.MustRetrieve(t, d)) != 1 {
		t.Fatalf("retrieve %v", dfkittest.MustRetrieve(t, d))
	}
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	if again, err := d.Create(ctx, v); err != nil || again != meta {
		t.Fatalf("re-apply %v %v", again, err)
	}
	if _, err := d.Create(ctx, ItfPair{Interface: "loop501", HostIfName: "w5-other", HostIfType: "tun"}.Proto()); !dfkit.IsVPPError(err, api.VALUE_EXIST) {
		t.Fatalf("conflicting pair: %v", err)
	}
	if _, err := d.Update(ctx, v, v, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	for range 2 {
		if err := d.Delete(ctx, v, meta); err != nil {
			t.Fatal(err)
		}
	}
	dfkittest.AssertAbsent(t, d, d.KeyOf(v))
	// an untagged NIC (P12's DPDK ports) is paired through a claim (D-071) and reported only while claimed
	nv := ItfPair{Interface: "ens192", HostIfName: "w5-e0", HostIfType: "tap"}.Proto()
	nm, err := d.Create(ctx, nv)
	if err != nil {
		t.Fatalf("untagged NIC: %v", err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, nv))
	if kvs := dfkittest.MustRetrieve(t, NewItfPair(f, "w7")); len(kvs) != 0 {
		t.Fatalf("unclaimed pair reported to another owner: %v", kvs)
	}
	if err := d.Delete(ctx, nv, nm); err != nil || dfkit.Claims("w5").Claimed("ens192", NameItfPair) {
		t.Fatalf("untagged delete: %v", err)
	}
	if _, err := d.Create(ctx, ItfPair{Interface: "loop601", HostIfName: "w5-x", HostIfType: "tap"}.Proto()); !errors.Is(err, dfkit.ErrNotOwned) {
		t.Fatalf("pairing another owner's interface must be refused: %v", err)
	}
	for _, bad := range []ItfPair{
		{Interface: "loop501", HostIfName: "w5-a-very-long-name", HostIfType: "tap"},
		{Interface: "loop501", HostIfName: "w5 x", HostIfType: "tap"},
		{Interface: "loop501", HostIfName: "w5-x", HostIfType: "tapx"},
		{Interface: "loop501", HostIfName: "w5-x", HostIfType: "tap", Netns: "../etc"},
		{Interface: "loop501", HostIfName: "..", HostIfType: "tap"},
	} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestDefaultNetns(t *testing.T) {
	f, m := newFake()
	ctx := context.Background()
	d := NewDefaultNetns(f, WithGlobals(dfkit.GlobalsOwner(true)))
	if kvs := dfkittest.MustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("unset reported %v", kvs)
	}
	v := DefaultNetns{Netns: "dataplane"}.Proto()
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	v2 := DefaultNetns{Netns: "other"}.Proto()
	if _, err := d.Update(ctx, v, v2, nil); err != nil || m.ns != "other" {
		t.Fatal(err)
	}
	// a pair created with netns "" lands in the default namespace and is reported with it
	p := NewItfPair(f, "w5")
	if _, err := p.Create(ctx, ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()); err != nil {
		t.Fatal(err)
	}
	kvs := dfkittest.MustRetrieve(t, p)
	var got ItfPair
	if err := dfkit.Decode(kvs[0].Value, &got); err != nil || got.Netns != "other" {
		t.Fatalf("pair netns %+v %v", got, err)
	}
	if err := d.Delete(ctx, v2, nil); err != nil || m.ns != "" {
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, d, KeyDefaultNetns)
	if _, err := d.Create(ctx, DefaultNetns{Netns: "a/b"}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
}

func TestPluginNotLoaded(t *testing.T) {
	f, _ := newFake()
	unknown := &adapter.UnknownMsgError{MsgName: "lcp_default_ns_get", MsgCrc: "x"}
	f.Fail("lcp_default_ns_get", unknown)
	f.Fail("lcp_itf_pair_get", unknown)
	if _, err := NewDefaultNetns(f, WithGlobals(dfkit.GlobalsOwner(true))).Retrieve(context.Background()); !errors.Is(err, dfkit.ErrPluginNotLoaded) {
		t.Fatalf("netns: %v", err)
	}
	if _, err := NewItfPair(f, "w5").Retrieve(context.Background()); !errors.Is(err, dfkit.ErrPluginNotLoaded) {
		t.Fatalf("pairs: %v", err)
	}
}

func TestReplaceAndRegister(t *testing.T) {
	f, _ := newFake()
	if err := ReplaceBegin(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceEnd(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	r := scheduler.NewRegistry()
	Register(r, f, "w5")
	if r.Len() != 2 {
		t.Fatal(r.Names())
	}
}
