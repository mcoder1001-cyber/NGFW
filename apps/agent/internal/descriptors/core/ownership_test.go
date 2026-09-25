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

// TestInterfaceObjectsRecordNoStore is a merge tripwire: interface-ip and interface-ip.table declare
// RecordsNoOwnership because this build's Env has no claim store. When an Env field for claims on
// untagged interfaces lands (TD-11c), their declaration must become CheckPersistent over it.
func TestInterfaceObjectsRecordNoStore(t *testing.T) {
	env := reflect.TypeFor[Env]()
	for i := 0; i < env.NumField(); i++ {
		if f := env.Field(i); f.Name != "Client" && f.Name != "Owner" && f.Name != "Owned" && f.Name != "IfRef" {
			t.Fatalf("core.Env gained %s (%s): if interface-ip / interface-ip.table now record ownership in it, replace their RecordsNoOwnership (ownership.go) with CheckPersistent over it (TD-11b review M1/Nit)", f.Name, f.Type)
		}
	}
}
