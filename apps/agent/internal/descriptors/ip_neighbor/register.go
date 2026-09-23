package ipneighbor

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the ip_neighbor descriptors (neighbor, config) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewNeighbor(c, owner))
	r.Register(NewConfig(c))
}
