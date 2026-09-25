package sr_mpls_test

// F-mpls-srmpls gap test (TD-11b): every SR-MPLS descriptor declares how it records ownership and
// requires a persisted claim store keyed by id — DF-6's EndpointColorDescriptor declared nothing, so
// registering the family in the product agent refused to start.

import (
	"errors"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/sr_mpls"
	"ngfw/agent/internal/scheduler"
)

// ifaceBound stands for subsystems.IfaceClaims: persisted, but it binds every claim to an interface.
type ifaceBound struct{ iface.ClaimStore }

func (ifaceBound) Persistent() bool          { return true }
func (ifaceBound) BindsInterfaceIndex() bool { return true }

func TestOwnershipDeclarations(t *testing.T) {
	f := newFake()
	check := func(opts ...df6.Option) (declared []error, checked []error) {
		r := scheduler.NewRegistry()
		sr_mpls.Register(r, f, "w5", opts...)
		if r.Len() != 3 {
			t.Fatal(r.Names())
		}
		for _, n := range r.Names() {
			d, _ := r.Get(n)
			if err := persist.Declared(d); err != nil {
				declared = append(declared, err)
			}
			if err := persist.Check(d); err != nil {
				checked = append(checked, err)
			}
		}
		return declared, checked
	}
	if declared, checked := check(); len(declared) != 0 || len(checked) != 3 {
		t.Fatalf("in-memory claims: undeclared %v; every descriptor must refuse them, got %d: %v", declared, len(checked), checked)
	}
	if _, checked := check(df6.WithClaims(ifaceBound{iface.NewMemoryClaimStore()})); len(checked) != 3 || !errors.Is(checked[0], df6.ErrClaimStoreKind) {
		t.Fatalf("interface-bound store must be refused: %v", checked)
	}
	store, err := df6.OpenFileClaimStore(filepath.Join(t.TempDir(), "claims.json"))
	if err != nil {
		t.Fatal(err)
	}
	if declared, checked := check(df6.WithClaims(store)); len(declared) != 0 || len(checked) != 0 {
		t.Fatalf("persisted keyed store: %v %v", declared, checked)
	}
}
