package detectors

import (
	"fmt"
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	"time"
)

// Native deduplicates owned native IKE authentication failures by SA identity.
type Native struct{ seen map[string]time.Time }

// NewNative creates an empty native authentication-failure detector.
func NewNative() *Native { return &Native{seen: map[string]time.Time{}} }

// Observe resolves network roles by the configured local endpoint. IKE identities
// are untrusted and never used for blocking. Poll repeats of one failed SA count
// once, and deduplication remains bounded when hostile initiators churn SPIs.
func (n *Native) Observe(states []ikev2.SAState, ds *ngfwv1.DesiredState, now time.Time) []Observation {
	for k, at := range n.seen {
		if now.Sub(at) > 24*time.Hour {
			delete(n.seen, k)
		}
	}
	out := []Observation{}
	for _, sa := range states {
		if sa.State != "AUTH_FAILED" {
			continue
		}
		tunnel := ds.GetVpn().GetIpsec().GetTunnels()[sa.Profile]
		if tunnel == nil || tunnel.GetEngine() != "vpp-ikev2" || (tunnel.Enabled != nil && !tunnel.GetEnabled()) {
			continue
		}
		local, err := netip.ParseAddr(tunnel.GetLocalAddr())
		if err != nil {
			continue
		}
		initiator, ie := netip.ParseAddr(sa.IAddr)
		responder, re := netip.ParseAddr(sa.RAddr)
		if ie != nil || re != nil {
			continue
		}
		remote, remoteErr := netip.ParseAddr(tunnel.GetRemoteAddr())
		if remoteErr != nil || remote.Unmap() == local.Unmap() {
			continue
		}
		initiatedLocally := initiator.Unmap() == local.Unmap() && responder.Unmap() == remote.Unmap()
		initiatedRemotely := responder.Unmap() == local.Unmap() && initiator.Unmap() == remote.Unmap()
		if !initiatedLocally && !initiatedRemotely {
			continue
		}

		peer := source(remote.String())
		if peer == "" {
			continue
		}
		key := fmt.Sprintf("%s/%x/%x/%s", sa.Profile, sa.ISPI, sa.RSPI, peer)
		if _, ok := n.seen[key]; ok {
			continue
		}
		if len(n.seen) >= 10000 {
			continue
		}
		n.seen[key] = now
		out = append(out, Observation{peer, "vpnAuth"})
	}
	return out
}
