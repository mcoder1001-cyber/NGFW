package iface

import (
	"context"
	"fmt"
	"sync"

	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/vpp"
)

// VPPIdentity returns the PID of VPP's main thread (show_threads, thread 0). It changes whenever
// VPP restarts (D-066, same approach as DF-4's acl.stats-enable).
func VPPIdentity(ctx context.Context, client vpp.Client) (uint32, error) {
	rep, err := vlib.NewServiceClient(client).ShowThreads(ctx, &vlib.ShowThreads{})
	if err != nil {
		return 0, fmt.Errorf("show_threads: %w", err)
	}
	for _, t := range rep.ThreadData {
		if t.ID == 0 {
			return t.PID, nil
		}
	}
	return 0, fmt.Errorf("show_threads: no main thread in %d entries", len(rep.ThreadData))
}

// vppEpoch remembers the VPP identity a process-memory "what I set" map (promisc, MAC) belongs to.
// After a VPP restart sw_if_indexes are reused by other interfaces, so the map must be dropped
// (review L1). Best effort: when show_threads fails the map is kept.
type vppEpoch struct {
	mu  sync.Mutex
	pid uint32
}

// restarted records the current identity and reports whether it differs from the recorded one.
func (e *vppEpoch) restarted(ctx context.Context, client vpp.Client) bool {
	pid, err := VPPIdentity(ctx, client)
	if err != nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	changed := e.pid != 0 && e.pid != pid
	e.pid = pid
	return changed
}
