package subsystems

import (
	"context"
	"errors"
	"io"
	"net/netip"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	tapapi "ngfw/agent/binapi/tapv2"
	"ngfw/agent/internal/descriptors/df6"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers/pppoe"
)

// promoteCarrierIPv4 runs only after the complete kernel/lease/NCP/forwarding
// admission, under the transaction fence. Native interface NAT picks the first
// address; the transit must follow the authenticated negotiated address.
// Exact subsets are recoverable after cancellation or agent death. Recovery only
// adds these owned addresses; a foreign prefix prevents every mutation. There is
// no detached cleanup context or extension of the original poll deadline.
func (rt *PppoeRuntime) promoteCarrierIPv4(ctx context.Context, s pppoe.Session, m desc.Mirror, index uint32) error {
	if s.Carrier == nil || s.Carrier.Validate() != nil || s.Carrier.Owner != rt.owner || s.Carrier.Logical != s.Iface || m.Interface != s.Iface {
		return errors.New("invalid primary carrier ownership")
	}
	if m.LocalIPv4 == "" {
		return nil
	}
	local, err := netip.ParsePrefix(m.LocalIPv4)
	transit := netip.MustParsePrefix(s.Carrier.VPP4())
	if err != nil || !local.Addr().Is4() || local.Bits() != 32 || local.Addr().IsUnspecified() || local.Addr().IsMulticast() || transit.Contains(local.Addr()) {
		return errors.New("invalid primary carrier address")
	}
	read := func() ([]netip.Prefix, error) {
		if err := rt.verifyPrimaryCarrierIdentity(ctx, s, index); err != nil {
			return nil, err
		}
		stream, err := ip.NewServiceClient(rt.vpp).IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(index)})
		if err != nil {
			return nil, err
		}
		var prefixes []netip.Prefix
		seen := map[netip.Prefix]bool{}
		for {
			row, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			prefix, err := netip.ParsePrefix(row.Prefix.String())
			if err != nil || prefix != local && prefix != transit || seen[prefix] {
				return nil, errors.New("foreign carrier IPv4 address")
			}
			seen[prefix] = true
			prefixes = append(prefixes, prefix)
		}
		return prefixes, ctx.Err()
	}
	prefixes, err := read()
	if err != nil {
		return err
	}
	if len(prefixes) == 2 && prefixes[0] == local {
		return nil
	}
	// A negotiated-only prefix is already primary. All other admitted subsets
	// must first become empty so native address callbacks cannot pick transit.
	type edit struct {
		prefix netip.Prefix
		add    bool
	}
	edits := []edit{}
	if len(prefixes) == 1 && prefixes[0] == local {
		edits = append(edits, edit{transit, true})
	} else {
		for _, prefix := range prefixes {
			if prefix == local {
				edits = append(edits, edit{local, false})
			}
		}
		for _, prefix := range prefixes {
			if prefix == transit {
				edits = append(edits, edit{transit, false})
			}
		}
		edits = append(edits, edit{local, true}, edit{transit, true})
	}
	for _, change := range edits {
		actual, err := read()
		if err != nil {
			return err
		}
		if !sameCarrierPrefixOrder(actual, prefixes) {
			return errors.New("carrier addresses changed during primary repair")
		}
		parsed, err := ip_types.ParseAddressWithPrefix(change.prefix.String())
		if err != nil {
			return err
		}
		_, err = interfaces.NewServiceClient(rt.vpp).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(index), IsAdd: change.add, Prefix: parsed})
		if err != nil {
			return err
		}
		if change.add {
			prefixes = append(prefixes, change.prefix)
		} else {
			next := []netip.Prefix{}
			for _, prefix := range prefixes {
				if prefix != change.prefix {
					next = append(next, prefix)
				}
			}
			prefixes = next
		}
	}
	actual, err := read()
	if err != nil {
		return err
	}
	if !sameCarrierPrefixOrder(actual, []netip.Prefix{local, transit}) {
		return errors.New("carrier primary address readback differs")
	}
	return nil
}

func sameCarrierPrefixOrder(a, b []netip.Prefix) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Recheck the live owner tag/index and exact native TAP identity before every
// read/write. An ordinary interface or reused index never receives this repair.
func (rt *PppoeRuntime) verifyPrimaryCarrierIdentity(ctx context.Context, s pppoe.Session, index uint32) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	ifs, err := df6.DumpInterfaces(ctx, rt.vpp, rt.owner)
	if err != nil {
		return err
	}
	got, ok := ifs.IndexByTag(s.Iface)
	if !ok || got != index {
		return errors.New("primary carrier interface identity changed")
	}
	stream, err := tapapi.NewServiceClient(rt.vpp).SwInterfaceTapV2Dump(ctx, &tapapi.SwInterfaceTapV2Dump{SwIfIndex: interface_types.InterfaceIndex(index)})
	if err != nil {
		return err
	}
	_, id := s.Carrier.TapIDs()
	count := 0
	for {
		row, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if uint32(row.SwIfIndex) != index || row.ID != id || row.HostNamespace != s.Carrier.Token() || row.HostIfName != s.Carrier.TransitHost() || row.HostMtuSize != s.MTU || row.HostBridge != "" {
			return errors.New("primary carrier native TAP differs")
		}
		count++
	}
	if count != 1 {
		return errors.New("primary carrier native TAP is absent or duplicated")
	}
	return ctx.Err()
}
