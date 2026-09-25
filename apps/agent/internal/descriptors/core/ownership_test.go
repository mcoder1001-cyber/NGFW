package core

import (
	"errors"
	"reflect"
	"testing"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

// TestOwnershipDeclared (TD-11b fix round 1, review M1): every core descriptor declares how it
// records ownership; the route owner table must be persisted.
func TestOwnershipDeclared(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, Env{Owner: "w1", Owned: ownertable.NewMemory()})
	for _, d := range reg.Descriptors() {
		if err := persist.Declared(d); err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}
	}
	d, _ := reg.Get(RouteName)
	if err := persist.Check(d); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("in-memory owner table: %v", err)
	}
	f, err := ownertable.Open(t.TempDir(), "w1")
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.Check(&RouteDescriptor{Env{Owned: f}}); err != nil {
		t.Fatal(err)
	}
}

// TestInterfaceObjectsRecordNoStore is a merge tripwire: a new Env field may be a new ownership store,
// and the descriptors that record ownership in it must declare CheckPersistent over it. Claims
// (TD-11c) is covered by TestInterfaceObjectsRequirePersistedClaims.
func TestInterfaceObjectsRecordNoStore(t *testing.T) {
	env := reflect.TypeFor[Env]()
	for i := 0; i < env.NumField(); i++ {
		if f := env.Field(i); f.Name != "Client" && f.Name != "Owner" && f.Name != "Owned" && f.Name != "IfRef" && f.Name != "Claims" {
			t.Fatalf("core.Env gained %s (%s): if interface-ip / interface-ip.table now record ownership in it, replace their RecordsNoOwnership (ownership.go) with CheckPersistent over it (TD-11b review M1/Nit)", f.Name, f.Type)
		}
	}
}

// persistedClaims is a ClaimStore that says it survives an agent restart (subsystems.IfaceClaims).
type persistedClaims struct{ memClaimStore }

func (persistedClaims) Persistent() bool { return true }

// memClaimStore is a volatile ClaimStore.
type memClaimStore struct{}

func (memClaimStore) Claim(string, string) error   { return nil }
func (memClaimStore) Release(string, string) error { return nil }
func (memClaimStore) Claimed(string, string) bool  { return false }

// TestInterfaceObjectsRequirePersistedClaims (TD-11c, TD-11b verify O1): interface-ip and
// interface-ip.table record ownership of untagged interfaces in Env.Claims, so the product guard
// refuses them without a persisted claim store — none, or an in-memory one.
func TestInterfaceObjectsRequirePersistedClaims(t *testing.T) {
	for _, tc := range []struct {
		claims ClaimStore
		ok     bool
	}{{nil, false}, {memClaimStore{}, false}, {persistedClaims{}, true}} {
		reg := scheduler.NewRegistry()
		Register(reg, Env{Owner: "w1", Owned: ownertable.NewMemory(), Claims: tc.claims})
		for _, name := range []string{InterfaceTableName, InterfaceAddrName} {
			d, _ := reg.Get(name)
			if err := persist.Declared(d); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			err := persist.Check(d)
			if tc.ok != (err == nil) || (!tc.ok && !errors.Is(err, persist.ErrVolatile)) {
				t.Fatalf("%s with claims %T: %v", name, tc.claims, err)
			}
		}
	}
}
