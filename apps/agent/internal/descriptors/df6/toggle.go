package df6

import (
	"context"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
)

// ToggleSpec describes a per-interface on/off feature without an address family
// (l2tpv3_interface_enable_disable, pppoe_add_del_cp). Like BypassSpec, VPP has no dump for
// these, so the descriptor is write-only.
type ToggleSpec[T proto.Message] struct {
	Name   string
	Plugin string
	// Iface returns the interface name of obj.
	Iface func(obj T) string
	// Set enables or disables the feature on idx.
	Set func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, enable bool) error
}

// NewToggleDescriptor builds the descriptor from spec on top of the bypass implementation
// (the "IPv4" family carries the single on/off state).
func NewToggleDescriptor[T proto.Message](spec ToggleSpec[T], c vpp.Client, owner string) *BypassDescriptor[T] {
	return NewBypassDescriptor(BypassSpec[T]{
		Name:   spec.Name,
		Plugin: spec.Plugin,
		Fields: func(obj T) (string, bool, bool) { return spec.Iface(obj), true, false },
		Set: func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, _ bool, enable bool) error {
			return spec.Set(ctx, c, idx, enable)
		},
	}, c, owner)
}
