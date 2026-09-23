package lb

import (
	"errors"
	"net/netip"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lb"
	"ngfw/agent/binapi/lb_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

var tcpVIP = VIP{Prefix: "10.0.30.1/32", Protocol: ProtoTCP, Port: 80}

func TestConf(t *testing.T) {
	f := df7test.NewFake()
	f.Reply("lb_conf", &lb.LbConfReply{})
	d := NewConf(f, df7test.Owner)
	v := df7.Encode(Conf{IP4Src: "10.0.0.1", StickyBucketsPerCore: 2048})
	if d.KeyOf(v) != "lb.conf/global" || d.Dependencies(v) != nil {
		t.Fatal("key/deps")
	}
	if _, err := d.Create(t.Context(), v); err != nil {
		t.Fatal(err)
	}
	r := df7test.Last[*lb.LbConf](t, f, "lb_conf")
	if r.IP4SrcAddress != [4]uint8{10, 0, 0, 1} || r.IP6SrcAddress[0] != 0xff || r.StickyBucketsPerCore != 2048 || r.FlowTimeout != ^uint32(0) {
		t.Fatalf("%+v", r)
	}
	if _, err := d.Update(t.Context(), v, df7.Encode(Conf{FlowTimeout: 10}), nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(t.Context(), v, nil); err != nil {
		t.Fatal(err)
	}
	r = df7test.Last[*lb.LbConf](t, f, "lb_conf")
	if r.IP4SrcAddress != [4]uint8{255, 255, 255, 255} || r.StickyBucketsPerCore != DefaultStickyBuckets || r.FlowTimeout != DefaultFlowTimeout {
		t.Fatalf("delete restores defaults: %+v", r)
	}
	if _, err := d.Retrieve(t.Context()); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	for i, bad := range []Conf{{IP4Src: "::1"}, {IP6Src: "10.0.0.1"}, {StickyBucketsPerCore: 1000}} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestVIP(t *testing.T) {
	f := df7test.NewFake()
	ctx := t.Context()
	f.Reply("lb_add_del_vip_v2", &lb.LbAddDelVipV2Reply{})
	d := NewVIP(f, df7test.Owner)
	v := df7.Encode(VIPSpec{VIP: tcpVIP, Encap: EncapL3DSR, DSCP: 10})
	if d.KeyOf(v) != "lb.vip/10.0.30.1/32/tcp/80" {
		t.Fatal(d.KeyOf(v))
	}
	if deps := d.Dependencies(v); len(deps) != 1 || deps[0].Key != "lb.conf/global" || !deps[0].Optional {
		t.Fatalf("deps %v", deps)
	}
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	r := df7test.Last[*lb.LbAddDelVipV2](t, f, "lb_add_del_vip_v2")
	// IPv4 VIPs travel with ip46 prefix lengths; encap is byte-swapped for VPP's missing ntohl
	if r.Pfx.Len != 128 || r.Protocol != 6 || r.Port != 80 || r.Encap != lb_types.LbEncapType(0x02000000) || r.Dscp != 10 || r.NewFlowsTableLength != 1024 || r.IsDel {
		t.Fatalf("%+v", r)
	}
	if _, err := d.Update(ctx, v, v, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, v, nil); err != nil || !df7test.Last[*lb.LbAddDelVipV2](t, f, "lb_add_del_vip_v2").IsDel {
		t.Fatal(err)
	}

	// idempotent re-apply: VALUE_EXIST is fine when the dump shows the same VIP
	f.Reply("lb_add_del_vip_v2", &lb.LbAddDelVipV2Reply{Retval: int32(api.VALUE_EXIST)})
	f.Reply("lb_vip_dump", &lb.LbVipDetails{Vip: lb_types.LbVip{Pfx: ip_types.AddressWithPrefix{Address: df7.ToAddress(mustAddr("10.0.30.1")), Len: 128}, Port: 80},
		Encap: lb_types.LbEncapType(lb_types.LB_API_VIP_TYPE_IP4_L3DSR), Dscp: 10})
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if _, err := d.Create(ctx, df7.Encode(VIPSpec{VIP: tcpVIP, Encap: EncapGRE4})); !df7.IsVPPError(err, api.VALUE_EXIST) {
		t.Fatalf("different VIP at the same address: %v", err)
	}
	st, err := DumpVIPs(ctx, f)
	if err != nil || len(st) != 1 || st[0].Prefix != "10.0.30.1/32" || st[0].Encap != EncapL3DSR {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	for i, bad := range []VIPSpec{
		{VIP: VIP{Prefix: "10.0.30.1/32", Protocol: ProtoTCP}, Encap: EncapGRE4},
		{VIP: VIP{Prefix: "10.0.30.1/32", Protocol: ProtoAny, Port: 80}, Encap: EncapGRE4},
		{VIP: VIP{Prefix: "10.0.30.1/24", Protocol: ProtoAny}, Encap: EncapGRE4},
		{VIP: tcpVIP, Encap: "vxlan"},
		{VIP: tcpVIP, Encap: EncapNAT6},
		{VIP: tcpVIP, Encap: EncapNAT4},
		{VIP: tcpVIP, Encap: EncapGRE4, TargetPort: 1},
		{VIP: tcpVIP, Encap: EncapGRE4, DSCP: 1},
		{VIP: tcpVIP, Encap: EncapGRE4, NewFlowsTableLength: 1000},
	} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	f.Reply("lb_flush_vip", &lb.LbFlushVipReply{})
	if err := FlushVIP(ctx, f, tcpVIP); err != nil {
		t.Fatal(err)
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestASAndNat(t *testing.T) {
	f := df7test.NewFake()
	ctx := t.Context()
	f.Reply("lb_add_del_as", &lb.LbAddDelAsReply{})
	d := NewAS(f, df7test.Owner)
	v := df7.Encode(AS{VIP: tcpVIP, Address: "10.0.31.1", FlushOnDelete: true})
	if d.KeyOf(v) != "lb.as/10.0.30.1/32/tcp/80/10.0.31.1" || d.Dependencies(v)[0].Key != "lb.vip/10.0.30.1/32/tcp/80" {
		t.Fatal(d.KeyOf(v))
	}
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*lb.LbAddDelAs](t, f, "lb_add_del_as"); r.IsDel || r.IsFlush || r.Pfx.Len != 128 || r.AsAddress.Un.GetIP4() != [4]uint8{10, 0, 31, 1} {
		t.Fatalf("%+v", r)
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*lb.LbAddDelAs](t, f, "lb_add_del_as"); !r.IsDel || !r.IsFlush {
		t.Fatalf("%+v", r)
	}
	if _, err := d.Update(ctx, v, df7.Encode(AS{VIP: tcpVIP, Address: "10.0.31.1"}), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Update(ctx, v, df7.Encode(AS{VIP: tcpVIP, Address: "10.0.31.2"}), nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	f.Reply("lb_add_del_as", &lb.LbAddDelAsReply{Retval: int32(api.VALUE_EXIST)})
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}

	// VPP stacks the in2out feature on every enable: model it (D-076)
	stack := map[uint32]int{}
	f.On("lb_add_del_intf_nat4", func(m api.Message) ([]api.Message, error) {
		r := m.(*lb.LbAddDelIntfNat4)
		if r.IsAdd {
			stack[uint32(r.SwIfIndex)]++
		} else if stack[uint32(r.SwIfIndex)] > 0 {
			stack[uint32(r.SwIfIndex)]--
		}
		return []api.Message{&lb.LbAddDelIntfNat4Reply{}}, nil
	})
	f.Reply("lb_add_del_intf_nat6", &lb.LbAddDelIntfNat6Reply{})
	nd := NewIntfNat(f, df7test.Owner)
	n4 := df7.Encode(IntfNat{Interface: "loop0", Family: FamilyIP4})
	if nd.KeyOf(n4) != "lb.intf-nat/loop0/ip4" || nd.Dependencies(n4)[0].Key != "interface/loop0" {
		t.Fatal("key/deps")
	}
	meta, err := nd.Create(ctx, n4)
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*lb.LbAddDelIntfNat4](t, f, "lb_add_del_intf_nat4"); !r.IsAdd || r.SwIfIndex != 1 {
		t.Fatalf("%+v", r)
	}
	for i := 0; i < 3; i++ { // write-only resyncs
		if _, err := nd.Create(ctx, n4); err != nil {
			t.Fatal(err)
		}
	}
	if stack[1] != 1 {
		t.Fatalf("feature stacked %d times", stack[1])
	}
	if err := nd.Delete(ctx, n4, meta); err != nil || stack[1] != 0 {
		t.Fatal(err, stack)
	}
	// after a VPP restart the feature is gone: re-enabled once, a stale delete sends nothing
	if _, err := nd.Create(ctx, n4); err != nil {
		t.Fatal(err)
	}
	f.Reboot()
	stack[1] = 0
	f.Reset()
	if err := nd.Delete(ctx, n4, nil); err != nil || len(f.CallsNamed("lb_add_del_intf_nat4")) != 0 {
		t.Fatal(err, f.CallsNamed("lb_add_del_intf_nat4"))
	}
	n6 := df7.Encode(IntfNat{Interface: "loop1", Family: FamilyIP6})
	if _, err := nd.Create(ctx, n6); err != nil || len(f.CallsNamed("lb_add_del_intf_nat6")) != 1 {
		t.Fatal(err)
	}
	if _, err := nd.Update(ctx, n4, n6, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if _, err := nd.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	if _, err := nd.Create(ctx, df7.Encode(IntfNat{Interface: "loop9", Family: FamilyIP4})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatal(err)
	}
	r := scheduler.NewRegistry()
	Register(r, f, df7test.Owner)
	if r.Len() != 3 {
		t.Fatal("non-owners register no globals (D-071):", r.Names())
	}
	RegisterGlobals(r, f, df7test.Owner)
	if r.Len() != 4 {
		t.Fatal(r.Names())
	}
}
