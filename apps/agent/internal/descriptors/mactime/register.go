package mactime

import (
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the mactime descriptors with r. store is the owner's persisted applied-once
// store (subsystems.Wiring.BootStore, D-076/D-080); nil = in memory (tests only).
func Register(r scheduler.Registry, c vpp.Client, owner string, store dfkit.BootStore) {
	r.Register(NewDevice(c, owner))
	r.Register(NewEnable(c, owner, store))
}
