package l2tp

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the l2tp descriptors (tunnel, interface-enable, lookup-key) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewTunnel(c, owner))
	r.Register(NewInterfaceEnable(c, owner))
	r.Register(NewLookupKey(c))
}
