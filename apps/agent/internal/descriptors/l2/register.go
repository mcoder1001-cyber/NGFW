package l2

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers every l2 descriptor with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewBridgeDomain(c, owner))
	r.Register(NewMember(c, owner))
	r.Register(NewXconnect(c, owner))
	r.Register(NewFibEntry(c, owner))
	r.Register(NewFlags(c, owner))
	r.Register(NewVlanTagRewrite(c, owner))
}
