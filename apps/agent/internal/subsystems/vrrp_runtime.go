package subsystems

import (
	"context"
	"ngfw/agent/internal/renderers/keepalived"
	"sync/atomic"
)

var keepalivedRuntime atomic.Pointer[keepalived.Renderer]

// KeepalivedState observes only this agent's daemon; the same engine/path gate as Apply applies.
func KeepalivedState(ctx context.Context) (*keepalived.State, error) {
	if !keepalivedEnabled.Load() {
		return nil, nil
	}
	r := keepalivedRuntime.Load()
	if r == nil {
		return nil, nil
	}
	return r.State(ctx)
}
