package scheduler

// Stages (TD-13, audit ARCH-02, D-125): the risky backends go last (docs/01-architecture.md AD-4).
// Among operations that no dependency orders, every VPP-stage create and update runs before every
// daemon-stage one, and the deletes run the other way round (daemon-stage first), because a plan
// runs its deletes in the reverse of the create order. A real dependency always wins: the stage
// only breaks ties in the topological sort, before the descriptor registration order and the key.
// The rollback undoes the journal in the exact reverse of what ran, as before.

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
