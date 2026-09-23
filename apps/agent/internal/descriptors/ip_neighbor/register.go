package ipneighbor

import (
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the per-owner ip_neighbor descriptor (neighbor) with r. Pass the agent's
// persisted claim store with df2.WithClaims so neighbours on physical ports survive a restart.
// The VPP-global ip-neighbor.config is NOT registered here (D-071): see RegisterGlobals.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(NewNeighbor(c, owner, opts...))
}

// RegisterGlobals registers ip-neighbor.config, a VPP-global singleton. Only the designated
// globals owner (agent config globalsOwner: true — the product agent on a real box, never a
// test slot on the shared host) may call it (D-071): a non-owner that registered it would set
// the neighbour limits for everyone, and its Delete resets them to VPP defaults.
func RegisterGlobals(r scheduler.Registry, c vpp.Client) {
	r.Register(NewConfig(c))
}
