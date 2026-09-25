package hoststack

import (
	"context"
	"errors"
	"sort"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp"
)

// Snapshot is the read-only host-stack view of one owner (HostStackState).
type Snapshot struct {
	SessionEnabled bool
	SessionDetail  string
	Namespaces     []string // applied by this owner on the running VPP instance
	Rules          []Rule   // this owner's rules
	AppnsIndexes   map[string][]uint32
	RuleCountTotal uint32
}

// State reads the snapshot. It never mutates VPP.
func State(ctx context.Context, c vpp.Client, owner string) (Snapshot, error) {
	var snap Snapshot
	details, err := dumpRules(ctx, c)
	switch {
	case errors.Is(err, dfkit.ErrPluginNotLoaded):
		snap.SessionDetail = "session plugin not loaded"
	case err != nil:
		on, perr := Probe(ctx, c)
		if perr != nil {
			return snap, perr
		}
		if !on {
			snap.SessionDetail = "session layer disabled (session_rules_v2_dump refused)"
		}
	default:
		snap.SessionEnabled = true
	}
	idx := map[string]uint32{}
	if st, ok := lookupState(owner); ok {
		if idx, err = st.currentIndexes(ctx); err != nil {
			return snap, err
		}
	}
	snap.Namespaces = sortedKeys(idx)
	snap.AppnsIndexes = map[string][]uint32{}
	snap.RuleCountTotal = uint32(len(details)) //nolint:gosec // rule count
	for _, d := range details {
		if r, ok := ruleFromDetails(owner, d, idx); ok {
			snap.Rules = append(snap.Rules, r)
			snap.AppnsIndexes[r.Tag] = d.AppnsIndex
		}
	}
	sort.Slice(snap.Rules, func(i, j int) bool { return snap.Rules[i].Tag < snap.Rules[j].Tag })
	return snap, nil
}
