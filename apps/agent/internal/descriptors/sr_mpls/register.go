package sr_mpls //nolint:revive // see policy.go

import (
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the SR-MPLS descriptors (policy, steering, endpoint color) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df6.Option) {
	r.Register(NewPolicy(c, owner, opts...))
	r.Register(NewSteering(c, owner, opts...))
	r.Register(NewEndpointColor(c, owner, opts...))
}
