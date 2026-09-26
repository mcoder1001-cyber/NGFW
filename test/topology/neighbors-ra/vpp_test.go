package neighborsra

// Direct VPP access of the test (never of the product API): binary API calls for the injected learned entries and the
// simulated loss, vppctl for the evidence the acceptance asks for. Every call names one of this slot's prefixed
// loopbacks; nothing is ever sent with sw_if_index ~0 or 0.

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"os/exec"
	"strings"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"

	arpapi "ngfw/agent/binapi/arp"
	"ngfw/agent/binapi/ethernet_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/ip_types"
)

const apiSocket = "/run/vpp/api.sock"

const noIndex = ^uint32(0)

func connectVPP(t *testing.T) vppapi.Connection {
	t.Helper()
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatalf("govpp connect: %v", err)
	}
	t.Cleanup(conn.Disconnect)
	return conn
}

func ctx10(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// ifIndex returns the sw_if_index of a prefixed interface (0 = absent).
func ifIndex(t *testing.T, conn vppapi.Connection, name string) uint32 {
	t.Helper()
	stream, err := interfaces.NewServiceClient(conn).SwInterfaceDump(ctx10(t), &interfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(noIndex), NameFilterValid: true, NameFilter: name,
	})
	if err != nil {
		t.Fatal(err)
	}
	var idx uint32
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimRight(d.InterfaceName, "\x00") == name {
			idx = uint32(d.SwIfIndex)
		}
	}
	return idx
}

func addr(s string) ip_types.Address {
	a := netip.MustParseAddr(s)
	if a.Is4() {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(a.As4())}
	}
	return ip_types.Address{Af: ip_types.ADDRESS_IP6, Un: ip_types.AddressUnionIP6(a.As16())}
}

func mac(s string) ethernet_types.MacAddress {
	m, err := ethernet_types.ParseMacAddress(s)
	if err != nil {
		panic(err)
	}
	return m
}

// neighbor adds (or deletes) a neighbour on one interface with the given flags; flags NONE is a learned (dynamic)
// entry, exactly what the data plane creates after ARP/ND — the test injects them instead of sending packets.
func neighbor(t *testing.T, conn vppapi.Connection, idx uint32, ip, hw string, flags ip_neighbor.IPNeighborFlags, add bool) {
	t.Helper()
	if idx == 0 || idx == noIndex {
		t.Fatalf("refusing sw_if_index %d", idx)
	}
	_, err := ip_neighbor.NewServiceClient(conn).IPNeighborAddDel(ctx10(t), &ip_neighbor.IPNeighborAddDel{IsAdd: add, Neighbor: ip_neighbor.IPNeighbor{
		SwIfIndex: interface_types.InterfaceIndex(idx), Flags: flags, MacAddress: mac(hw), IPAddress: addr(ip),
	}})
	if err != nil {
		t.Fatalf("ip_neighbor_add_del %s on %d (add=%v): %v", ip, idx, add, err)
	}
}

// raPrefix withdraws (is_no) an advertised prefix.
func raPrefixDelete(t *testing.T, conn vppapi.Connection, idx uint32, prefix string) {
	t.Helper()
	p := netip.MustParsePrefix(prefix)
	_, err := ip6_nd.NewServiceClient(conn).SwInterfaceIP6ndRaPrefix(ctx10(t), &ip6_nd.SwInterfaceIP6ndRaPrefix{
		SwIfIndex: interface_types.InterfaceIndex(idx), IsNo: true,
		Prefix: ip_types.Prefix{Address: addr(p.Addr().String()), Len: uint8(p.Bits())}, //nolint:gosec // G115: ≤ 128
	})
	if err != nil {
		t.Fatalf("sw_interface_ip6nd_ra_prefix is_no %s: %v", prefix, err)
	}
}

// raReset returns the RA settings of idx to VPP's fresh-interface state (what DF-2's Delete sends).
func raReset(t *testing.T, conn vppapi.Connection, idx uint32) {
	t.Helper()
	svc := ip6_nd.NewServiceClient(conn)
	for _, req := range []*ip6_nd.SwInterfaceIP6ndRaConfig{
		{SwIfIndex: interface_types.InterfaceIndex(idx), IsNo: true, Managed: 1, Other: 1, LlOption: 1, SendUnicast: 1, Cease: 1, DefaultRouter: 1, Lifetime: 1, MaxInterval: 1, MinInterval: 1, InitialCount: 1, InitialInterval: 1},
		{SwIfIndex: interface_types.InterfaceIndex(idx), Suppress: 1},
	} {
		if _, err := svc.SwInterfaceIP6ndRaConfig(ctx10(t), req); err != nil {
			t.Fatalf("sw_interface_ip6nd_ra_config reset: %v", err)
		}
	}
}

func proxyArpInterface(t *testing.T, conn vppapi.Connection, idx uint32, enable bool) {
	t.Helper()
	if _, err := arpapi.NewServiceClient(conn).ProxyArpIntfcEnableDisable(ctx10(t), &arpapi.ProxyArpIntfcEnableDisable{SwIfIndex: interface_types.InterfaceIndex(idx), Enable: enable}); err != nil {
		t.Fatalf("proxy_arp_intfc_enable_disable: %v", err)
	}
}

func proxyArpRange(t *testing.T, conn vppapi.Connection, table uint32, lo, hi string, add bool) {
	t.Helper()
	l, h := netip.MustParseAddr(lo).As4(), netip.MustParseAddr(hi).As4()
	if _, err := arpapi.NewServiceClient(conn).ProxyArpAddDel(ctx10(t), &arpapi.ProxyArpAddDel{IsAdd: add, Proxy: arpapi.ProxyArp{TableID: table, Low: l, Hi: h}}); err != nil {
		t.Fatalf("proxy_arp_add_del: %v", err)
	}
}

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vppctl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("vppctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// linesWith returns the lines of out that contain every needle (the evidence excerpt).
func linesWith(out string, needles ...string) []string {
	var res []string
	for _, l := range strings.Split(out, "\n") {
		ok := true
		for _, n := range needles {
			if !strings.Contains(l, n) {
				ok = false
				break
			}
		}
		if ok && strings.TrimSpace(l) != "" {
			res = append(res, strings.TrimRight(l, " "))
		}
	}
	return res
}
