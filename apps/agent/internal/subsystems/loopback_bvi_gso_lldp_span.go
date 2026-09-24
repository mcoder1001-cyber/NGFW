package subsystems

// F-loopback-bvi-gso-lldp-span wiring (wave-A-hotspots A1: the registration and the descriptor names
// live here; subsystems.go carries the Interfaces names and one registration call under this task's
// anchors):
//
//	interfaces  F-loopback-bvi-gso-lldp-span: gso.interface (applied-once records in the persisted
//	            BootStore, feature_is_enabled read-back); DF-7: span.mirror
//	services    DF-7: lldp.interface (every agent), lldp.global (globals owner only, D-071);
//	            nsim.config, nsim.cross-connect, nsim.output (globals owner only, D-071; applied-once
//	            records in the persisted BootStore)
//
// Loopbacks are P08's (core interface.loopback), their BVI role F-bridge-l2's (l2.bridge-domain-member):
// nothing is registered for them here.
//
// The `services` domain: F-rpf-adl-pbr registers it first (its `Services` constant and `Services: {…}`
// entry in subsystems.go). This branch does not contain it, so the names below are appended to
// Domains["services"] from init() — the append composes with F-rpf-adl-pbr's literal entry at the merge
// (no duplicate map key, no second constant; servicesDomain is the same string).

import (
	"sync/atomic"

	"ngfw/agent/internal/descriptors/gso"
	"ngfw/agent/internal/descriptors/lldp"
	"ngfw/agent/internal/descriptors/nsim"
	"ngfw/agent/internal/descriptors/span"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// servicesDomain is the `services` root key (F-rpf-adl-pbr's subsystems.Services).
const servicesDomain = "services"

// Descriptor names of F-loopback-bvi-gso-lldp-span.
const (
	loopbackGso        = gso.Name
	loopbackSpanMirror = span.NameMirror
)

// loopbackServices are the feature's descriptors of the services domain.
var loopbackServices = []string{
	lldp.NameGlobal,
	lldp.NameInterface,
	nsim.ConfigName,
	nsim.CrossConnectName,
	nsim.OutputName,
}

func init() {
	Domains[servicesDomain] = append(Domains[servicesDomain], loopbackServices...)
}

// loopbackState is what the projection reads (desired.LoopbackBviGsoLldpSpanEnv): process-wide, set by
// Register.
var loopbackState struct{ globalsOwner atomic.Bool }

// LoopbackBviGsoLldpSpanEnv returns what desired.LoopbackBviGsoLldpSpan needs.
func LoopbackBviGsoLldpSpanEnv() desired.LoopbackBviGsoLldpSpanEnv {
	return desired.LoopbackBviGsoLldpSpanEnv{GlobalsOwner: loopbackState.globalsOwner.Load()}
}

// registerLoopbackBviGsoLldpSpan registers the feature's families. gso.interface and nsim keep their
// D-076/D-080 applied-once records in the owner's persisted BootStore (never in memory); DF-7's lldp
// and span families take theirs through df7.SetBootStore (Register) and the persisted interface claims.
func (w *Wiring) registerLoopbackBviGsoLldpSpan(r scheduler.Registry) {
	c, owner := w.env.Client, w.env.Owner
	loopbackState.globalsOwner.Store(w.env.GlobalsOwner)
	gso.Register(r, c, owner, w.BootStore())
	span.Register(r, c, owner)
	lldp.Register(r, c, owner)
	if w.env.GlobalsOwner {
		lldp.RegisterGlobals(r, c, owner)
		nsim.RegisterGlobals(r, c, owner, w.BootStore())
	} else {
		w.env.Log.Info("not the globals owner: lldp.global and nsim are not registered; services.lldp globals and services.nsim are reported as unsupported (D-071)")
	}
}
