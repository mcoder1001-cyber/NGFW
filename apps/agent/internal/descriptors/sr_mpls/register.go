package sr_mpls //nolint:revive,stylecheck // see policy.go

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the SR-MPLS descriptors (policy, steering, endpoint color) with r.
func Register(r scheduler.Registry, c vpp.Client) {
	r.Register(NewPolicy(c))
	r.Register(NewSteering(c))
	r.Register(NewEndpointColor(c))
}
