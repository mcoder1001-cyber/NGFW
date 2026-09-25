package scheduler

// Stages (TD-13, audit ARCH-02, D-125): the risky backends go last (docs/01-architecture.md AD-4).
// The stage breaks ties in the dependency sort: it is the first tie-breaker of the topological
// order, before the descriptor registration order and the key. Whenever a VPP-stage and a
// daemon-stage operation are both ready, the VPP-stage one goes first; so a daemon object that no
// VPP object waits for runs after every VPP object that is ready by then. It is a greedy tie-break,
// not a phase split: a daemon object that a VPP object depends on runs early (the dependency wins),
// and another ready daemon object may then run before that VPP dependent. Deletes run in the reverse
// order (daemon-stage first when ready together). The rollback undoes the journal in the exact
// reverse of what ran, as before.

// Stage is the phase of a transaction a descriptor's operations belong to.
type Stage int

// Stages, in execution order of creates and updates.
const (
	// StageVPP is the default: objects in VPP (binary API).
	StageVPP Stage = iota
	// StageDaemon: configuration of a daemon (Kea, Unbound, FRR, strongSwan, nftables, …), applied
	// after the VPP objects of the same transaction.
	StageDaemon
)

func (st Stage) String() string {
	switch st {
	case StageVPP:
		return "vpp"
	case StageDaemon:
		return "daemon"
	default:
		return "unknown"
	}
}

// Stager is an optional Descriptor extension: the stage of the descriptor's operations. A
// descriptor without it is StageVPP. Stage must be constant for a descriptor.
type Stager interface {
	Stage() Stage
}

// StageOf returns d's stage (StageVPP when d does not declare one).
func StageOf(d Descriptor) Stage {
	if st, ok := d.(Stager); ok {
		return st.Stage()
	}
	return StageVPP
}

// stages maps every registered descriptor name to its stage (the topological tie-breaker).
func (s *Scheduler) stages() map[string]Stage {
	out := make(map[string]Stage)
	for _, d := range s.reg.Descriptors() {
		out[d.Name()] = StageOf(d)
	}
	return out
}
