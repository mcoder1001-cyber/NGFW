package subsystems

// F-ospf: the `ospf` FRR section, its interface lines, state readers and neighbour poller register themselves from
// init(); the FRR stage (frr.go, P12) renders every registered section, so this blank import is the whole wiring.
import _ "ngfw/agent/internal/renderers/frr/ospf" // the ospf section, interface lines, readers and poller (init)
