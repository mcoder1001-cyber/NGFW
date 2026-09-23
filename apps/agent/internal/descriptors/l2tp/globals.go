package l2tp

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/interface_types"
	l2tpapi "ngfw/agent/binapi/l2tp"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// InterfaceEnableName is the descriptor name; keys are "l2tp.interface-enable/<interface>".
const InterfaceEnableName = "l2tp.interface-enable"

// LookupKeyName is the descriptor name; the key is "l2tp.lookup-key/global".
const LookupKeyName = "l2tp.lookup-key"

// InterfaceEnableDescriptor enables L2TPv3 decap on an interface (write-only).
type InterfaceEnableDescriptor = df6.BypassDescriptor[*InterfaceEnable]

// NewInterfaceEnable returns the descriptor for the given owner.
func NewInterfaceEnable(c vpp.Client, owner string) *InterfaceEnableDescriptor {
	return df6.NewToggleDescriptor(df6.ToggleSpec[*InterfaceEnable]{
		Name:   InterfaceEnableName,
		Plugin: Plugin,
		Iface:  func(e *InterfaceEnable) string { return e.GetInterface() },
		Set: func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, enable bool) error {
			if _, err := l2tpapi.NewServiceClient(c).L2tpv3InterfaceEnableDisable(ctx, &l2tpapi.L2tpv3InterfaceEnableDisable{SwIfIndex: idx, EnableDisable: enable}); err != nil {
				return fmt.Errorf("l2tpv3_interface_enable_disable: %w", err)
			}
			return nil
		},
	}, c, owner)
}

// LookupKeyDescriptor sets the global L2TPv3 session lookup key (write-only singleton;
// Delete is a no-op because VPP has no getter or reset).
type LookupKeyDescriptor = df6.SingletonDescriptor[*LookupKey]

// NewLookupKey returns the descriptor.
func NewLookupKey(c vpp.Client) *LookupKeyDescriptor {
	return df6.NewSingletonDescriptor(df6.SingletonSpec[*LookupKey]{
		Name:   LookupKeyName,
		Plugin: Plugin,
		Validate: func(k *LookupKey) error {
			if k.GetKey() < LookupKeyType_SRC_ADDR || k.GetKey() > LookupKeyType_SESSION_ID {
				return fmt.Errorf("%w: unknown lookup key %d", df6.ErrBadValue, k.GetKey())
			}
			return nil
		},
		Set: func(ctx context.Context, c vpp.Client, k *LookupKey) error {
			if _, err := l2tpapi.NewServiceClient(c).L2tpv3SetLookupKey(ctx, &l2tpapi.L2tpv3SetLookupKey{Key: l2tpapi.L2tLookupKey(k.GetKey())}); err != nil { //nolint:gosec // validated enum
				return fmt.Errorf("l2tpv3_set_lookup_key: %w", err)
			}
			return nil
		},
	}, c)
}
