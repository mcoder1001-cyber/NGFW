package subsystems

// F-isis-rip: the `isis` and `rip` FRR sections, the IS-IS interface lines, state reader and adjacency poller register
// themselves from init(); the FRR stage (frr.go, P12) renders every registered section, so these blank imports are the
// whole wiring.
import (
	_ "ngfw/agent/internal/renderers/frr/isis" // the isis section, interface lines, reader and poller (init)
	_ "ngfw/agent/internal/renderers/frr/rip"  // the rip section (init)
)
