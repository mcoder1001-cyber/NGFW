package nat44ed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// NameWANPool identifies exclusive static WAN pools.
const NameWANPool = "nat44-ed.static-address.wan"

// WANPoolSpec binds a static member address to its FIB, unlike AnyVRF interface pools.
type WANPoolSpec struct {
	Interface string `json:"interface"`
	Prefix    string `json:"prefix"`
	VRF       uint32 `json:"vrf"`
}

func (s WANPoolSpec) address() AddressPoolSpec {
	prefix, _ := netip.ParsePrefix(s.Prefix)
	return AddressPoolSpec{First: prefix.Addr().String(), Last: prefix.Addr().String(), VRF: s.VRF}
}

// WANPoolID is the exact native address/FIB identity used by both descriptor families.
func WANPoolID(s WANPoolSpec) string { return PoolID(s.address()) }

// WANPool creates exclusive, claim-backed pools after the address and output feature exist.
// Native output allocation then matches both source FIB and selected egress interface.
func (p *Plugin) WANPool(name, outputName string) *natcommon.Descriptor[WANPoolSpec] {
	return natcommon.New(natcommon.Ops[WANPoolSpec]{
		Name: name, Claims: p.claims(), Exclusive: true,
		ID: WANPoolID,
		Deps: func(s WANPoolSpec) []scheduler.Dependency {
			return append(natcommon.WithVRF(enableDep(), s.VRF), scheduler.Dependency{Key: core.InterfaceAddrKey(s.Interface, s.Prefix)}, scheduler.Dependency{Key: scheduler.Join(outputName, s.Interface)})
		},
		Create: func(ctx context.Context, s WANPoolSpec) (any, error) {
			if _, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface); err != nil {
				return nil, err
			}
			return nil, p.addDelAddressRange(ctx, s.address(), true)
		},
		Delete: func(ctx context.Context, s WANPoolSpec, _ any) error {
			if !p.claims().Claimed(string(scheduler.Join(name, WANPoolID(s)))) {
				return fmt.Errorf("%s: pool is not claimed by this instance", name)
			}
			return p.addDelAddressRange(ctx, s.address(), false)
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[WANPoolSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			tracked, err := p.interfacePoolAddresses(ctx)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44AddressDump(ctx, &nat44_ed.Nat44AddressDump{})
			if err != nil {
				return nil, err
			}
			var pools []*nat44_ed.Nat44AddressDetails
			for {
				row, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, err
				}
				if row.VrfID != AnyVRF && row.Flags&nat_types.NAT_IS_TWICE_NAT == 0 && !tracked[netip.AddrFrom4(row.IPAddress)] {
					pools = append(pools, row)
				}
			}
			var out []natcommon.Item[WANPoolSpec]
			for _, iface := range ifaces.All() {
				if owned, _ := p.scope.InterfaceOwnership(iface); !owned {
					continue
				}
				logical := p.scope.LogicalNameOf(ifaces, iface.SwIfIndex)
				if logical == "" {
					continue
				}
				addresses, err := ip.NewServiceClient(p.client).IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex)})
				if err != nil {
					return nil, err
				}
				for {
					row, err := addresses.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						return nil, err
					}
					address, err := netip.ParseAddr(natcommon.AddrString(ip_types.Prefix(row.Prefix).Address))
					if err != nil || !address.Is4() {
						continue
					}
					for _, pool := range pools {
						if netip.AddrFrom4(pool.IPAddress) == address {
							out = append(out, natcommon.Item[WANPoolSpec]{Spec: WANPoolSpec{Interface: logical, Prefix: netip.PrefixFrom(address, int(row.Prefix.Len)).String(), VRF: pool.VrfID}, NeedsClaim: true})
						}
					}
				}
			}
			return out, nil
		},
	})
}
