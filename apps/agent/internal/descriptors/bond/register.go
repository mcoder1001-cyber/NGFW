package bond

import (
	"ngfw/agent/internal/descriptors/kit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the bond descriptors with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	kit.Register(r, kit.Env{Client: c, Owner: owner},
		func(e kit.Env) scheduler.Descriptor { return NewBond(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewMember(e.Client, e.Owner) },
	)
}
