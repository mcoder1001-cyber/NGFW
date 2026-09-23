package pppoe

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/interface_types"
	pppoeapi "ngfw/agent/binapi/pppoe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// CpName is the descriptor name; keys are "pppoe.cp/<interface>".
const CpName = "pppoe.cp"

// CpDescriptor sets the PPPoE control-plane punt interface (write-only, no dump).
type CpDescriptor = df6.BypassDescriptor[*Cp]

// NewCp returns the descriptor for the given owner.
func NewCp(c vpp.Client, owner string) *CpDescriptor {
	return df6.NewToggleDescriptor(df6.ToggleSpec[*Cp]{
		Name:   CpName,
		Plugin: Plugin,
		Iface:  func(cp *Cp) string { return cp.GetInterface() },
		Set: func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, enable bool) error {
			var isAdd uint8
			if enable {
				isAdd = 1
			}
			if _, err := pppoeapi.NewServiceClient(c).PppoeAddDelCp(ctx, &pppoeapi.PppoeAddDelCp{SwIfIndex: idx, IsAdd: isAdd}); err != nil {
				return fmt.Errorf("pppoe_add_del_cp: %w", err)
			}
			return nil
		},
	}, c, owner)
}
