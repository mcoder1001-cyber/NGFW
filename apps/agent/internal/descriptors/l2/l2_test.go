package l2_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ethernet_types"
	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/scheduler"
)

const (
	owner   = "w2"
	loopKey = "interface.loopback/loop201"
	tapKey  = "tapv2.tap/w2-tap0"
	tap2Key = "tapv2.tap/w2-tap1"
)

var ctx = context.Background()

// fakeL2 extends the interface fake with bridge domains, xconnects, the L2 FIB and feature flags.
type fakeL2 struct {
	*ifacetest.VPP
	bds     map[uint32]*l2api.BridgeDomainDetails
	members map[uint32]uint32 // sw_if_index → bd_id
	shg     map[uint32]uint8
	ptype   map[uint32]l2api.L2PortType
	xc      map[uint32]uint32 // rx → tx
	fib     map[string]*l2api.L2FibTableDetails
	feat    map[uint32]l2api.L2IntfFeatFlags
	loop, tap, tap2, other uint32
}

func bdFeat(bd *l2api.BridgeDomainDetails) l2api.L2IntfFeatFlags {
	var f l2api.L2IntfFeatFlags
	if bd.Learn {
		f |= l2api.L2_INTF_FEAT_LEARN
	}
	if bd.Forward {
		f |= l2api.L2_INTF_FEAT_FWD
	}
	if bd.Flood {
		f |= l2api.L2_INTF_FEAT_FLOOD
	}
	if bd.UuFlood {
		f |= l2api.L2_INTF_FEAT_UU_FLOOD
	}
	if bd.ArpTerm {
		f |= l2api.L2_INTF_FEAT_ARP_TERM
	}
	if bd.ArpUfwd {
		f |= l2api.L2_INTF_FEAT_ARP_UFWD
	}
	return f
}

func newFake() *fakeL2 {
	f := &fakeL2{VPP: ifacetest.New(), bds: map[uint32]*l2api.BridgeDomainDetails{}, members: map[uint32]uint32{}, shg: map[uint32]uint8{},
		ptype: map[uint32]l2api.L2PortType{}, xc: map[uint32]uint32{}, fib: map[string]*l2api.L2FibTableDetails{}, feat: map[uint32]l2api.L2IntfFeatFlags{}}
	f.loop = f.Add("loop201", "Loopback", "w2:loop201")
	f.tap = f.Add("tap0", "virtio", "w2:w2-tap0")
	f.tap2 = f.Add("tap1", "virtio", "w2:w2-tap1")
	f.other = f.Add("tap2", "virtio", "w3:w3-tap0")
	// another owner's bridge domain with the other owner's member
	f.bds[3001] = &l2api.BridgeDomainDetails{BdID: 3001, Flood: true, UuFlood: true, Forward: true, Learn: true, BdTag: "w3:3001", BviSwIfIndex: ^interface_types.InterfaceIndex(0), UuFwdSwIfIndex: ^interface_types.InterfaceIndex(0)}
	f.members[f.other] = 3001
	f.feat[f.other] = bdFeat(f.bds[3001])

	f.On("bridge_domain_add_del_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeDomainAddDelV2)
		if !r.IsAdd {
			delete(f.bds, r.BdID)
			return []api.Message{&l2api.BridgeDomainAddDelV2Reply{}}, nil
		}
		if _, dup := f.bds[r.BdID]; dup {
			return []api.Message{&l2api.BridgeDomainAddDelV2Reply{Retval: -100}}, nil
		}
		f.bds[r.BdID] = &l2api.BridgeDomainDetails{BdID: r.BdID, Flood: r.Flood, UuFlood: r.UuFlood, Forward: r.Forward, Learn: r.Learn, ArpTerm: r.ArpTerm, ArpUfwd: r.ArpUfwd, MacAge: r.MacAge, BdTag: r.BdTag,
			BviSwIfIndex: ^interface_types.InterfaceIndex(0), UuFwdSwIfIndex: ^interface_types.InterfaceIndex(0)}
		return []api.Message{&l2api.BridgeDomainAddDelV2Reply{BdID: r.BdID}}, nil
	})
	f.On("bridge_domain_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeDomainDump)
		ids := make([]uint32, 0, len(f.bds))
		for id := range f.bds {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var out []api.Message
		for _, id := range ids {
			bd := *f.bds[id]
			bd.SwIfDetails = nil
			for sw, b := range f.members {
				if b == id {
					bd.SwIfDetails = append(bd.SwIfDetails, l2api.BridgeDomainSwIf{SwIfIndex: interface_types.InterfaceIndex(sw), Shg: f.shg[sw]})
				}
			}
			sort.Slice(bd.SwIfDetails, func(i, j int) bool { return bd.SwIfDetails[i].SwIfIndex < bd.SwIfDetails[j].SwIfIndex })
			bd.NSwIfs = uint32(len(bd.SwIfDetails)) //nolint:gosec // small
			if r.BdID != ^uint32(0) && r.BdID != id {
				continue
			}
			if uint32(r.SwIfIndex) != ^uint32(0) && f.members[uint32(r.SwIfIndex)] != id {
				continue
			}
			out = append(out, &bd)
		}
		return out, nil
	})
	f.On("bridge_flags", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeFlags)
		bd, ok := f.bds[r.BdID]
		if !ok {
			return []api.Message{&l2api.BridgeFlagsReply{Retval: -1}}, nil
		}
		set := func(p *bool, bit l2api.BdFlags) {
			if r.Flags&bit != 0 {
				*p = r.IsSet
			}
		}
		set(&bd.Learn, l2api.BRIDGE_API_FLAG_LEARN)
		set(&bd.Forward, l2api.BRIDGE_API_FLAG_FWD)
		set(&bd.Flood, l2api.BRIDGE_API_FLAG_FLOOD)
		set(&bd.UuFlood, l2api.BRIDGE_API_FLAG_UU_FLOOD)
		set(&bd.ArpTerm, l2api.BRIDGE_API_FLAG_ARP_TERM)
		set(&bd.ArpUfwd, l2api.BRIDGE_API_FLAG_ARP_UFWD)
		return []api.Message{&l2api.BridgeFlagsReply{}}, nil
	})
	f.On("bridge_domain_set_mac_age", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeDomainSetMacAge)
		f.bds[r.BdID].MacAge = r.MacAge
		return []api.Message{&l2api.BridgeDomainSetMacAgeReply{}}, nil
	})
	f.On("sw_interface_set_l2_bridge", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.SwInterfaceSetL2Bridge)
		sw := uint32(r.RxSwIfIndex)
		if _, ok := f.Ifs[sw]; !ok {
			return []api.Message{&l2api.SwInterfaceSetL2BridgeReply{Retval: -2}}, nil
		}
		if !r.Enable {
			delete(f.members, sw)
			delete(f.feat, sw)
			for _, bd := range f.bds {
				if uint32(bd.BviSwIfIndex) == sw {
					bd.BviSwIfIndex = ^interface_types.InterfaceIndex(0)
				}
				if uint32(bd.UuFwdSwIfIndex) == sw {
					bd.UuFwdSwIfIndex = ^interface_types.InterfaceIndex(0)
				}
			}
			return []api.Message{&l2api.SwInterfaceSetL2BridgeReply{}}, nil
		}
		bd, ok := f.bds[r.BdID]
		if !ok {
			return []api.Message{&l2api.SwInterfaceSetL2BridgeReply{Retval: -1}}, nil
		}
		f.members[sw], f.shg[sw], f.ptype[sw] = r.BdID, r.Shg, r.PortType
		f.feat[sw] = bdFeat(bd)
		switch r.PortType {
		case l2api.L2_API_PORT_TYPE_BVI:
			bd.BviSwIfIndex = r.RxSwIfIndex
		case l2api.L2_API_PORT_TYPE_UU_FWD:
			bd.UuFwdSwIfIndex = r.RxSwIfIndex
		}
		return []api.Message{&l2api.SwInterfaceSetL2BridgeReply{}}, nil
	})
	f.On("sw_interface_set_l2_xconnect", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.SwInterfaceSetL2Xconnect)
		if r.Enable {
			f.xc[uint32(r.RxSwIfIndex)] = uint32(r.TxSwIfIndex)
		} else {
			delete(f.xc, uint32(r.RxSwIfIndex))
		}
		return []api.Message{&l2api.SwInterfaceSetL2XconnectReply{}}, nil
	})
	f.On("l2_xconnect_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for rx, tx := range f.xc {
			out = append(out, &l2api.L2XconnectDetails{RxSwIfIndex: interface_types.InterfaceIndex(rx), TxSwIfIndex: interface_types.InterfaceIndex(tx)})
		}
		return out, nil
	})
	f.On("l2fib_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2fibAddDel)
		if _, ok := f.bds[r.BdID]; !ok {
			return []api.Message{&l2api.L2fibAddDelReply{Retval: -1}}, nil
		}
		k := r.Mac.String() + "@" + string(rune(r.BdID))
		if r.IsAdd {
			f.fib[k] = &l2api.L2FibTableDetails{BdID: r.BdID, Mac: r.Mac, SwIfIndex: r.SwIfIndex, StaticMac: r.StaticMac, FilterMac: r.FilterMac, BviMac: r.BviMac}
		} else {
			delete(f.fib, k)
		}
		return []api.Message{&l2api.L2fibAddDelReply{}}, nil
	})
	f.On("l2_fib_table_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2FibTableDump)
		var out []api.Message
		for _, e := range f.fib {
			if r.BdID == ^uint32(0) || r.BdID == e.BdID {
				out = append(out, e)
			}
		}
		return out, nil
	})
	f.On("l2_interface_feat_flags_get", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2InterfaceFeatFlagsGet)
		return []api.Message{&l2api.L2InterfaceFeatFlagsGetReply{Flags: f.feat[uint32(r.SwIfIndex)]}}, nil
	})
	f.On("l2_interface_feat_flags_set", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2InterfaceFeatFlagsSet)
		if r.IsSet {
			f.feat[uint32(r.SwIfIndex)] |= r.Flags
		} else {
			f.feat[uint32(r.SwIfIndex)] &^= r.Flags
		}
		return []api.Message{&l2api.L2InterfaceFeatFlagsSetReply{}}, nil
	})
	f.On("l2_interface_vlan_tag_rewrite", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2InterfaceVlanTagRewrite)
		i, ok := f.Ifs[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&l2api.L2InterfaceVlanTagRewriteReply{Retval: -2}}, nil
		}
		i.VtrOp, i.VtrPushDot1q, i.VtrTag1, i.VtrTag2 = r.VtrOp, r.PushDot1q, r.Tag1, r.Tag2
		return []api.Message{&l2api.L2InterfaceVlanTagRewriteReply{}}, nil
	})
	return f
}

func retrieve(t *testing.T, d scheduler.Descriptor) []scheduler.KV {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	return kvs
}

func mustCreate(t *testing.T, d scheduler.Descriptor, obj proto.Message) any {
	t.Helper()
	meta, err := d.Create(ctx, obj)
	if err != nil {
		t.Fatalf("%s Create(%v): %v", d.Name(), obj, err)
	}
	return meta
}

func assertOnly(t *testing.T, d scheduler.Descriptor, want map[scheduler.Key]proto.Message) {
	t.Helper()
	kvs := retrieve(t, d)
	if len(kvs) != len(want) {
		t.Fatalf("%s Retrieve = %d objects %+v, want %d", d.Name(), len(kvs), kvs, len(want))
	}
	for _, kv := range kvs {
		w, ok := want[kv.Key]
		if !ok {
			t.Fatalf("%s Retrieve unexpected key %s", d.Name(), kv.Key)
		}
		if !proto.Equal(kv.Value, w) {
			t.Fatalf("%s Retrieve %s = %v, want %v", d.Name(), kv.Key, kv.Value, w)
		}
	}
}

func TestRegister(t *testing.T) {
	r := scheduler.NewRegistry()
	l2.Register(r, newFake(), owner)
	if r.Len() != 6 {
		t.Fatalf("names = %v", r.Names())
	}
	for _, n := range r.Names() {
		if !scheduler.ValidName(n) {
			t.Errorf("invalid %q", n)
		}
	}
}

func TestBridgeDomain(t *testing.T) {
	f := newFake()
	d := l2.NewBridgeDomain(f, owner)
	desired := &l2.BridgeDomain{Id: 2001, Flood: true, UuFlood: true, Forward: true, Learn: true, MacAge: 5}
	if d.KeyOf(desired) != "l2.bridge-domain/2001" || d.Dependencies(desired) != nil {
		t.Fatal("key / deps")
	}
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("the other owner's bridge is visible: %+v", kvs)
	}
	meta := mustCreate(t, d, desired)
	req := f.CallsNamed("bridge_domain_add_del_v2")[0].(*l2api.BridgeDomainAddDelV2)
	if req.BdTag != "w2:2001" || !req.IsAdd || req.MacAge != 5 || !req.Learn || req.ArpTerm {
		t.Fatalf("add_del_v2 = %+v", req)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.bridge-domain/2001": desired})
	updated := &l2.BridgeDomain{Id: 2001, Flood: true, Forward: true, Learn: false, ArpTerm: true, MacAge: 0}
	if _, err := d.Update(ctx, desired, updated, meta); err != nil {
		t.Fatal(err)
	}
	flags := f.CallsNamed("bridge_flags")
	if len(flags) != 2 || !flags[0].(*l2api.BridgeFlags).IsSet || flags[0].(*l2api.BridgeFlags).Flags != l2api.BRIDGE_API_FLAG_ARP_TERM ||
		flags[1].(*l2api.BridgeFlags).IsSet || flags[1].(*l2api.BridgeFlags).Flags != l2api.BRIDGE_API_FLAG_LEARN|l2api.BRIDGE_API_FLAG_UU_FLOOD {
		t.Fatalf("bridge_flags = %+v %+v", flags[0], flags[1])
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.bridge-domain/2001": updated})
	if _, err := d.Update(ctx, updated, &l2.BridgeDomain{Id: 2002}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("id change: %v", err)
	}
	if _, err := d.Create(ctx, &l2.BridgeDomain{Id: 0}); err == nil {
		t.Fatal("bd 0 accepted")
	}
	if err := d.Delete(ctx, updated, meta); err != nil {
		t.Fatal(err)
	}
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
	if _, ok := f.bds[3001]; !ok {
		t.Fatal("the other owner's bridge domain was deleted")
	}
}

func TestBridgeDomainMember(t *testing.T) {
	f := newFake()
	bd := l2.NewBridgeDomain(f, owner)
	mustCreate(t, bd, &l2.BridgeDomain{Id: 2001, Flood: true, UuFlood: true, Forward: true, Learn: true})
	d := l2.NewMember(f, owner)
	m1 := &l2.BridgeDomainMember{BridgeDomain: 2001, Interface: tapKey, Shg: 1}
	bvi := &l2.BridgeDomainMember{BridgeDomain: 2001, Interface: loopKey, PortType: l2.PortType_PORT_TYPE_BVI}
	if k := d.KeyOf(m1); k != "l2.bridge-domain-member/2001/w2-tap0" {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(m1)
	if len(deps) != 2 || deps[0].Key != "l2.bridge-domain/2001" || deps[1].Key != tapKey {
		t.Fatalf("deps = %+v", deps)
	}
	meta := mustCreate(t, d, m1)
	mustCreate(t, d, bvi)
	req := f.CallsNamed("sw_interface_set_l2_bridge")[0].(*l2api.SwInterfaceSetL2Bridge)
	if uint32(req.RxSwIfIndex) != f.tap || req.BdID != 2001 || req.Shg != 1 || !req.Enable || req.PortType != l2api.L2_API_PORT_TYPE_NORMAL {
		t.Fatalf("set_l2_bridge = %+v", req)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.bridge-domain-member/2001/w2-tap0": m1, "l2.bridge-domain-member/2001/loop201": bvi})
	if _, err := d.Update(ctx, m1, &l2.BridgeDomainMember{BridgeDomain: 2001, Interface: tapKey, Shg: 2}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, m1, meta); err != nil {
		t.Fatal(err)
	}
	if last := f.CallsNamed("sw_interface_set_l2_bridge"); last[len(last)-1].(*l2api.SwInterfaceSetL2Bridge).Enable {
		t.Fatal("Delete must disable")
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.bridge-domain-member/2001/loop201": bvi})
	if _, err := d.Create(ctx, &l2.BridgeDomainMember{BridgeDomain: 2001, Interface: "tapv2.tap/w3-tap0"}); !errors.Is(err, iface.ErrNotFound) {
		t.Fatalf("other owner's interface: %v", err)
	}
}

func TestXconnect(t *testing.T) {
	f := newFake()
	d := l2.NewXconnect(f, owner)
	ab := &l2.Xconnect{Rx: tapKey, Tx: tap2Key}
	ba := &l2.Xconnect{Rx: tap2Key, Tx: tapKey}
	if d.KeyOf(ab) != "l2.xconnect/w2-tap0" || len(d.Dependencies(ab)) != 2 {
		t.Fatal("key/deps")
	}
	meta := mustCreate(t, d, ab)
	mustCreate(t, d, ba)
	f.xc[f.other] = f.tap // another owner cross-connecting into our tap: not ours (rx not owned)
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.xconnect/w2-tap0": ab, "l2.xconnect/w2-tap1": ba})
	moved := &l2.Xconnect{Rx: tapKey, Tx: loopKey}
	newMeta, err := d.Update(ctx, ab, moved, meta)
	if err != nil || newMeta.(l2.XcMeta).Tx != f.loop {
		t.Fatalf("Update: %v %+v", err, newMeta)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.xconnect/w2-tap0": moved, "l2.xconnect/w2-tap1": ba})
	if _, err := d.Update(ctx, moved, &l2.Xconnect{Rx: loopKey, Tx: tapKey}, newMeta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("rx change: %v", err)
	}
	if err := d.Delete(ctx, moved, newMeta); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.xconnect/w2-tap1": ba})
}

func TestFibEntry(t *testing.T) {
	f := newFake()
	mustCreate(t, l2.NewBridgeDomain(f, owner), &l2.BridgeDomain{Id: 2001, Flood: true, UuFlood: true, Forward: true, Learn: true})
	d := l2.NewFibEntry(f, owner)
	static := &l2.FibEntry{BridgeDomain: 2001, Mac: "02:aa:bb:cc:dd:01", Interface: tapKey, Static: true}
	filter := &l2.FibEntry{BridgeDomain: 2001, Mac: "02:aa:bb:cc:dd:02", Filter: true}
	if k := d.KeyOf(&l2.FibEntry{BridgeDomain: 2001, Mac: "02:AA:BB:CC:DD:01"}); k != "l2.fib-entry/2001/02:aa:bb:cc:dd:01" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(static); len(deps) != 2 || deps[0].Key != "l2.bridge-domain/2001" || deps[1].Key != tapKey {
		t.Fatalf("deps = %+v", deps)
	}
	if deps := d.Dependencies(filter); len(deps) != 1 {
		t.Fatalf("filter deps = %+v", deps)
	}
	meta := mustCreate(t, d, static)
	mustCreate(t, d, filter)
	// a learned entry and the other owner's static entry must be invisible
	mac, _ := ethernet_types.ParseMacAddress("02:aa:bb:cc:dd:03")
	f.fib["learned"] = &l2api.L2FibTableDetails{BdID: 2001, Mac: mac, SwIfIndex: interface_types.InterfaceIndex(f.tap)}
	f.fib["foreign"] = &l2api.L2FibTableDetails{BdID: 3001, Mac: mac, SwIfIndex: interface_types.InterfaceIndex(f.other), StaticMac: true}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.fib-entry/2001/02:aa:bb:cc:dd:01": static, "l2.fib-entry/2001/02:aa:bb:cc:dd:02": filter})
	if _, err := d.Create(ctx, &l2.FibEntry{BridgeDomain: 2001, Mac: "02:aa:bb:cc:dd:04", Interface: tapKey}); err == nil {
		t.Fatal("learned-type entry accepted")
	}
	if _, err := d.Update(ctx, static, filter, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, static, meta); err != nil {
		t.Fatal(err)
	}
	del := f.CallsNamed("l2fib_add_del")
	if r := del[len(del)-1].(*l2api.L2fibAddDel); r.IsAdd || uint32(r.SwIfIndex) != f.tap {
		t.Fatalf("delete request = %+v", r)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.fib-entry/2001/02:aa:bb:cc:dd:02": filter})
}

func TestFlags(t *testing.T) {
	f := newFake()
	mustCreate(t, l2.NewBridgeDomain(f, owner), &l2.BridgeDomain{Id: 2001, Flood: true, UuFlood: true, Forward: true, Learn: true})
	mustCreate(t, l2.NewMember(f, owner), &l2.BridgeDomainMember{BridgeDomain: 2001, Interface: tapKey})
	d := l2.NewFlags(f, owner)
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("member with bridge defaults has no flags object: %+v", kvs)
	}
	noLearn := &l2.Flags{Interface: tapKey, Learn: false, Forward: true, Flood: true, UuFlood: true}
	meta := mustCreate(t, d, noLearn)
	sets := f.CallsNamed("l2_interface_feat_flags_set")
	if len(sets) != 1 || sets[0].(*l2api.L2InterfaceFeatFlagsSet).IsSet || sets[0].(*l2api.L2InterfaceFeatFlagsSet).Flags != l2api.L2_INTF_FEAT_LEARN {
		t.Fatalf("feat_flags_set = %+v", sets)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.flags/w2-tap0": noLearn})
	if _, err := d.Create(ctx, &l2.Flags{Interface: tapKey, Learn: true, Forward: true, Flood: true, UuFlood: true}); !errors.Is(err, l2.ErrEqualsBridgeDefault) {
		t.Fatalf("defaults: %v", err)
	}
	if _, err := d.Create(ctx, &l2.Flags{Interface: loopKey}); err == nil {
		t.Fatal("non-member accepted")
	}
	arp := &l2.Flags{Interface: tapKey, Learn: true, Forward: true, Flood: true, UuFlood: true, ArpTerm: true}
	if _, err := d.Update(ctx, noLearn, arp, meta); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.flags/w2-tap0": arp})
	if err := d.Delete(ctx, arp, meta); err != nil {
		t.Fatal(err)
	}
	if kvs := retrieve(t, d); len(kvs) != 0 || f.feat[f.tap] != bdFeat(f.bds[2001]) {
		t.Fatalf("after Delete: %+v feat=%v", kvs, f.feat[f.tap])
	}
}

func TestVlanTagRewrite(t *testing.T) {
	f := newFake()
	d := l2.NewVlanTagRewrite(f, owner)
	desired := &l2.VlanTagRewrite{Interface: tapKey, Op: l2.VtrOp_VTR_OP_PUSH_1, PushDot1Q: true, Tag1: 100}
	if d.KeyOf(desired) != "l2.vlan-tag-rewrite/w2-tap0" {
		t.Fatal("key")
	}
	meta := mustCreate(t, d, desired)
	req := f.CallsNamed("l2_interface_vlan_tag_rewrite")[0].(*l2api.L2InterfaceVlanTagRewrite)
	if req.VtrOp != 1 || req.PushDot1q != 1 || req.Tag1 != 100 || req.Tag2 != 0 {
		t.Fatalf("vlan_tag_rewrite = %+v", req)
	}
	// the other owner's interface has a rewrite too: invisible
	f.Ifs[f.other].VtrOp = 3
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.vlan-tag-rewrite/w2-tap0": desired})
	translate := &l2.VlanTagRewrite{Interface: tapKey, Op: l2.VtrOp_VTR_OP_TRANSLATE_2_2, Tag1: 10, Tag2: 20}
	if _, err := d.Update(ctx, desired, translate, meta); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.vlan-tag-rewrite/w2-tap0": translate})
	if _, err := d.Create(ctx, &l2.VlanTagRewrite{Interface: loopKey}); err == nil {
		t.Fatal("op disabled accepted")
	}
	if err := d.Delete(ctx, translate, meta); err != nil {
		t.Fatal(err)
	}
	if kvs := retrieve(t, d); len(kvs) != 0 || f.Ifs[f.tap].VtrOp != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
}
