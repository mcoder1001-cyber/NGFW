package bond

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the bond descriptors with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewBond(c, owner))
	r.Register(NewMember(c, owner))
}
