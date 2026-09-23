package pppoe

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the pppoe descriptors (session, cp) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewSession(c, owner))
	r.Register(NewCp(c, owner))
}
