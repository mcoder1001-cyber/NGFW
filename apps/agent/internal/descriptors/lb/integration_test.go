package lb

import (
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

// Host test: VIPs and ASes in this slot's 10.<slot>.30.0/24 / 10.<slot>.31.0/24, the NAT
// feature on this slot's loopback (loop<slot>30). lb.conf is VPP-wide with no getter: the test
// sets this slot's GRE source addresses and restores VPP's start-up values (255.255.255.255,
// ffff:…, 1024 buckets, 40 s) in Cleanup — nothing else on the host uses the lb plugin.
func TestLBOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	cd, vd, ad, nd := NewConf(h.C, h.Owner), NewVIP(h.C, h.Owner), NewAS(h.C, h.Owner), NewIntfNat(h.C, h.Owner)
	ifA, _ := h.Loopback(30, true, true)

	conf := df7.Encode(Conf{IP4Src: h.Addr(30, 254), IP6Src: "2001:db8:10::fe", StickyBucketsPerCore: 1024, FlowTimeout: 40})
	h.Apply(cd, df7test.Desired(cd, conf))
	t.Cleanup(func() {
		if err := cd.Delete(h.Ctx, conf, nil); err != nil {
			t.Errorf("restore lb conf: %v", err)
		} else {
			t.Log("lb conf restored to VPP start-up values")
		}
	})

	tcp := VIP{Prefix: h.Addr(30, 1) + "/32", Protocol: ProtoTCP, Port: 80}
	all := VIP{Prefix: h.Addr(30, 2) + "/32", Protocol: ProtoAny}
	natVIP := VIP{Prefix: h.Addr(30, 3) + "/32", Protocol: ProtoUDP, Port: 8080}
	vips := []scheduler.KV{
		df7test.Desired(vd, df7.Encode(VIPSpec{VIP: tcp, Encap: EncapGRE4, NewFlowsTableLength: 1024})),
		df7test.Desired(vd, df7.Encode(VIPSpec{VIP: all, Encap: EncapL3DSR, DSCP: 10})),
		df7test.Desired(vd, df7.Encode(VIPSpec{VIP: natVIP, Encap: EncapNAT4, SrvType: SrvClusterIP, TargetPort: 80})),
	}
	ases := []scheduler.KV{
		df7test.Desired(ad, df7.Encode(AS{VIP: tcp, Address: h.Addr(31, 1)})),
		df7test.Desired(ad, df7.Encode(AS{VIP: tcp, Address: h.Addr(31, 2), FlushOnDelete: true})),
		df7test.Desired(ad, df7.Encode(AS{VIP: all, Address: h.Addr(31, 3)})),
	}
	var cv []scheduler.KV
	t.Cleanup(func() { h.DeleteAll(vd, cv) })
	for _, v := range vips {
		cv = append(cv, h.Apply(vd, v)...)
	}
	ca := h.Apply(ad, ases...)
	t.Cleanup(func() { h.DeleteAll(ad, ca) })
	nat := h.Apply(nd, df7test.Desired(nd, df7.Encode(IntfNat{Interface: ifA, Family: FamilyIP4})))
	t.Cleanup(func() { h.DeleteAll(nd, nat) })

	// write-only re-application (resync) is idempotent
	h.Apply(vd, vips...)
	h.Apply(ad, ases...)
	h.Apply(nd, df7test.Desired(nd, df7.Encode(IntfNat{Interface: ifA, Family: FamilyIP4})))
	for _, d := range []scheduler.Descriptor{cd, vd, ad, nd} {
		if _, err := d.Retrieve(h.Ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
			t.Fatalf("%s Retrieve: %v", d.Name(), err)
		}
	}
	state, err := DumpVIPs(h.Ctx, h.C)
	h.Must("lb_vip_dump", err)
	for _, s := range state {
		t.Logf("lb_vip_dump: %+v", s)
	}
	h.Hold("lb vips")
	h.Must("flush", FlushVIP(h.Ctx, h.C, tcp))

	h.DeleteAll(nd, nat)
	h.DeleteAll(ad, ca)
	h.DeleteAll(vd, cv)
	nat, ca, cv = nil, nil, nil
}
