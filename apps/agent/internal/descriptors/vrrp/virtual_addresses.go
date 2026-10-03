package vrrp

import (
	"context"
	"errors"
	"go.fd.io/govpp/adapter"
	"ngfw/agent/binapi/vrrp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
)

// OwnedVirtualAddresses names only VIPs installed by this owner's active accept-mode Master VRs.
// Backup/disabled VRs and VRs without an ownership claim never hide interface addresses.
func OwnedVirtualAddresses(ctx context.Context, c vpp.Client, owner string) (map[uint32]map[string]bool, error) {
	out := map[uint32]map[string]bool{}
	vrs, _, err := ownedVRs(ctx, df7.NewBase(NameVR, c, owner, nil))
	if err != nil {
		var unknown *adapter.UnknownMsgError
		if errors.As(err, &unknown) {
			return out, nil
		}
		return nil, err
	}
	for _, vr := range vrs {
		if vr.Detail.Config.Flags&vrrp.VRRP_API_VR_ACCEPT == 0 || vr.Detail.Runtime.State != vrrp.VRRP_API_VR_STATE_MASTER {
			continue
		}
		if out[vr.Idx] == nil {
			out[vr.Idx] = map[string]bool{}
		}
		for _, address := range fromAddrs(vr.Detail.Addrs) {
			out[vr.Idx][address] = true
		}
	}
	return out, nil
}
