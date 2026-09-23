package iface_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	tapapi "ngfw/agent/binapi/tapv2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestAliasOnHost (D-065): the alias of a slot loopback (with its creator key) and of a
// pre-existing interface no descriptor created (an untagged tap made directly through the binary
// API, standing in for a DPDK NIC) — Create verifies, Retrieve lists both, Delete touches nothing.
func TestAliasOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	ctx := context.Background()
	loopIdx, loopKey := ifacetest.Loopback(t, c, owner, 70)

	// pre-existing interface: untagged tap, created outside the scheduler
	inst := vpptest.LoopbackInstance(t, 71)
	host := vpptest.Name(t, "tap71")
	svc := tapapi.NewServiceClient(c)
	rep, err := svc.TapCreateV3(ctx, &tapapi.TapCreateV3{ID: inst, UseRandomMac: true, NumRxQueues: 1, NumTxQueues: 1,
		RxRingSz: 256, TxRingSz: 256, HostIfNameSet: true, HostIfName: host})
	if err != nil {
		t.Fatalf("tap_create_v3: %v", err)
	}
	tapIdx := uint32(rep.SwIfIndex)
	t.Cleanup(func() {
		if _, err := svc.TapDeleteV2(context.Background(), &tapapi.TapDeleteV2{SwIfIndex: interface_types.InterfaceIndex(tapIdx)}); err != nil {
			t.Errorf("cleanup tap_delete_v2: %v", err)
		}
	})

	d := iface.NewAlias(c, owner)
	loopName := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 70))
	ours := &iface.InterfaceAlias{Name: loopName, Creator: loopKey}
	pre := &iface.InterfaceAlias{Name: fmt.Sprintf("tap%d", inst)}
	for _, tc := range []struct {
		obj *iface.InterfaceAlias
		idx uint32
	}{{ours, loopIdx}, {pre, tapIdx}} {
		meta, err := d.Create(ctx, tc.obj)
		if err != nil {
			t.Fatalf("Create %s: %v", d.KeyOf(tc.obj), err)
		}
		if meta != (iface.Meta{SwIfIndex: tc.idx}) {
			t.Fatalf("Create %s meta = %+v, want sw_if_index %d", d.KeyOf(tc.obj), meta, tc.idx)
		}
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := ifacetest.Find(t, kvs, string(d.KeyOf(tc.obj)), func(kv scheduler.KV) string { return string(kv.Key) })
		if !proto.Equal(got.Value, tc.obj) || got.Meta != meta {
			t.Fatalf("Retrieve %s = %v %+v, want %v %+v", got.Key, got.Value, got.Meta, tc.obj, meta)
		}
		t.Logf("interface alias: Retrieve == desired: %s %v deps=%v meta=%+v", got.Key, got.Value, d.Dependencies(tc.obj), meta)
		// Delete is a no-op: the interface is still there
		if err := d.Delete(ctx, tc.obj, meta); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Create(ctx, tc.obj); err != nil {
			t.Fatalf("interface gone after alias Delete: %v", err)
		}
	}
	if _, err := d.Create(ctx, &iface.InterfaceAlias{Name: vpptest.Name(t, "nope")}); !errors.Is(err, iface.ErrNotFound) {
		t.Fatalf("missing interface: %v", err)
	}
	kvs, _ := d.Retrieve(ctx)
	for _, kv := range kvs {
		if kv.Key == "interface/local0" {
			t.Fatal("local0 retrieved")
		}
	}
	t.Logf("alias Delete left both interfaces in place; missing interface rejected; %d aliases retrieved (all VPP interfaces but local0)", len(kvs))
}
