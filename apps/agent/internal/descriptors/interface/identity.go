package iface

import (
	"context"
	"sync"

	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// vppEpoch remembers the VPP identity (D-080 boot identity, bootid.Current) a process-memory
// "what I set" map (promisc, MAC) belongs to. After a VPP restart sw_if_indexes are reused by
// other interfaces, so the map must be dropped (review L1). Best effort: when the identity cannot
// be read the map is kept.
type vppEpoch struct {
	mu    sync.Mutex
	known bool
	id    bootid.Identity
}

// restarted records the current identity and reports whether it differs from the recorded one.
func (e *vppEpoch) restarted(ctx context.Context, client vpp.Client) bool {
	id, err := bootid.Current(ctx, client)
	if err != nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	changed := e.known && !e.id.Equal(id)
	e.id, e.known = id, true
	return changed
}
