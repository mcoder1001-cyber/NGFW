package subsystems

// F-vrf-static-ecmp wiring (wave-A-hotspots A1): subsystems.go carries one registration line and the svs descriptor names in
// Domains[VRFs] under its anchors; everything else is here.

import (
	"sync"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/svs"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/scheduler"
)

// Descriptor names of the svs family (Domains[VRFs]).
const (
	svsTableName     = svs.TableName
	svsInterfaceName = svs.InterfaceName
	svsRouteName     = svs.RouteName
)

// registerVrfStaticEcmp registers the svs descriptors (the applied-once records of svs routes go to the persisted
// BootStore, D-076/D-080) and the D-072 static-route selector.
func registerVrfStaticEcmp(r scheduler.Registry, w *Wiring) {
	RegisterStaticSelector()
	svs.Register(r, svs.Env{Client: w.env.Client, Owner: w.env.Owner, Boot: w.boot, IfTableRef: core.InterfaceTableKey})
}

var staticSelectorOnce sync.Once

// The selector is installed when this package is linked (review L3), so every binary or test that projects or renders
// routing.static — the agent, the FRR renderer harness, P12's golden tests — sees `viaFrr` without calling Register.
// P12 and every other package must NOT call frr.RegisterStaticSelector: the one selector is ViaFrr, installed here.
func init() { RegisterStaticSelector() }

// RegisterStaticSelector installs ViaFrr as RF-1's D-072 selector, once per process: frr.RegisterStaticSelector panics on
// a second call and Register runs many times in tests. The package init already calls it; calling it again is a no-op.
func RegisterStaticSelector() {
	staticSelectorOnce.Do(func() { frr.RegisterStaticSelector(ViaFrr) })
}

// ViaFrr is the D-072 selector: routing.static[i] belongs to FRR (staticd) when it carries `viaFrr: true` (or RF-1's
// renderer-side stand-in flag, so the FRR renderer's own tests keep working); the agent never programs such a route.
func ViaFrr(i int, sr *vrxv1.StaticRoute, ext *frr.Extensions) bool {
	return sr.GetViaFrr() || frr.FlaggedStatic(i, sr, ext)
}

// SvsRange is the id range source-VRF-select tables are allocated from, derived from the agent's VPP id scope (TD-8,
// fail closed): the top 100 ids of the slot's or reserved range with VRX_VPP_TABLE_BASE (shared-host rules §1, §12),
// svs.DefaultRange (just below 2^32-1) with VRX_VPP_ID_RANGE=all (the product agent on a box of its own), and the empty
// range when neither is set or the setting is malformed or contradictory — the projection then fails loudly on the first
// sourceSelect entry instead of allocating ids the agent does not own (it needs no id otherwise).
//
// It reads ResolveIDScope, the same function the agent resolves Config.IDs (and so Env.IDs / Wiring.IDRange) from:
// the projection runs in the service, which holds no Wiring, and the service and agent core are not this feature's
// files — so the scope is resolved from the same environment rather than passed down (F-vrf-static-ecmp questions Q16).
func SvsRange() svs.Range {
	s, err := ResolveIDScope()
	switch {
	case err != nil:
		return svs.Range{}
	case s.Range != nil:
		return svs.RangeIn(s.Range.Lo, s.Range.Hi)
	case s.All:
		return svs.DefaultRange
	}
	return svs.Range{}
}
