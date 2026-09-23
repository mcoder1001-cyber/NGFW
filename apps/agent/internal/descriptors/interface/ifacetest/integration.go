package ifacetest

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// LoopbackKeyPrefix is P05 core's loopback descriptor name (docs/agent/descriptors/interface.md).
const LoopbackKeyPrefix = "interface.loopback/"

// Loopback creates loop<slot><ii> on the host VPP (create_loopback_instance with
// vpptest.LoopbackInstance(t, i)), tags it "<owner>:loop<inst>" exactly as P05 core's descriptor
// will, deletes it in Cleanup and returns its sw_if_index and full key.
func Loopback(t testing.TB, c vpp.Client, owner string, i int) (uint32, string) {
	t.Helper()
	ctx := context.Background()
	inst := vpptest.LoopbackInstance(t, i)
	name := fmt.Sprintf("loop%d", inst)
	svc := ifapi.NewServiceClient(c)
	rep, err := svc.CreateLoopbackInstance(ctx, &ifapi.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatalf("create_loopback_instance %s: %v", name, err)
	}
	idx := uint32(rep.SwIfIndex)
	t.Cleanup(func() {
		if _, err := svc.DeleteLoopback(context.Background(), &ifapi.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
			t.Errorf("cleanup delete_loopback %s: %v", name, err)
		}
	})
	tag, err := vpp.OwnerTag(owner, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwInterfaceTagAddDel(ctx, &ifapi.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
		t.Fatalf("tag %s: %v", name, err)
	}
	return idx, LoopbackKeyPrefix + name
}

// Workers returns the number of VPP worker threads (show_threads, type "workers"); 0 on the
// dev host (cpu { } in startup.conf).
func Workers(t testing.TB, c vpp.Client) int {
	t.Helper()
	rep, err := vlib.NewServiceClient(c).ShowThreads(context.Background(), &vlib.ShowThreads{})
	if err != nil {
		t.Fatalf("show_threads: %v", err)
	}
	n := 0
	for _, th := range rep.ThreadData {
		if th.Type == "workers" {
			n++
		}
	}
	return n
}

// Hold sleeps VRX_DF1_HOLD seconds (if set) so an operator can run vppctl show ... against the
// objects a test created before its Cleanup removes them. Never set in CI.
func Hold(t testing.TB) {
	t.Helper()
	if s := os.Getenv("VRX_DF1_HOLD"); s != "" {
		n, err := strconv.Atoi(s)
		if err == nil && n > 0 {
			t.Logf("VRX_DF1_HOLD: holding objects for %ds", n)
			time.Sleep(time.Duration(n) * time.Second)
		}
	}
}

// Find returns the KV with key from kvs, or fails.
func Find[T any](t testing.TB, kvs []T, key string, keyOf func(T) string) T {
	t.Helper()
	for _, kv := range kvs {
		if keyOf(kv) == key {
			return kv
		}
	}
	var zero T
	t.Fatalf("Retrieve does not contain %s (got %d objects)", key, len(kvs))
	return zero
}
