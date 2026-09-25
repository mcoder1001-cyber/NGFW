package iface

import (
	"ngfw/agent/internal/descriptors/kit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers every descriptor of this package with r. Order matters only as the
// deterministic tie-breaker of the plan: attributes come after the sub-interface so that a
// sub-interface's attributes sort behind their parent's.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	kit.Register(r, kit.Env{Client: c, Owner: owner},
		func(e kit.Env) scheduler.Descriptor { return NewSubinterface(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewAdminState(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewMtu(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewMacAddress(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewPromisc(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewRxMode(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewRxPlacement(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewAlias(e.Client, e.Owner) }, // "interface/<name>", the key every consumer depends on (D-065)
	)
}
