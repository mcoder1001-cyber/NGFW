package arp

import (
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the arp descriptors (proxy-range, proxy-interface) with r. tables scopes
// the ranges this agent owns on a shared VPP (nil = all, the single production agent); opts
// (df2.WithClaims) attribute proxy-ARP on untagged interfaces.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, tables *df2.IDRange, opts ...df2.Option) {
	r.Register(NewRange(c, tables))
	r.Register(NewInterface(c, owner, opts...))
}
