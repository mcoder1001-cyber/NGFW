package vxlan_gpe

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/interface_types"
	gpeapi "ngfw/agent/binapi/vxlan_gpe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// BypassName is the descriptor name; keys are "vxlan-gpe.bypass/<interface>".
const BypassName = "vxlan-gpe.bypass"

// BypassDescriptor toggles ip4/ip6-vxlan-gpe-bypass on an interface (write-only, no dump).
type BypassDescriptor = df6.BypassDescriptor[*Bypass]

// NewBypass returns the descriptor for the given owner.
func NewBypass(c vpp.Client, owner string) *BypassDescriptor {
	return df6.NewBypassDescriptor(df6.BypassSpec[*Bypass]{
		Name:   BypassName,
		Plugin: Plugin,
		Fields: func(b *Bypass) (string, bool, bool) { return b.GetInterface(), b.GetIpv4(), b.GetIpv6() },
		Set: func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6, enable bool) error {
			if _, err := gpeapi.NewServiceClient(c).SwInterfaceSetVxlanGpeBypass(ctx, &gpeapi.SwInterfaceSetVxlanGpeBypass{SwIfIndex: idx, IsIPv6: ipv6, Enable: enable}); err != nil {
				return fmt.Errorf("sw_interface_set_vxlan_gpe_bypass: %w", err)
			}
			return nil
		},
	}, c, owner)
}
