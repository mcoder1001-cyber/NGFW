package pppoe

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"ngfw/agent/internal/descriptors/dfkit"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

// Only the transport/daemon operations are recording stand-ins. The namespace
// descriptor, retrieved repair marker, scheduler planning, and recreation are real.
type carrierRecoveryGraph struct {
	mu       sync.Mutex
	objects  map[scheduler.Key]scheduler.KV
	lease    *CarrierLease
	sequence uint64
	events   []string
}

func (g *carrierRecoveryGraph) Provision(_ context.Context, spec ren.CarrierSpec) (CarrierLease, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lease != nil {
		return CarrierLease{}, fmt.Errorf("old namespace still present")
	}
	g.sequence++
	lease := CarrierLease{Spec: spec, Token: spec.Token(), Generation: fmt.Sprintf("%032x", g.sequence), Boot: "graph-test-boot", Namespace: []uint64{1, 100 + g.sequence}}
	g.lease = &lease
	g.events = append(g.events, "namespace-create:"+lease.Generation)
	return lease, nil
}
func (g *carrierRecoveryGraph) Inventory(context.Context, string) ([]CarrierLease, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lease == nil {
		return nil, nil
	}
	return []CarrierLease{*g.lease}, nil
}
func (g *carrierRecoveryGraph) Remove(_ context.Context, lease CarrierLease) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lease == nil || !reflect.DeepEqual(*g.lease, lease) {
		return fmt.Errorf("removal did not use exact observed generation/inode receipt")
	}
	for key := range g.objects {
		if key.Descriptor() != "interface" {
			return fmt.Errorf("namespace removed before consumer %s", key)
		}
	}
	g.events = append(g.events, "namespace-delete:"+lease.Generation)
	g.lease = nil
	return nil
}

type carrierRecoveryNode struct {
	name  string
	graph *carrierRecoveryGraph
	deps  []scheduler.Dependency
}

func (d *carrierRecoveryNode) Name() string { return d.name }
func (d *carrierRecoveryNode) KeyOf(v proto.Message) scheduler.Key {
	return scheduler.Join(d.name, v.(*structpb.Struct).Fields["name"].GetStringValue())
}
func (d *carrierRecoveryNode) Dependencies(proto.Message) []scheduler.Dependency { return d.deps }
func (d *carrierRecoveryNode) Create(_ context.Context, v proto.Message) (any, error) {
	g := d.graph
	g.mu.Lock()
	defer g.mu.Unlock()
	key := d.KeyOf(v)
	generation := "physical"
	if d.name != "interface" {
		if g.lease == nil || g.lease.RepairRequired {
			return nil, fmt.Errorf("consumer created before healthy namespace")
		}
		generation = g.lease.Generation
	}
	for _, dep := range d.deps {
		if dep.Key.Descriptor() == CarrierNamespaceName {
			continue
		}
		if _, ok := g.objects[dep.Key]; !ok {
			return nil, fmt.Errorf("dependency %s absent", dep.Key)
		}
	}
	g.objects[key] = scheduler.KV{Key: key, Value: proto.Clone(v), Meta: generation}
	g.events = append(g.events, "create:"+string(key)+":"+generation)
	return generation, nil
}
func (d *carrierRecoveryNode) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}
func (d *carrierRecoveryNode) Delete(_ context.Context, v proto.Message, meta any) error {
	g := d.graph
	g.mu.Lock()
	defer g.mu.Unlock()
	key := d.KeyOf(v)
	if d.name == "recovery.tap" {
		if _, ok := g.objects["recovery.daemon/pppwan"]; ok {
			return fmt.Errorf("TAP deletion preceded daemon stop")
		}
	}
	old, ok := g.objects[key]
	if !ok || old.Meta != meta {
		return fmt.Errorf("stale consumer deletion %s", key)
	}
	g.events = append(g.events, "delete:"+string(key)+":"+meta.(string))
	delete(g.objects, key)
	return nil
}
func (d *carrierRecoveryNode) Retrieve(context.Context) ([]scheduler.KV, error) {
	g := d.graph
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []scheduler.KV
	for key, row := range g.objects {
		if key.Descriptor() == d.name {
			out = append(out, row)
		}
	}
	return out, nil
}
func TestCarrierRecoveryGraphTapLossRecreatesNamespaceBeforeConsumers(t *testing.T) {
	testCarrierRecoveryGraph(t, true)
}
func TestCarrierRecoveryGraphHostTapLossWithVPPInventory(t *testing.T) {
	testCarrierRecoveryGraph(t, false)
}
func testCarrierRecoveryGraph(t *testing.T, absentVPP bool) {
	spec, err := ren.NewCarrierSpec("ngfw", "pppwan", "wan", 1492)
	if err != nil {
		t.Fatal(err)
	}
	g := &carrierRecoveryGraph{objects: map[scheduler.Key]scheduler.KV{}}
	ns := NewCarrierNamespace("ngfw", g)
	ns.Admit = func(context.Context, ren.CarrierSpec) error { return nil }
	reg := scheduler.NewRegistry()
	reg.Register(ns)
	reg.Register(&carrierRecoveryNode{name: "interface", graph: g})
	reg.Register(&carrierRecoveryNode{name: "recovery.tap", graph: g, deps: []scheduler.Dependency{{Key: CarrierNamespaceKey(spec.Token())}}})
	reg.Register(&carrierRecoveryNode{name: "recovery.daemon", graph: g, deps: []scheduler.Dependency{{Key: "recovery.tap/raw"}, {Key: "recovery.tap/transit"}}})
	obj := func(name string) *structpb.Struct { return dfkit.Encode(map[string]string{"name": name}) }
	desired := []scheduler.KV{{Key: "recovery.daemon/pppwan", Value: obj("pppwan")}, {Key: "recovery.tap/transit", Value: obj("transit")}, {Key: "recovery.tap/raw", Value: obj("raw")}, {Key: CarrierNamespaceKey(spec.Token()), Value: dfkit.Encode(spec)}, {Key: "interface/wan", Value: obj("wan")}}
	engine := scheduler.New(reg, nil)
	engine.VerifyRetries = 0
	if result := engine.Apply(t.Context(), desired, nil); result.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("initial apply: %+v", result)
	}
	g.mu.Lock()
	old := *g.lease
	g.lease.RepairRequired = true
	if absentVPP {
		delete(g.objects, "recovery.tap/raw")
	}
	g.events = nil
	g.mu.Unlock()
	// A fresh scheduler deliberately loses its in-memory graph: repair must be
	// driven by actual descriptor inventory and the retrieved-only marker.
	engine = scheduler.New(reg, nil)
	engine.VerifyRetries = 0
	if result := engine.Apply(t.Context(), desired, nil); result.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("repair: %+v events=%v", result, g.events)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lease == nil || g.lease.Generation == old.Generation || g.lease.Namespace[1] == old.Namespace[1] || g.lease.RepairRequired {
		t.Fatalf("namespace generation was reused: %+v", g.lease)
	}
	expected := []string{"delete:recovery.daemon/pppwan:" + old.Generation, "delete:recovery.tap/transit:" + old.Generation, "namespace-delete:" + old.Generation, "namespace-create:" + g.lease.Generation, "create:recovery.tap/raw:" + g.lease.Generation, "create:recovery.tap/transit:" + g.lease.Generation, "create:recovery.daemon/pppwan:" + g.lease.Generation}
	if !absentVPP {
		expected = append(expected[:2], append([]string{"delete:recovery.tap/raw:" + old.Generation}, expected[2:]...)...)
	}
	if !reflect.DeepEqual(g.events, expected) {
		t.Fatalf("recovery order\ngot  %v\nwant %v", g.events, expected)
	}
	for key, row := range g.objects {
		if key.Descriptor() != "interface" && row.Meta != g.lease.Generation {
			t.Fatalf("stale consumer generation: %s=%v", key, row.Meta)
		}
	}
}
