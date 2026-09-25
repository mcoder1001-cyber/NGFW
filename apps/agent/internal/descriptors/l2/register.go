package l2

import (
	"ngfw/agent/internal/descriptors/kit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers every l2 descriptor with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	kit.Register(r, kit.Env{Client: c, Owner: owner},
		func(e kit.Env) scheduler.Descriptor { return NewBridgeDomain(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewMember(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewXconnect(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewFibEntry(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewFlags(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewVlanTagRewrite(e.Client, e.Owner) },
	)
}
