package ikev2

import (
	"context"
	"errors"
	api "ngfw/agent/binapi/ikev2"
	"ngfw/agent/internal/vpp"
)

// SafeStateRevision identifies the NGFW patch that guards profile lookup for
// unauthenticated SAs. Unpatched VPP 26.06 crashes on normal state polling during
// IKE_AUTH. A plugin version alone must never be treated as evidence of this fix.
const SafeStateRevision uint32 = 0x56525801

var ErrUnsafeState = errors.New("native IPsec requires the VPP safe-state patch (0002-ikev2-safe-native-state)")

func RequireSafeState(ctx context.Context, c vpp.Client) error {
	v, err := api.NewServiceClient(c).Ikev2PluginGetVersion(ctx, &api.Ikev2PluginGetVersion{})
	if err != nil {
		return ErrUnsafeState
	}
	if v.Major != 1 || v.Minor != SafeStateRevision {
		return ErrUnsafeState
	}
	return nil
}
