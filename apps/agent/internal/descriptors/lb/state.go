package lb

// Read-only lb state and the lb_flush_vip action helper (F-lb gaps: DumpASes is a missing helper, FlushVIP a proven
// defect — TestFlushVIPIPv4Layout, TestFlushVIPRefusesWithoutServerInUse).

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lb"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
)

// ASFlagUsed is LB_AS_FLAGS_USED (lb.h): the application server is in use; without it the AS is "removed" and waits
// for VPP's lb garbage collection.
const ASFlagUsed = 0x1

// ASState is one row of lb_as_dump: an application server of the VIP (prefix, port). VPP 26.06 reports the VIP
// protocol as 0 (htonl into a u8), so a tcp and a udp VIP with the same prefix and port cannot be told apart.
type ASState struct {
	Prefix  string // VIP prefix (canonical CIDR)
	Port    uint16 // VIP port (0 = all ports)
	Address string // application server
	InUse   bool   // LB_AS_FLAGS_USED
	// InUseSince is the VPP clock (seconds) of the last use / removal (lb_as_t.last_used).
	InUseSince uint32
}

// DumpASes runs lb_as_dump for every VIP (a zero prefix dumps all).
func DumpASes(ctx context.Context, c vpp.Client) ([]ASState, error) {
	stream, err := lb.NewServiceClient(c).LbAsDump(ctx, &lb.LbAsDump{})
	if err != nil {
		return nil, fmt.Errorf("lb_as_dump: %w", df7.PluginError("lb", err))
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("lb_as_dump: %w", err)
	}
	out := make([]ASState, 0, len(dets))
	for _, det := range dets {
		pfx := det.Vip.Pfx
		if pfx.Address.Af == ip_types.ADDRESS_IP4 && pfx.Len >= 96 {
			pfx.Len -= 96 // ip46 length of an IPv4 VIP
		}
		out = append(out, ASState{
			Prefix: df7.FromAddressWithPrefix(pfx).Masked().String(), Port: det.Vip.Port,
			Address: df7.FromAddress(det.AppSrv).String(), InUse: det.Flags&ASFlagUsed != 0, InUseSince: det.InUseSince,
		})
	}
	return out, nil
}

// ErrFlushNotSafe is returned by FlushVIP when it cannot prove that VPP's lookup of the VIP will succeed.
var ErrFlushNotSafe = errors.New("lb: flush refused")

// flushPrefix encodes a VIP prefix the way lb_flush_vip reads it: the handler memcpy's the 16 bytes of
// pfx.address.un.ip6 into an ip46_address_t whatever the address family, so an IPv4 VIP must travel in the ip46
// layout (12 zero bytes, then the IPv4 address) with its ip46 length — the plain ip_types encoding (the IPv4 address
// in the first 4 bytes) never matches the VIP, and VPP then flushes an uninitialised VIP index.
func flushPrefix(v VIP) ip_types.AddressWithPrefix {
	p := netip.MustParsePrefix(v.Prefix)
	out := vipPrefix(v)
	if p.Addr().Is4() {
		var b [16]byte
		a4 := p.Masked().Addr().As4()
		copy(b[12:], a4[:])
		out.Address.Un.SetIP6(ip_types.IP6Address(b))
	}
	return out
}

// FlushVIP is the lb_flush_vip action helper: flush the VIP's sticky flow table so established flows are re-hashed.
// VPP 26.06's handler ignores the lookup result and flushes the VIP index it found — an uninitialised one when there
// is none (possibly ~0 = every VIP of every owner) — so the flush is sent only when lb_as_dump shows an application
// server of this prefix and port in use (a VIP that exists and is in use; the protocol is not reported, see ASState).
// The caller proves ownership (the agent's boot record of the VIP).
func FlushVIP(ctx context.Context, c vpp.Client, v VIP) error {
	if _, err := v.validate(); err != nil {
		return err
	}
	ases, err := DumpASes(ctx, c)
	if err != nil {
		return err
	}
	want := netip.MustParsePrefix(v.Prefix).Masked().String()
	inUse := false
	for _, a := range ases {
		if a.Prefix == want && a.Port == v.Port && a.InUse {
			inUse = true
			break
		}
	}
	if !inUse {
		return fmt.Errorf("%w: VIP %s/%s/%d has no application server in use in VPP (nothing to flush, and VPP 26.06 would flush an uninitialised VIP index if the VIP is gone)",
			ErrFlushNotSafe, v.Prefix, v.Protocol, v.Port)
	}
	_, err = lb.NewServiceClient(c).LbFlushVip(ctx, &lb.LbFlushVip{Pfx: flushPrefix(v), Protocol: protocols[v.Protocol], Port: v.Port})
	return df7.PluginError("lb", err)
}
