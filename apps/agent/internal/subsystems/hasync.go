package subsystems

import (
	"ngfw/agent/internal/descriptors/hasync"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

func init() { Domains[HA] = append(Domains[HA], hasync.NameListener, hasync.NameFailover) }
func registerHaSync(r scheduler.Registry, w *Wiring) {
	hasync.Register(r, w.env.Client, natcommon.WithGlobalsOwner(w.env.GlobalsOwner))
}
