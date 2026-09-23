package arp

import (
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the arp descriptors (proxy-range, proxy-interface) with r. tables scopes
// the ranges this agent owns on a shared VPP (nil = all).
func Register(r scheduler.Registry, c vpp.Client, owner string, tables *df2.IDRange) {
	r.Register(NewRange(c, tables))
	r.Register(NewInterface(c, owner))
}
