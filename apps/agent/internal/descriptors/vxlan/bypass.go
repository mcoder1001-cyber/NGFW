package vxlan

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/interface_types"
	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// BypassName is the descriptor name; keys are "vxlan.bypass/<interface>".
const BypassName = "vxlan.bypass" //nolint:gosec // descriptor name, not a credential

// BypassDescriptor toggles ip4/ip6-vxlan-bypass on an interface (write-only, no dump).
type BypassDescriptor = df6.BypassDescriptor[*Bypass]

// NewBypass returns the descriptor for the given owner.
func NewBypass(c vpp.Client, owner string) *BypassDescriptor {
	return df6.NewBypassDescriptor(df6.BypassSpec[*Bypass]{
		Name:   BypassName,
		Plugin: Plugin,
		Fields: func(b *Bypass) (string, bool, bool) { return b.GetInterface(), b.GetIpv4(), b.GetIpv6() },
		Set: func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6, enable bool) error {
			if _, err := vxlanapi.NewServiceClient(c).SwInterfaceSetVxlanBypass(ctx, &vxlanapi.SwInterfaceSetVxlanBypass{SwIfIndex: idx, IsIPv6: ipv6, Enable: enable}); err != nil {
				return fmt.Errorf("sw_interface_set_vxlan_bypass: %w", err)
			}
			return nil
		},
	}, c, owner)
}
