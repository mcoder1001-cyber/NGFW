package agent

import (
	"context"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
	"testing"
	"time"
)

// Read-only runtime probes; creates no objects and never restarts VPP.
func TestDataplaneRuntimeOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := raw.WaitConnected(ctx); err != nil {
		t.Fatal(err)
	}
	out := &ngfwv1.DataplaneStartupStateResponse{}
	fillRuntime(ctx, raw, out)
	if len(out.RuntimeErrors) > 0 || len(out.RuntimeThreads) == 0 || out.LoadedPlugins == "" || out.RuntimeMemory == "" {
		t.Fatalf("threads=%d errors=%v plugins=%d memory=%d", len(out.RuntimeThreads), out.RuntimeErrors, len(out.LoadedPlugins), len(out.RuntimeMemory))
	}
	t.Logf("threads=%d plugins_bytes=%d queues_bytes=%d memory_bytes=%d", len(out.RuntimeThreads), len(out.LoadedPlugins), len(out.NicQueues), len(out.RuntimeMemory))
}
