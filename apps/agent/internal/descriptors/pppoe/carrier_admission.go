package pppoe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/interface_types"
	ipapi "ngfw/agent/binapi/ip"
	l2api "ngfw/agent/binapi/l2"
	pppoeapi "ngfw/agent/binapi/pppoe"
	tapapi "ngfw/agent/binapi/tapv2"
	iface "ngfw/agent/internal/descriptors/interface"
	lcpdesc "ngfw/agent/internal/descriptors/lcp"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/vpp"
)

// AdmitCarrier is read-only live admission, called immediately before namespace
// creation under the scheduler transaction lock. Unknown readback fails closed.
func AdmitCarrier(ctx context.Context, c vpp.Client, owner string, spec ren.CarrierSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	table, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return err
	}
	parent, err := table.IndexByName(spec.Parent)
	if err != nil {
		return err
	}
	if _, err := table.IndexByName(spec.Logical); err == nil {
		return errors.New("PPPoE logical interface already exists outside its carrier")
	} else if !errors.Is(err, iface.ErrNotFound) {
		return err
	}
	taps, err := tapapi.NewServiceClient(c).SwInterfaceTapV2Dump(ctx, &tapapi.SwInterfaceTapV2Dump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return err
	}
	rawID, transitID := spec.TapIDs()
	for {
		row, e := taps.Recv()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return e
		}
		if row.ID == rawID || row.ID == transitID || row.HostNamespace == spec.Token() {
			return errors.New("PPPoE TAP identity is already present outside its namespace lease")
		}
	}
	unnumbered, err := ipapi.NewServiceClient(c).IPUnnumberedDump(ctx, &ipapi.IPUnnumberedDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return err
	}
	for {
		row, e := unnumbered.Recv()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return e
		}
		if uint32(row.SwIfIndex) == parent || uint32(row.IPSwIfIndex) == parent {
			return errors.New("PPPoE raw parent participates in unnumbered addressing")
		}
	}
	bonds, err := bondapi.NewServiceClient(c).SwBondInterfaceDump(ctx, &bondapi.SwBondInterfaceDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return err
	}
	for {
		bond, e := bonds.Recv()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return e
		}
		if uint32(bond.SwIfIndex) == parent {
			return errors.New("PPPoE raw parent is a bond")
		}
		members, e := bondapi.NewServiceClient(c).SwMemberInterfaceDump(ctx, &bondapi.SwMemberInterfaceDump{SwIfIndex: bond.SwIfIndex})
		if e != nil {
			return e
		}
		for {
			member, e := members.Recv()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return e
			}
			if uint32(member.SwIfIndex) == parent {
				return errors.New("PPPoE raw parent belongs to a bond")
			}
		}
	}
	for _, index := range table.Indexes() {
		details, _ := table.Details(index)
		if index != parent && uint32(details.SupSwIfIndex) == parent {
			return errors.New("PPPoE raw parent has live subinterfaces")
		}
	}
	var reserved []netip.Prefix
	for _, index := range table.Indexes() {
		for _, ipv6 := range []bool{false, true} {
			stream, err := ipapi.NewServiceClient(c).IPAddressDump(ctx, &ipapi.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(index), IsIPv6: ipv6})
			if err != nil {
				return err
			}
			for {
				row, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return err
				}
				if index == parent {
					return errors.New("PPPoE raw parent still has an IP address")
				}
				prefix, err := netip.ParsePrefix(row.Prefix.String())
				if err != nil {
					return fmt.Errorf("invalid VPP address during carrier admission: %w", err)
				}
				reserved = append(reserved, prefix)
			}
		}
	}
	if err := ren.CheckCarrierPrefixes([]ren.CarrierSpec{spec}, reserved); err != nil {
		return err
	}
	pairs, err := lcpdesc.Pairs(ctx, c)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if uint32(pair.PhySwIfIndex) == parent {
			return errors.New("PPPoE raw parent already belongs to a linux-cp pair")
		}
	}
	xcs, err := l2api.NewServiceClient(c).L2XconnectDump(ctx, &l2api.L2XconnectDump{})
	if err != nil {
		return err
	}
	for {
		row, err := xcs.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if uint32(row.RxSwIfIndex) == parent || uint32(row.TxSwIfIndex) == parent {
			return errors.New("PPPoE raw parent already belongs to a cross-connect")
		}
	}
	bds, err := l2api.NewServiceClient(c).BridgeDomainDump(ctx, &l2api.BridgeDomainDump{BdID: ^uint32(0), SwIfIndex: interface_types.InterfaceIndex(parent)})
	if err != nil {
		return err
	}
	for {
		row, err := bds.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		for _, member := range row.SwIfDetails {
			if uint32(member.SwIfIndex) == parent {
				return errors.New("PPPoE raw parent already belongs to a bridge")
			}
		}
	}
	sessions, err := pppoeapi.NewServiceClient(c).PppoeSessionDump(ctx, &pppoeapi.PppoeSessionDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return err
	}
	for {
		row, err := sessions.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if uint32(row.EncapIfIndex) == parent {
			return errors.New("PPPoE raw parent already carries a server session")
		}
	}
	active, err := cpProbe(ctx, c, interface_types.InterfaceIndex(parent), false)
	if err != nil {
		return err
	}
	if active {
		return errors.New("PPPoE raw parent has an existing control-plane attachment")
	}
	return nil
}
