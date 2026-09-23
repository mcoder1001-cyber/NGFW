package gtpu

import (
	"context"
	"fmt"

	gtpuapi "ngfw/agent/binapi/gtpu"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// BypassName is the descriptor name; keys are "gtpu.bypass/<interface>".
const BypassName = "gtpu.bypass" //nolint:gosec // descriptor name, not a credential

// BypassDescriptor toggles ip4/ip6-gtpu-bypass on an interface (write-only, no dump).
type BypassDescriptor = df6.BypassDescriptor[*Bypass]

// NewBypass returns the descriptor for the given owner.
func NewBypass(c vpp.Client, owner string, opts ...df6.Option) *BypassDescriptor {
	return df6.NewBypassDescriptor(df6.BypassSpec[*Bypass]{
		Probe:  df6.FeatureProbe("ip4-unicast", "ip4-gtpu-bypass", "ip6-unicast", "ip6-gtpu-bypass"),
		Name:   BypassName,
		Plugin: Plugin,
		Fields: func(b *Bypass) (string, bool, bool) { return b.GetInterface(), b.GetIpv4(), b.GetIpv6() },
		Set: func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6, enable bool) error {
			if _, err := gtpuapi.NewServiceClient(c).SwInterfaceSetGtpuBypass(ctx, &gtpuapi.SwInterfaceSetGtpuBypass{SwIfIndex: idx, IsIPv6: ipv6, Enable: enable}); err != nil {
				return fmt.Errorf("sw_interface_set_gtpu_bypass: %w", err)
			}
			return nil
		},
	}, c, owner, opts...)
}
