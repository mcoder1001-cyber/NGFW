package iface

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers every descriptor of this package with r. Order matters only as the
// deterministic tie-breaker of the plan: attributes come after the sub-interface so that a
// sub-interface's attributes sort behind their parent's.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewSubinterface(c, owner))
	r.Register(NewAdminState(c, owner))
	r.Register(NewMtu(c, owner))
	r.Register(NewMacAddress(c, owner))
	r.Register(NewPromisc(c, owner))
	r.Register(NewRxMode(c, owner))
	r.Register(NewRxPlacement(c, owner))
	r.Register(NewAlias(c, owner)) // "interface/<name>", the key every consumer depends on (D-065)
}
