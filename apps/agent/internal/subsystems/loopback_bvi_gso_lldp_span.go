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
	"os"
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
var loopbackState struct{ globalsOwner, nsim atomic.Bool }

// The nsim lab gate (review M2): nsim is a lab tool with VPP-side hazards (a worker box without poll-main-thread
// crashes on main-thread traffic; configuring it keeps the main thread polling until VPP restarts), so the agent applies
// it only as the globals owner AND with VRX_NSIM=lab (off by default); VRX_NSIM_POLL_MAIN_THREAD=1 asserts that
// startup.conf carries `nsim { poll-main-thread }` (needed when VPP has worker threads).
const (
	EnvNsim               = "VRX_NSIM"
	EnvNsimLab            = "lab"
	EnvNsimPollMainThread = "VRX_NSIM_POLL_MAIN_THREAD"
)

// getenv is os.Getenv (a variable for tests).
var getenv = os.Getenv

// LoopbackBviGsoLldpSpanEnv returns what desired.LoopbackBviGsoLldpSpan needs.
func LoopbackBviGsoLldpSpanEnv() desired.LoopbackBviGsoLldpSpanEnv {
	return desired.LoopbackBviGsoLldpSpanEnv{GlobalsOwner: loopbackState.globalsOwner.Load(), Nsim: loopbackState.nsim.Load()}
}

// registerLoopbackBviGsoLldpSpan registers the feature's families. gso.interface and nsim keep their
// D-076/D-080 applied-once records in the owner's persisted BootStore (never in memory); DF-7's lldp
// and span families take theirs through df7.SetBootStore (Register) and the persisted interface claims.
func (w *Wiring) registerLoopbackBviGsoLldpSpan(r scheduler.Registry) {
	c, owner := w.env.Client, w.env.Owner
	nsimOn := w.env.GlobalsOwner && getenv(EnvNsim) == EnvNsimLab
	loopbackState.globalsOwner.Store(w.env.GlobalsOwner)
	loopbackState.nsim.Store(nsimOn)
	gso.Register(r, c, owner, w.BootStore())
	span.Register(r, c, owner)
	lldp.Register(r, c, owner)
	switch {
	case !w.env.GlobalsOwner:
		w.env.Log.Info("not the globals owner: lldp.global and nsim are not registered; services.lldp globals and services.nsim are reported as unsupported (D-071)")
	case !nsimOn:
		lldp.RegisterGlobals(r, c, owner)
		w.env.Log.Info("nsim (lab tool) is off: set " + EnvNsim + "=" + EnvNsimLab + " to apply services.nsim")
	default:
		lldp.RegisterGlobals(r, c, owner)
		nsim.RegisterGlobals(r, c, owner, w.BootStore(), nsim.WithPollMainThread(getenv(EnvNsimPollMainThread) == "1"))
		w.env.Log.Warn("nsim (lab tool) is on: configuring it keeps VPP's main thread polling until VPP restarts")
	}
}
