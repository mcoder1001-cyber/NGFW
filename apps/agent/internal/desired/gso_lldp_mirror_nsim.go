package desired

// F-loopback-bvi-gso-lldp-span entry points (wave-A-hotspots A2): projection.go calls
// LoopbackBviGsoLldpSpan once in project() and AssembleLoopbackBviGsoLldpSpan once in assemble()
// (after desired.Assemble). Loopbacks and loopback BVIs need nothing here: `interfaces.loop<N>` is
// P08's creator (core interface.loopback) and `interfaces.loop<N>.l2.bvi` F-bridge-l2's member object.
//
//	interfaces  gso.go (gso.interface), mirror.go (span.mirror)
//	services    lldp.go (lldp.global, lldp.interface), nsim.go (nsim.config, nsim.cross-connect,
//	            nsim.output)

import (
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// LoopbackBviGsoLldpSpanEnv is what the projection needs from the wiring (subsystems.LoopbackBviGsoLldpSpanEnv).
type LoopbackBviGsoLldpSpanEnv struct {
	// GlobalsOwner is D-071's flag: lldp.global and nsim are applied only by the globals owner.
	GlobalsOwner bool
	// Nsim is the lab gate (review M2): the globals owner with VRX_NSIM=lab; nsim is applied only then.
	Nsim bool
}

// LoopbackBviGsoLldpSpan emits the feature's objects for the domains in `in`.
func LoopbackBviGsoLldpSpan(s Sink, ds *vrxv1.DesiredState, in map[string]bool, env LoopbackBviGsoLldpSpanEnv) {
	if in["interfaces"] {
		Gso(s, ds.GetInterfaces())
		Mirror(s, ds.GetInterfaces())
	}
	if in["services"] {
		svc := ds.GetServices()
		Lldp(s, svc.GetLldp(), env.GlobalsOwner)
		Nsim(s, svc.GetNsim(), env)
	}
}

// AssembleLoopbackBviGsoLldpSpan adds the feature's retrievable leaves to ds: interfaces.<if>.gso and
// interfaces.<if>.mirror. services.lldp and services.nsim are write-only (nothing to report); `services`
// itself is present whenever it is requested (an implemented domain), like F-rpf-adl-pbr's assembler does.
func AssembleLoopbackBviGsoLldpSpan(ds *vrxv1.DesiredState, kvs []scheduler.KV, in map[string]bool, stored map[string]*vrxv1.Interface) {
	if in["interfaces"] {
		AssembleGso(ds, kvs, stored)
		AssembleMirror(ds, kvs, stored)
	}
	if in["services"] && ds.Services == nil {
		ds.Services = &vrxv1.ServicesConfig{}
	}
}
