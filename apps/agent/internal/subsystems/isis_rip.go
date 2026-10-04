package subsystems

// F-isis-rip: the `isis` and `rip` FRR sections, the IS-IS interface lines, state reader and adjacency poller register
// themselves from init(); the FRR stage (frr.go, P12) renders every registered section, so these blank imports are the
// whole wiring.
import (
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/lcp_osi"
	"ngfw/agent/internal/desired"
	_ "ngfw/agent/internal/renderers/frr/isis" // the isis section, interface lines, reader and poller (init)
	_ "ngfw/agent/internal/renderers/frr/rip"  // the rip section (init)
	_ "ngfw/agent/internal/renderers/frr/ripng"
	"ngfw/agent/internal/scheduler"
	"os"
)

// registerIsisOSI registers only for the globals owner and projects only with explicit opt-in.
func registerIsisOSI(r scheduler.Registry, w *Wiring) {
	optedIn := os.Getenv("NGFW_ISIS_OSI_ENABLE") == "1"
	desired.ConfigureIsisOSI(w.env.GlobalsOwner, optedIn)
	if w.env.GlobalsOwner {
		r.Register(lcp_osi.New(w.env.Client, dfkit.GlobalsOwner(true), optedIn))
	}
}
