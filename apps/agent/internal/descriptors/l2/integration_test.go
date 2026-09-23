package l2_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func keyOf(kv scheduler.KV) string { return string(kv.Key) }

func has(kvs []scheduler.KV, key string) bool {
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return true
		}
	}
	return false
}

type hostFixture struct {
	t     *testing.T
	ctx   context.Context
	owner string
}

// create applies desired, checks Retrieve == desired and registers Delete + "gone" check in Cleanup.
func (h *hostFixture) create(d scheduler.Descriptor, desired proto.Message) any {
	h.t.Helper()
	key := string(d.KeyOf(desired))
	meta, err := d.Create(h.ctx, desired)
	if err != nil {
		h.t.Fatalf("%s Create %s: %v", d.Name(), key, err)
	}
	h.t.Cleanup(func() {
		if err := d.Delete(context.Background(), desired, meta); err != nil {
			h.t.Errorf("%s Delete %s: %v", d.Name(), key, err)
			return
		}
		kvs, err := d.Retrieve(context.Background())
		if err != nil {
			h.t.Errorf("%s Retrieve after Delete: %v", d.Name(), err)
		}
		if has(kvs, key) {
			h.t.Errorf("%s: %s still retrieved after Delete", d.Name(), key)
		}
	})
	kvs, err := d.Retrieve(h.ctx)
	if err != nil {
		h.t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	got := ifacetest.Find(h.t, kvs, key, keyOf)
	if !proto.Equal(got.Value, desired) {
		h.t.Fatalf("%s Retrieve %s = %v, want %v", d.Name(), key, got.Value, desired)
	}
	h.t.Logf("%s: Retrieve == desired: %s %v", d.Name(), key, got.Value)
	return meta
}

func (h *hostFixture) tap(td *tapv2.TapDescriptor, i int) string {
	name := vpptest.Name(h.t, "tap"+itoa(i))
	desired := &tapv2.Tap{Name: name, Id: vpptest.LoopbackInstance(h.t, i), HostIfName: name, RxRingSize: 256, TxRingSize: 256}
	h.create(td, desired)
	return string(td.KeyOf(desired))
}

func itoa(i int) string { return string(rune('0'+i/10)) + string(rune('0'+i%10)) }

func TestL2OnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	h := &hostFixture{t: t, ctx: context.Background(), owner: owner}
	_, loopKey := ifacetest.Loopback(t, c, owner, 10)
	td := tapv2.New(c, owner)
	tap10, tap11, tap12, tap13 := h.tap(td, 10), h.tap(td, 11), h.tap(td, 12), h.tap(td, 13)
	bdID := vpptest.TableBase(t) + 10

	// bridge-domain: create, update in place (flags + mac age), Retrieve tracks it
	bd := l2.NewBridgeDomain(c, owner)
	bdObj := &l2.BridgeDomain{Id: bdID, Flood: true, UuFlood: true, Forward: true, Learn: true, MacAge: 3}
	bdMeta := h.create(bd, bdObj)
	bdUpd := &l2.BridgeDomain{Id: bdID, Flood: true, UuFlood: true, Forward: true, Learn: true, ArpTerm: true}
	if _, err := bd.Update(h.ctx, bdObj, bdUpd, bdMeta); err != nil {
		t.Fatalf("bridge-domain Update: %v", err)
	}
	kvs, _ := bd.Retrieve(h.ctx)
	if got := ifacetest.Find(t, kvs, string(bd.KeyOf(bdUpd)), keyOf); !proto.Equal(got.Value, bdUpd) {
		t.Fatalf("after Update: %v", got.Value)
	}

	// members: two taps (one with a split-horizon group) and the loopback as BVI
	md := l2.NewMember(c, owner)
	h.create(md, &l2.BridgeDomainMember{BridgeDomain: bdID, Interface: tap10, Shg: 1})
	h.create(md, &l2.BridgeDomainMember{BridgeDomain: bdID, Interface: tap11})
	h.create(md, &l2.BridgeDomainMember{BridgeDomain: bdID, Interface: loopKey, PortType: l2.PortType_PORT_TYPE_BVI})

	// static + filter L2 FIB entries
	fd := l2.NewFibEntry(c, owner)
	h.create(fd, &l2.FibEntry{BridgeDomain: bdID, Mac: "02:02:00:00:0a:01", Interface: tap10, Static: true})
	h.create(fd, &l2.FibEntry{BridgeDomain: bdID, Mac: "02:02:00:00:0a:02", Filter: true})

	// per-interface feature override: no learning on tap10
	h.create(l2.NewFlags(c, owner), &l2.Flags{Interface: tap10, Learn: false, Forward: true, Flood: true, UuFlood: true, ArpTerm: true})

	// cross-connect tap12 <-> tap13 (two objects)
	xd := l2.NewXconnect(c, owner)
	h.create(xd, &l2.Xconnect{Rx: tap12, Tx: tap13})
	h.create(xd, &l2.Xconnect{Rx: tap13, Tx: tap12})

	// QinQ: a dot1ad sub-interface on tap11 pushing one tag
	sd := iface.NewSubinterface(c, owner)
	sub := &iface.Subinterface{Parent: tap11, SubId: 100, OuterVlan: 100, InnerVlan: 200, Dot1Ad: true, ExactMatch: true}
	h.create(sd, sub)
	h.create(md, &l2.BridgeDomainMember{BridgeDomain: bdID, Interface: string(sd.KeyOf(sub))})
	h.create(l2.NewVlanTagRewrite(c, owner), &l2.VlanTagRewrite{Interface: string(sd.KeyOf(sub)), Op: l2.VtrOp_VTR_OP_POP_2})

	t.Logf("bridge-domain %d with %s, %s, %s(BVI) and %s.100 configured; vppctl show bridge-domain %d detail", bdID, tap10, tap11, loopKey, tap11, bdID)
	ifacetest.Hold(t)
}
