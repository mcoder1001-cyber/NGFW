package gso_test

import (
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/gso"
	iface "ngfw/agent/internal/descriptors/interface"
)

type failClaims struct{}

func (failClaims) Claim(string, string) error   { return errors.New("claim store down") }
func (failClaims) Release(string, string) error { return nil }
func (failClaims) Claimed(string, string) bool  { return false }

// failPut is a boot store whose records cannot be written.
type failPut struct{ dfkit.BootStore }

func (failPut) Put(dfkit.BootRecord) error { return errors.New("state dir read-only") }

// Review M3 (TD-11b claim first): GSO on an untagged interface is claimed before any VPP write; a claim that cannot be
// recorded sends nothing (the old code enabled first and claimed after).
func TestCreateClaimsFirst(t *testing.T) {
	f := newFake()
	iface.SetClaimStore(owner, failClaims{})
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	d := gso.New(f, owner, dfkit.NewMemoryBootStore())
	if _, err := d.Create(ctx, gso.Interface{Interface: "eth0"}.Proto()); err == nil {
		t.Fatal("Create succeeded without its claim")
	}
	if n := len(f.CallsNamed("feature_gso_enable_disable")); n != 0 {
		t.Fatalf("%d GSO calls before the claim", n)
	}
}

// Review M3: an enable whose applied-once record cannot be written is taken back (it would be invisible to Retrieve and
// stacked by the next Create), and the claim this Create made is released.
func TestCreateUndoesEnableWithoutRecord(t *testing.T) {
	f := newFake()
	iface.SetClaimStore(owner, nil)
	d := gso.New(f, owner, failPut{dfkit.NewMemoryBootStore()})
	if _, err := d.Create(ctx, gso.Interface{Interface: "eth0"}.Proto()); err == nil {
		t.Fatal("Create succeeded without its record")
	}
	if f.count[4] != 0 {
		t.Fatalf("GSO left enabled (%d) without a record", f.count[4])
	}
	tg, err := dfkit.ResolveTarget(ctx, f, "eth0", owner, gso.Name)
	if err != nil {
		t.Fatal(err)
	}
	if tg.Claimed() {
		t.Fatal("the claim of a failed Create was kept")
	}
}
