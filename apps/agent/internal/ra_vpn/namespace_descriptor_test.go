package ravpn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestNamespaceDescriptorPublicContractOwnership(t *testing.T) {
	plan := networkFixture()
	value, err := NamespaceValue(plan)
	if err != nil {
		t.Fatal(err)
	}
	owner := NewNamespaceDescriptor(plan.Owner)
	if _, err = owner.input(value); err != nil {
		t.Fatal(err)
	}
	if _, err = NewNamespaceDescriptor("foreign").input(value); err == nil {
		t.Fatal("foreign ownership accepted")
	}
	value.Fields["shell"] = structpb.NewStringValue("sh")
	if _, err = owner.input(value); err == nil {
		t.Fatal("unknown helper input accepted")
	}
	plan.Instance = InstanceID("foreign", plan.Profile)
	if _, err = NamespaceValue(plan); err == nil {
		t.Fatal("forged instance ownership accepted")
	}
}

func TestIntegrationNamespaceDescriptorRestartReadbackAndRollback(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires disposable namespace fixture")
	}
	plan := networkFixture()
	plan.Owner = "w19-descriptor"
	plan.Profile = strconv.Itoa(os.Getpid())
	plan.Instance = InstanceID(plan.Owner, plan.Profile)
	value, err := NamespaceValue(plan)
	if err != nil {
		t.Fatal(err)
	}
	d := NewNamespaceDescriptor(plan.Owner)
	meta, err := d.Create(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			if err := d.Delete(context.Background(), value, meta); err != nil {
				t.Error(err)
			}
		}
	})
	runtime, err := ReadAgentPlan(plan.Instance)
	if err != nil || runtime.NamespaceInode == 0 || runtime.HostNamespaceInode == 0 || runtime.NamespaceInode == runtime.HostNamespaceInode {
		t.Fatal("trusted agent did not verify namespace pair", err)
	}
	manifest := filepath.Join(InstanceRoot, plan.Instance, "network.json")
	original, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	runtime.HostNamespaceInode++
	changed, _ := json.Marshal(runtime)
	if os.WriteFile(manifest, changed, 0600) != nil {
		t.Fatal("fixture manifest write")
	}
	_, refusal := ReadAgentPlan(plan.Instance)
	// #nosec G703 -- manifest is derived from the validated test-only full-instance plan; restores the exact fixture bytes after corruption.
	if os.WriteFile(manifest, original, 0600) != nil {
		t.Fatal("fixture manifest restore")
	}
	if refusal == nil {
		t.Fatal("replaced host namespace pair accepted")
	}
	// A fresh descriptor has no memory of Create and must recover real binding.
	actual, err := NewNamespaceDescriptor(plan.Owner).Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 || !proto.Equal(actual[0].Value, value) || actual[0].Meta != meta {
		t.Fatal("restart readback lost input or binding identity")
	}
	foreign, err := NewNamespaceDescriptor("foreign").Retrieve(context.Background())
	if err != nil || len(foreign) != 0 {
		t.Fatal("foreign namespace adopted")
	}
	if err = d.Delete(context.Background(), value, actual[0].Meta); err != nil {
		t.Fatal(err)
	}
	deleted = true
	actual, err = d.Retrieve(context.Background())
	if err != nil || len(actual) != 0 {
		t.Fatal("rollback left owned namespace")
	}
}

func TestIntegrationCancelledNamespaceCreationLeavesNoBinding(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires disposable namespace fixture")
	}
	plan := networkFixture()
	plan.Owner = "w19-cancelled"
	plan.Profile = strconv.Itoa(os.Getpid())
	plan.Instance = InstanceID(plan.Owner, plan.Profile)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if CreateNamespace(ctx, plan) == nil {
		t.Fatal("cancelled namespace creation succeeded")
	}
	if plan.NamespaceInode != 0 {
		t.Fatal("cancelled namespace retained binding identity")
	}
	if _, err := os.Stat(filepath.Join(InstanceRoot, plan.Instance)); !os.IsNotExist(err) {
		t.Fatal("cancelled creation left runtime directory")
	}
}

func TestNamespaceDescriptorTrustedInventoryIsolation(t *testing.T) {
	plan := networkFixture()
	plan.NamespaceInode = 123
	plan.HostNamespaceInode = 456
	descriptor := NewNamespaceDescriptor(plan.Owner)
	descriptor.Inventory = func(_ context.Context, owner string) ([]*NetworkPlan, error) {
		if owner != plan.Owner {
			t.Fatal("inventory owner mismatch")
		}
		return []*NetworkPlan{plan}, nil
	}
	values, err := descriptor.Retrieve(context.Background())
	if err != nil || len(values) != 1 {
		t.Fatalf("trusted observed fixture inventory: %v", err)
	}
	if plan.NamespaceInode != 123 || plan.HostNamespaceInode != 456 {
		t.Fatal("mutated inventory input")
	}
	descriptor.Inventory = func(context.Context, string) ([]*NetworkPlan, error) { return []*NetworkPlan{plan, plan}, nil }
	if _, err := descriptor.Retrieve(context.Background()); err == nil {
		t.Fatal("accepted duplicate instance")
	}
	plan.HostNamespaceInode = 123
	descriptor.Inventory = func(context.Context, string) ([]*NetworkPlan, error) { return []*NetworkPlan{plan}, nil }
	if _, err := descriptor.Retrieve(context.Background()); err == nil {
		t.Fatal("accepted host/private namespace alias")
	}
	descriptor.Inventory = func(context.Context, string) ([]*NetworkPlan, error) { return nil, nil }
	values, err = descriptor.Retrieve(context.Background())
	if err != nil || len(values) != 0 {
		t.Fatal("empty mock inventory touched shared host")
	}
}
