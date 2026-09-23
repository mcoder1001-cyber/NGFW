package ipneighbor

import (
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the ip_neighbor descriptors (neighbor, config) with r. Pass the agent's
// persisted claim store with df2.WithClaims so neighbours on physical ports survive a restart.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(NewNeighbor(c, owner, opts...))
	r.Register(NewConfig(c))
}
