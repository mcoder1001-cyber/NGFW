package mactime

import (
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the mactime descriptors with r. store is the owner's persisted applied-once
// store (subsystems.Wiring.BootStore, D-076/D-080); nil = in memory (tests only). globalsOwner is
// D-071's flag: only then may a configured device replace VPP's own learned entry for its MAC.
func Register(r scheduler.Registry, c vpp.Client, owner string, store dfkit.BootStore, globalsOwner bool) {
	r.Register(NewDevice(c, owner, WithGlobalsOwner(globalsOwner)))
	r.Register(NewEnable(c, owner, store))
}
