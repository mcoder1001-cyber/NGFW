package pppoe

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/dfkit"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

type namespaceHostFake struct {
	leases []CarrierLease
	calls  int
}

func (h *namespaceHostFake) Provision(_ context.Context, spec ren.CarrierSpec) (CarrierLease, error) {
	h.calls++
	return namespaceTestLease(spec), nil
}
func (h *namespaceHostFake) Inventory(context.Context, string) ([]CarrierLease, error) {
	return h.leases, nil
}
func (h *namespaceHostFake) Remove(context.Context, CarrierLease) error { h.calls++; return nil }
func namespaceTestLease(spec ren.CarrierSpec) CarrierLease {
	return CarrierLease{Spec: spec, Token: spec.Token(), Generation: "0123456789abcdef0123456789abcdef", Boot: "test-boot", Namespace: []uint64{1, 2}}
}

func TestCarrierNamespaceAdmissionPrecedesProvision(t *testing.T) {
	spec, _ := ren.NewCarrierSpec("ngfw", "pppwan", "wan", 1492)
	host := &namespaceHostFake{}
	d := NewCarrierNamespace("ngfw", host)
	d.Admit = func(context.Context, ren.CarrierSpec) error { return errors.New("foreign parent") }
	if _, err := d.Create(context.Background(), dfkit.Encode(spec)); err == nil || host.calls != 0 {
		t.Fatalf("provision before admission: calls=%d err=%v", host.calls, err)
	}
	d.Admit = func(context.Context, ren.CarrierSpec) error { return nil }
	meta, err := d.Create(context.Background(), dfkit.Encode(spec))
	if err != nil || host.calls != 1 {
		t.Fatalf("provision: calls=%d err=%v", host.calls, err)
	}
	next := spec
	next.MTU = 1400
	if _, err := d.Update(context.Background(), dfkit.Encode(spec), dfkit.Encode(next), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("MTU change must rebuild consumers/namespace: %v", err)
	}
}

func TestCarrierNamespaceInventoryAndDeletionRefuseForeignLease(t *testing.T) {
	spec, _ := ren.NewCarrierSpec("ngfw", "pppwan", "wan", 1492)
	host := &namespaceHostFake{leases: []CarrierLease{namespaceTestLease(spec)}}
	d := NewCarrierNamespace("ngfw", host)
	rows, err := d.Retrieve(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Key != CarrierNamespaceKey(spec.Token()) {
		t.Fatalf("inventory %v %v", rows, err)
	}
	wrong := namespaceTestLease(spec)
	wrong.Spec.Parent = "another-wan"
	if err := d.Delete(context.Background(), dfkit.Encode(spec), wrong); err == nil || host.calls != 0 {
		t.Fatalf("accepted foreign deletion: %v", err)
	}
	host.leases = append(host.leases, host.leases[0])
	if _, err := d.Retrieve(context.Background()); err == nil {
		t.Fatal("accepted duplicate live namespace identity")
	}
	host.leases = host.leases[:1]
	host.leases[0].Namespace[1] = 0
	if _, err := d.Retrieve(context.Background()); err == nil {
		t.Fatal("accepted incomplete namespace identity")
	}
}
