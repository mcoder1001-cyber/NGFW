package ravpn

import (
	"context"
	"google.golang.org/protobuf/proto"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"testing"
)

type receiptMemory map[string]TAPReceipt

func (m receiptMemory) Load(key string) (TAPReceipt, error) {
	value, ok := m[key]
	if !ok {
		return value, os.ErrNotExist
	}
	return value, nil
}
func (m receiptMemory) Save(key string, value TAPReceipt) error { m[key] = value; return nil }
func (m receiptMemory) Remove(key string) error                 { delete(m, key); return nil }

type tapMemory struct {
	rows     []scheduler.KV
	deleted  int
	onCreate func()
}

func (*tapMemory) Name() string { return tapv2.TapName }
func (*tapMemory) KeyOf(v proto.Message) scheduler.Key {
	return scheduler.Join(tapv2.TapName, v.(*tapv2.Tap).Name)
}
func (*tapMemory) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (m *tapMemory) Create(_ context.Context, v proto.Message) (any, error) {
	m.rows = []scheduler.KV{{Key: m.KeyOf(v), Value: proto.Clone(v), Meta: iface.Meta{SwIfIndex: 19001}}}
	if m.onCreate != nil {
		m.onCreate()
	}
	return m.rows[0].Meta, nil
}
func (*tapMemory) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}
func (m *tapMemory) Delete(context.Context, proto.Message, any) error {
	m.deleted++
	m.rows = nil
	return nil
}
func (m *tapMemory) Retrieve(context.Context) ([]scheduler.KV, error) { return m.rows, nil }

func guardedFixture(t *testing.T) (*GuardedTAP, *tapMemory, *NetworkPlan, *bootid.Identity, *tapv2.Tap) {
	t.Helper()
	plan := networkFixture()
	plan.NamespaceInode = 100
	plan.HostNamespaceInode = 99
	boot := &bootid.Identity{BootID: "host-a", PID: 1000, StartTime: 2000}
	tap := &tapMemory{}
	guard := &GuardedTAP{Tap: tap, Store: receiptMemory{}, Boot: func() bootid.Identity { return *boot }, Plan: func(string) (*NetworkPlan, error) { return plan, nil }, AllowedID: func(id uint32) bool { return id >= 2432 && id <= 2433 }}
	outer, _, err := TransitTAPs(plan, 2432, 2433)
	if err != nil {
		t.Fatal(err)
	}
	return guard, tap, plan, boot, outer
}
func TestGuardedTAPRefusesStaleBootNamespaceAndRecycledIndexBeforeDelete(t *testing.T) {
	for _, change := range []func(*NetworkPlan, *bootid.Identity, *tapMemory){
		func(_ *NetworkPlan, b *bootid.Identity, _ *tapMemory) { b.PID++ },
		func(_ *NetworkPlan, b *bootid.Identity, _ *tapMemory) { b.StartTime++ },
		func(_ *NetworkPlan, b *bootid.Identity, _ *tapMemory) { b.BootID = "host-b" },
		func(p *NetworkPlan, _ *bootid.Identity, _ *tapMemory) { p.NamespaceInode++ },
		func(p *NetworkPlan, _ *bootid.Identity, _ *tapMemory) { p.HostNamespaceInode++ },
		func(_ *NetworkPlan, _ *bootid.Identity, m *tapMemory) { m.rows[0].Meta = iface.Meta{SwIfIndex: 19002} },
		func(_ *NetworkPlan, _ *bootid.Identity, m *tapMemory) {
			m.rows[0].Value.(*tapv2.Tap).HostNamespace = "/proc/1/ns/net"
		},
	} {
		guard, tap, plan, boot, endpoint := guardedFixture(t)
		meta, err := guard.Create(context.Background(), endpoint)
		if err != nil {
			t.Fatal(err)
		}
		change(plan, boot, tap)
		if guard.Delete(context.Background(), endpoint, meta) == nil || tap.deleted != 0 {
			t.Fatal("stale runtime allowed destructive VPP call")
		}
	}
}
func TestGuardedTAPOwnLifecycleAndCrashClaim(t *testing.T) {
	guard, tap, _, _, endpoint := guardedFixture(t)
	meta, err := guard.Create(context.Background(), endpoint)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := guard.Retrieve(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatal("own verified readback missing", err)
	}
	if _, err = guard.Create(context.Background(), endpoint); err == nil {
		t.Fatal("existing claim overwritten")
	}
	if err = guard.Delete(context.Background(), endpoint, meta); err != nil || tap.deleted != 1 {
		t.Fatal("own endpoint not removed", err)
	}
	if _, err = guard.Store.Load(endpoint.Name); !os.IsNotExist(err) {
		t.Fatal("owned claim remained")
	}
}
func TestGuardedTAPBootChangeDuringCreateKeepsUncertainClaim(t *testing.T) {
	guard, tap, _, boot, endpoint := guardedFixture(t)
	tap.onCreate = func() { boot.StartTime++ }
	meta, err := guard.Create(context.Background(), endpoint)
	if err == nil || !scheduler.IsPartialCreate(err) {
		t.Fatal("boot change claimed successful creation")
	}
	if guard.Delete(context.Background(), endpoint, meta) == nil || tap.deleted != 0 {
		t.Fatal("old boot index deleted")
	}
	if _, err = guard.Retrieve(context.Background()); err == nil {
		t.Fatal("pending claim adopted after restart")
	}
}

func TestGuardedTAPRetainsTransportWhenDaemonCannotQuiesce(t *testing.T) {
	guard, tap, _, _, endpoint := guardedFixture(t)
	guard.Guard = func(context.Context, *NetworkPlan) error { return ErrEngine }
	if _, err := guard.Create(context.Background(), endpoint); err == nil || len(tap.rows) != 0 || len(guard.Store.(receiptMemory)) != 0 {
		t.Fatal("failed quiesce mutated or claimed TAP")
	}
	guard.Guard = nil
	meta, err := guard.Create(context.Background(), endpoint)
	if err != nil {
		t.Fatal(err)
	}
	guard.Guard = func(context.Context, *NetworkPlan) error { return ErrEngine }
	if guard.Delete(context.Background(), endpoint, meta) == nil || tap.deleted != 0 || len(tap.rows) != 1 || len(guard.Store.(receiptMemory)) != 1 {
		t.Fatal("failed stop removed active transport or its receipt")
	}
}
