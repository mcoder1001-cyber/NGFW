package lb

import (
	"errors"
	"math/bits"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lb"
	"ngfw/agent/binapi/lb_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
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

// lbFake models the lb plugin's VIP and AS tables. fixed selects a VPP that converts the enums
// with ntohl (V20 fixed); otherwise VPP 26.06 reads the big-endian wire value as host order.
// force, when set, makes VPP create that VIP type whatever was requested.
type lbFake struct {
	*df7test.Fake
	fixed bool
	force lb_types.LbVipType
	vips  map[string]lb_types.LbVipType
	ases  map[string]bool
}

func newLBFake(fixed bool) *lbFake {
	enumNative.Store(false)
	df7.SetBootStore(df7test.Owner, nil)
	f := &lbFake{Fake: df7test.NewFake(), fixed: fixed, vips: map[string]lb_types.LbVipType{}, ases: map[string]bool{}}
	vipType := map[uint32]lb_types.LbVipType{0: lb_types.LB_API_VIP_TYPE_IP4_GRE4, 1: lb_types.LB_API_VIP_TYPE_IP4_GRE6,
		2: lb_types.LB_API_VIP_TYPE_IP4_L3DSR, 3: lb_types.LB_API_VIP_TYPE_IP4_NAT4}
	vkey := func(p ip_types.AddressWithPrefix, port uint16) string {
		return df7.FromAddress(p.Address).String() + "/" + strconv.Itoa(int(port))
	}
	f.On("lb_add_del_vip_v2", func(m api.Message) ([]api.Message, error) {
		r := m.(*lb.LbAddDelVipV2)
		reply := func(rv api.VPPApiError) []api.Message {
			return []api.Message{&lb.LbAddDelVipV2Reply{Retval: int32(rv)}}
		}
		k := vkey(r.Pfx, r.Port)
		if r.IsDel {
			if _, ok := f.vips[k]; !ok {
				return reply(api.NO_SUCH_ENTRY), nil
			}
			delete(f.vips, k)
			return reply(0), nil
		}
		enc := uint32(r.Encap)
		if !f.fixed {
			enc = bits.ReverseBytes32(enc) // VPP 26.06: no ntohl
		}
		vt, ok := vipType[enc]
		if !ok {
			return reply(api.INVALID_ADDRESS_FAMILY), nil
		}
		if _, ok := f.vips[k]; ok {
			return reply(api.VALUE_EXIST), nil
		}
		if f.force != 0 {
			vt = f.force
		}
		f.vips[k] = vt
		return reply(0), nil
	})
	f.On("lb_vip_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for k, vt := range f.vips {
			a, port, _ := strings.Cut(k, "/")
			p, _ := strconv.ParseUint(port, 10, 16)
			pfx := df7.ToAddressWithPrefix(netip.MustParsePrefix(a + "/32"))
			pfx.Len += 96
			out = append(out, &lb.LbVipDetails{Vip: lb_types.LbVip{Pfx: pfx, Port: uint16(p)}, Encap: lb_types.LbEncapType(vt)})
		}
		return out, nil
	})
	f.On("lb_add_del_as", func(m api.Message) ([]api.Message, error) {
		r := m.(*lb.LbAddDelAs)
		reply := func(rv api.VPPApiError) []api.Message { return []api.Message{&lb.LbAddDelAsReply{Retval: int32(rv)}} }
		k := vkey(r.Pfx, r.Port) + "/" + df7.FromAddress(r.AsAddress).String()
		if _, ok := f.vips[vkey(r.Pfx, r.Port)]; !ok {
			return reply(api.NO_SUCH_ENTRY), nil
		}
		switch {
		case r.IsDel && !f.ases[k]:
			return reply(api.NO_SUCH_ENTRY), nil
		case r.IsDel:
			delete(f.ases, k)
		case f.ases[k]:
			return reply(api.VALUE_EXIST), nil
		default:
			f.ases[k] = true
		}
		return reply(0), nil
	})
	return f
}

func TestVIP(t *testing.T) {
	f := newLBFake(false)
	ctx := t.Context()
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
	r := df7test.Last[*lb.LbAddDelVipV2](t, f.Fake, "lb_add_del_vip_v2")
	// IPv4 VIPs travel with ip46 prefix lengths; encap is byte-swapped for VPP's missing ntohl
	if r.Pfx.Len != 128 || r.Protocol != 6 || r.Port != 80 || r.Encap != lb_types.LbEncapType(0x02000000) || r.Dscp != 10 || r.NewFlowsTableLength != 1024 || r.IsDel {
		t.Fatalf("%+v", r)
	}
	if len(f.CallsNamed("lb_add_del_vip_v2")) != 1 || enumNative.Load() {
		t.Fatal("VPP 26.06: first try, no flip")
	}
	if _, err := d.Update(ctx, v, v, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	// re-apply of our own recorded VIP: VALUE_EXIST is success
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	st, err := DumpVIPs(ctx, f)
	if err != nil || len(st) != 1 || st[0].Prefix != "10.0.30.1/32" || st[0].Encap != EncapL3DSR {
		t.Fatalf("%+v %v", st, err)
	}
	if err := d.Delete(ctx, v, nil); err != nil || !df7test.Last[*lb.LbAddDelVipV2](t, f.Fake, "lb_add_del_vip_v2").IsDel || len(f.vips) != 0 {
		t.Fatal(err)
	}
	// review M4: an already-gone VIP is deleted already
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	// review M4: an existing VIP without our record (another owner's, or of an earlier VPP
	// instance) is never adopted, and never deleted
	f.vips["10.0.30.1/80"] = lb_types.LB_API_VIP_TYPE_IP4_L3DSR
	if _, err := d.Create(ctx, v); !errors.Is(err, dfkit.ErrNotOurs) {
		t.Fatalf("adopted: %v", err)
	}
	f.Reset()
	if err := d.Delete(ctx, v, nil); err != nil || len(f.CallsNamed("lb_add_del_vip_v2")) != 0 || len(f.vips) != 1 {
		t.Fatal("a VIP that is not ours must not be deleted", err)
	}
	delete(f.vips, "10.0.30.1/80")
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	f.RestartSamePID() // the VIP of an earlier VPP instance record never matches
	f.Reset()
	if err := d.Delete(ctx, v, nil); err != nil || len(f.CallsNamed("lb_add_del_vip_v2")) != 0 {
		t.Fatal(err)
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

// Review M2: a VPP whose lb API converts the enums (V20 fixed) is detected at runtime and the
// byte order adapts; a VPP that makes another VIP type in both orders fails loudly.
func TestVIPEnumOrder(t *testing.T) {
	ctx := t.Context()
	f := newLBFake(true)
	d := NewVIP(f, df7test.Owner)
	v := df7.Encode(VIPSpec{VIP: tcpVIP, Encap: EncapL3DSR, DSCP: 10})
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if !enumNative.Load() || f.vips["10.0.30.1/80"] != lb_types.LB_API_VIP_TYPE_IP4_L3DSR {
		t.Fatalf("fixed VPP: native order must be detected: %v %v", enumNative.Load(), f.vips)
	}
	if r := df7test.Last[*lb.LbAddDelVipV2](t, f.Fake, "lb_add_del_vip_v2"); r.Encap != lb_types.LB_API_ENCAP_TYPE_L3DSR {
		t.Fatalf("retried with %#x", r.Encap)
	}
	// later adds use the detected order directly
	f.Reset()
	v2 := df7.Encode(VIPSpec{VIP: VIP{Prefix: "10.0.30.2/32", Protocol: ProtoTCP, Port: 80}, Encap: EncapGRE6})
	if _, err := d.Create(ctx, v2); err != nil || len(f.CallsNamed("lb_add_del_vip_v2")) != 1 {
		t.Fatal(err, len(f.CallsNamed("lb_add_del_vip_v2")))
	}
	// a wrong type in both orders: the wrong VIP is removed and the error is loud
	g := newLBFake(false)
	g.force = lb_types.LB_API_VIP_TYPE_IP4_GRE4
	gd := NewVIP(g, df7test.Owner)
	if _, err := gd.Create(ctx, v); !errors.Is(err, ErrEnumOrder) {
		t.Fatalf("want ErrEnumOrder: %v", err)
	}
	if len(g.vips) != 0 {
		t.Fatalf("the wrong VIP must be removed: %v", g.vips)
	}
	if ok, _ := gd.Recorded(ctx, string(KeyVIP(tcpVIP))); ok {
		t.Fatal("a failed VIP must not be recorded")
	}
	enumNative.Store(false)
}

func TestASAndNat(t *testing.T) {
	f := newLBFake(false)
	ctx := t.Context()
	f.vips["10.0.30.1/80"] = lb_types.LB_API_VIP_TYPE_IP4_GRE4
	d := NewAS(f, df7test.Owner)
	v := df7.Encode(AS{VIP: tcpVIP, Address: "10.0.31.1", FlushOnDelete: true})
	if d.KeyOf(v) != "lb.as/10.0.30.1/32/tcp/80/10.0.31.1" || d.Dependencies(v)[0].Key != "lb.vip/10.0.30.1/32/tcp/80" {
		t.Fatal(d.KeyOf(v))
	}
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*lb.LbAddDelAs](t, f.Fake, "lb_add_del_as"); r.IsDel || r.IsFlush || r.Pfx.Len != 128 || r.AsAddress.Un.GetIP4() != [4]uint8{10, 0, 31, 1} {
		t.Fatalf("%+v", r)
	}
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatalf("idempotent for our recorded AS: %v", err)
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*lb.LbAddDelAs](t, f.Fake, "lb_add_del_as"); !r.IsDel || !r.IsFlush || len(f.ases) != 0 {
		t.Fatalf("%+v", r)
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal("an already-gone AS is deleted already", err)
	}
	if _, err := d.Update(ctx, v, df7.Encode(AS{VIP: tcpVIP, Address: "10.0.31.1"}), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Update(ctx, v, df7.Encode(AS{VIP: tcpVIP, Address: "10.0.31.2"}), nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	// review M4: an existing AS without our record is never adopted nor deleted
	f.ases["10.0.30.1/80/10.0.31.1"] = true
	if _, err := d.Create(ctx, v); !errors.Is(err, dfkit.ErrNotOurs) {
		t.Fatalf("adopted: %v", err)
	}
	if err := d.Delete(ctx, v, nil); err != nil || !f.ases["10.0.30.1/80/10.0.31.1"] {
		t.Fatal("a foreign AS must survive", err)
	}
	delete(f.ases, "10.0.30.1/80/10.0.31.1")
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	delete(f.vips, "10.0.30.1/80") // the VIP (and its ASes) went away underneath
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal("NO_SUCH_ENTRY is success", err)
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
	if r := df7test.Last[*lb.LbAddDelIntfNat4](t, f.Fake, "lb_add_del_intf_nat4"); !r.IsAdd || r.SwIfIndex != 1 {
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
