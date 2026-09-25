package lb

// F-lb gap tests (envelope: descriptors/lb/** gap-only — a proven defect or a missing helper, each named by a test).
// Every test here fails on the DF-7 base: no ownership declarations (TD-11b), claim after the enable, the flush
// helper's IPv4 encoding and unguarded flush, no lb_as_dump / garbage-collection helpers.

import (
	"errors"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lb"
	"ngfw/agent/binapi/lb_types"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// persistedClaims is an in-memory claim store that says it survives an agent restart (the product installs
// subsystems.IfaceClaims).
type persistedClaims struct{ iface.ClaimStore }

func (persistedClaims) Persistent() bool { return true }

// TD-11b: every lb descriptor declares how it records ownership, and the records must be persisted.
func TestOwnershipDeclared(t *testing.T) {
	f := df7test.NewFake()
	ds := []scheduler.Descriptor{NewConf(f, df7test.Owner), NewVIP(f, df7test.Owner), NewAS(f, df7test.Owner), NewIntfNat(f, df7test.Owner)}
	for _, d := range ds {
		if err := persist.Declared(d); err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}
	}
	df7.SetBootStore(df7test.Owner, nil) // in memory
	iface.SetClaimStore(df7test.Owner, nil)
	t.Cleanup(func() { df7.SetBootStore(df7test.Owner, nil); iface.SetClaimStore(df7test.Owner, nil) })
	for _, d := range ds[1:] {
		if err := persist.Check(d); !errors.Is(err, persist.ErrVolatile) {
			t.Fatalf("%s with in-memory stores: %v", d.Name(), err)
		}
	}
	bs, err := dfkit.NewFileBootStore(filepath.Join(t.TempDir(), "boot.json"))
	if err != nil {
		t.Fatal(err)
	}
	df7.SetBootStore(df7test.Owner, bs)
	if err := persist.Check(ds[3]); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("intf-nat also needs a persisted claim store: %v", err)
	}
	iface.SetClaimStore(df7test.Owner, persistedClaims{iface.NewMemoryClaimStore()})
	for _, d := range ds {
		if err := persist.Check(d); err != nil {
			t.Fatalf("%s with persisted stores: %v", d.Name(), err)
		}
	}
}

// failClaims is a claim store that cannot record (a full disk, a read-only state dir).
type failClaims struct{}

func (failClaims) Claim(string, string) error   { return errors.New("claim store down") }
func (failClaims) Release(string, string) error { return nil }
func (failClaims) Claimed(string, string) bool  { return false }

// TD-11b claim first: on an untagged interface (eth0) the claim is recorded before lb_add_del_intf_nat4, so a claim
// that cannot be recorded leaves nothing in VPP (DF-7 enabled first and claimed after).
func TestIntfNatClaimsFirst(t *testing.T) {
	f := newLBFake(false)
	f.Reply("lb_add_del_intf_nat4", &lb.LbAddDelIntfNat4Reply{})
	iface.SetClaimStore(df7test.Owner, failClaims{})
	t.Cleanup(func() { iface.SetClaimStore(df7test.Owner, nil) })
	d := NewIntfNat(f, df7test.Owner)
	n := df7.Encode(IntfNat{Interface: "eth0", Family: FamilyIP4})
	if _, err := d.Create(t.Context(), n); err == nil {
		t.Fatal("Create succeeded without its claim")
	}
	if c := len(f.CallsNamed("lb_add_del_intf_nat4")); c != 0 {
		t.Fatalf("the feature was enabled before the claim (%d calls)", c)
	}
	// a failed enable releases the claim this Create made
	iface.SetClaimStore(df7test.Owner, nil)
	f.Fail("lb_add_del_intf_nat4", errors.New("vpp says no"))
	if _, err := d.Create(t.Context(), n); err == nil {
		t.Fatal("Create succeeded")
	}
	if df7test.Claimed(t.Context(), f, df7test.Owner, "eth0", string(KeyIntfNat("eth0", FamilyIP4))) {
		t.Fatal("the claim of a failed Create was kept")
	}
	// and a successful one keeps it
	f.Reply("lb_add_del_intf_nat4", &lb.LbAddDelIntfNat4Reply{})
	if _, err := d.Create(t.Context(), n); err != nil {
		t.Fatal(err)
	}
	if !df7test.Claimed(t.Context(), f, df7test.Owner, "eth0", string(KeyIntfNat("eth0", FamilyIP4))) {
		t.Fatal("claim missing after a successful Create")
	}
}

// asRow builds an lb_as_details row the way VPP 26.06 sends it (IPv4 VIPs with ip46 lengths, protocol 0).
func asRow(vip, port, addr string, used bool) *lb.LbAsDetails {
	p := netip.MustParsePrefix(vip)
	pfx := df7.ToAddressWithPrefix(p)
	if p.Addr().Is4() {
		pfx.Len += 96
	}
	n, _ := strconv.Atoi(port)
	var flags uint8
	if used {
		flags = ASFlagUsed
	}
	return &lb.LbAsDetails{Vip: lb_types.LbVip{Pfx: pfx, Port: uint16(n)}, AppSrv: df7.ToAddress(netip.MustParseAddr(addr)), Flags: flags, InUseSince: 77}
}

func TestDumpASes(t *testing.T) {
	f := df7test.NewFake()
	f.Reply("lb_as_dump", asRow("10.0.30.1/32", "80", "10.0.31.1", true), asRow("2001:db8::1/128", "0", "2001:db8:1::1", false))
	got, err := DumpASes(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	want := []ASState{
		{Prefix: "10.0.30.1/32", Port: 80, Address: "10.0.31.1", InUse: true, InUseSince: 77},
		{Prefix: "2001:db8::1/128", Port: 0, Address: "2001:db8:1::1", InUse: false, InUseSince: 77},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("%+v", got)
	}
	if r := df7test.Last[*lb.LbAsDump](t, f, "lb_as_dump"); r.Pfx != (ip_types.AddressWithPrefix{}) {
		t.Fatalf("dump all needs a zero prefix: %+v", r)
	}
}

// Defect (VPP 26.06 api.c vl_api_lb_flush_vip_t_handler): the handler copies pfx.address.un.ip6 whatever the family,
// so an IPv4 VIP must travel in the ip46 layout — DF-7's FlushVIP sent the IPv4 address in the first 4 bytes, the
// lookup never matched, and VPP flushed an uninitialised VIP index.
func TestFlushVIPIPv4Layout(t *testing.T) {
	f := df7test.NewFake()
	f.Reply("lb_as_dump", asRow("10.0.30.1/32", "80", "10.0.31.1", true))
	f.Reply("lb_flush_vip", &lb.LbFlushVipReply{})
	if err := FlushVIP(t.Context(), f, tcpVIP); err != nil {
		t.Fatal(err)
	}
	r := df7test.Last[*lb.LbFlushVip](t, f, "lb_flush_vip")
	raw := r.Pfx.Address.Un.GetIP6()
	if want := [16]byte{12: 10, 13: 0, 14: 30, 15: 1}; [16]byte(raw) != want || r.Pfx.Len != 128 || r.Protocol != 6 || r.Port != 80 {
		t.Fatalf("flush request %+v (un bytes % x)", r, raw)
	}
	// IPv6 VIPs are unchanged
	f.Reset()
	v6 := VIP{Prefix: "2001:db8::1/128", Protocol: ProtoUDP, Port: 53}
	f.Reply("lb_as_dump", asRow("2001:db8::1/128", "53", "2001:db8:1::1", true))
	if err := FlushVIP(t.Context(), f, v6); err != nil {
		t.Fatal(err)
	}
	r = df7test.Last[*lb.LbFlushVip](t, f, "lb_flush_vip")
	if [16]byte(r.Pfx.Address.Un.GetIP6()) != netip.MustParseAddr("2001:db8::1").As16() || r.Pfx.Len != 128 {
		t.Fatalf("%+v", r)
	}
}

// Defect: a flush for a VIP VPP does not find flushes an uninitialised index (possibly every VIP): refused unless an
// application server of the VIP is in use.
func TestFlushVIPRefusesWithoutServerInUse(t *testing.T) {
	for name, rows := range map[string][]api.Message{
		"no VIP":           nil,
		"removed only":     {asRow("10.0.30.1/32", "80", "10.0.31.1", false)},
		"other port":       {asRow("10.0.30.1/32", "81", "10.0.31.1", true)},
		"other prefix":     {asRow("10.0.30.2/32", "80", "10.0.31.1", true)},
		"other family bit": {asRow("2001:db8::1/128", "80", "10.0.31.1", true)},
	} {
		f := df7test.NewFake()
		f.Reply("lb_as_dump", rows...)
		f.Reply("lb_flush_vip", &lb.LbFlushVipReply{})
		if err := FlushVIP(t.Context(), f, tcpVIP); !errors.Is(err, ErrFlushNotSafe) {
			t.Fatalf("%s: %v", name, err)
		}
		if n := len(f.CallsNamed("lb_flush_vip")); n != 0 {
			t.Fatalf("%s: flush sent", name)
		}
	}
}

func vipRow(prefix string, port uint16, vt lb_types.LbVipType, targetPort uint16) *lb.LbVipDetails {
	p := netip.MustParsePrefix(prefix)
	pfx := df7.ToAddressWithPrefix(p)
	if p.Addr().Is4() {
		pfx.Len += 96
	}
	return &lb.LbVipDetails{Vip: lb_types.LbVip{Pfx: pfx, Port: port}, Encap: lb_types.LbEncapType(vt), TargetPort: targetPort}
}

// D-090 (2): the garbage collection is one constant cli_inband command; its expected answer is the sentinel VIP's
// failed lookup.
func TestGarbageCollect(t *testing.T) {
	f := df7test.NewFake()
	f.Reply("lb_vip_dump", vipRow("10.0.30.1/32", 80, lb_types.LB_API_VIP_TYPE_IP4_GRE4, 0))
	f.Reply("lb_as_dump", asRow("10.0.30.1/32", "80", "10.0.31.1", false))
	f.Reply("cli_inband", &vlib.CliInbandReply{Retval: -1, Reply: "lb_vip_find_index error -6\n"})
	if err := GarbageCollect(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	calls := f.CallsNamed("cli_inband")
	if len(calls) != 1 || calls[0].(*vlib.CliInband).Cmd != GCCommand || GCCommand != "lb vip 0.0.0.0/32 del" {
		t.Fatalf("%+v", calls)
	}
	if !GCSentinelRange.Contains(netip.MustParseAddr("0.0.0.0")) {
		t.Fatal("the sentinel VIP must be inside the reserved range")
	}
	// the lb plugin is not loaded / the CLI failed otherwise: loud
	f.Reply("cli_inband", &vlib.CliInbandReply{Retval: -1, Reply: "unknown input `lb vip 0.0.0.0/32 del'"})
	if err := GarbageCollect(t.Context(), f); err == nil || !strings.Contains(err.Error(), "unknown input") {
		t.Fatal(err)
	}
	// success = the sentinel existed and was deleted: impossible for VRX, loud
	f.Reply("cli_inband", &vlib.CliInbandReply{})
	if err := GarbageCollect(t.Context(), f); err == nil {
		t.Fatal("a deleted sentinel VIP must be reported")
	}
	// nothing in VPP: nothing sent
	g := df7test.NewFake()
	g.Reply("lb_vip_dump")
	if err := GarbageCollect(t.Context(), g); err != nil || len(g.CallsNamed("cli_inband")) != 0 {
		t.Fatal(err, g.CallsNamed("cli_inband"))
	}
}

// V20 follow-up: a NAT VIP and its "removed" predecessor share the SNAT mapping of (AS, target port); VPP's
// collection would free the live mapping and later pool_put a NULL one — skipped, nothing sent.
func TestGarbageCollectRefusesSharedSNATMapping(t *testing.T) {
	nat := lb_types.LB_API_VIP_TYPE_IP4_NAT4
	f := df7test.NewFake()
	f.Reply("lb_vip_dump", vipRow("10.0.30.3/32", 8080, nat, 20480), vipRow("10.0.30.3/32", 8080, nat, 20480))
	f.Reply("lb_as_dump", asRow("10.0.30.3/32", "8080", "10.0.31.9", false), asRow("10.0.30.3/32", "8080", "10.0.31.9", true))
	f.Reply("cli_inband", &vlib.CliInbandReply{Retval: -1, Reply: "lb_vip_find_index error -6"})
	if err := GarbageCollect(t.Context(), f); !errors.Is(err, ErrGCUnsafe) {
		t.Fatal(err)
	}
	if len(f.CallsNamed("cli_inband")) != 0 {
		t.Fatal("GC sent despite the shared SNAT mapping")
	}
	// GRE VIPs with the same server and port are no NAT mappings: safe
	vips := []VIPState{{Prefix: "10.0.30.1/32", Port: 80, Encap: EncapGRE4}, {Prefix: "10.0.30.1/32", Port: 80, Encap: EncapGRE4}}
	ases := []ASState{{Prefix: "10.0.30.1/32", Port: 80, Address: "10.0.31.1"}, {Prefix: "10.0.30.1/32", Port: 80, Address: "10.0.31.1", InUse: true}}
	if ok, why := GCSafe(vips, ases); !ok {
		t.Fatal(why)
	}
	// two different NAT VIPs sharing (AS, target port): unsafe too
	vips = []VIPState{{Prefix: "10.0.30.3/32", Port: 80, Encap: EncapNAT4, TargetPort: 8080}, {Prefix: "10.0.30.4/32", Port: 80, Encap: EncapNAT4, TargetPort: 8080}}
	ases = []ASState{{Prefix: "10.0.30.3/32", Port: 80, Address: "10.0.31.1", InUse: true}, {Prefix: "10.0.30.4/32", Port: 80, Address: "10.0.31.1", InUse: true}}
	if ok, _ := GCSafe(vips, ases); ok {
		t.Fatal("two NAT VIPs sharing an SNAT key must be unsafe")
	}
}
