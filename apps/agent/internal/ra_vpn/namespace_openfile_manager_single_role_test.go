package ravpn

import (
	"context"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestNumericPublisherSingleRoleClosedFieldsetsAndInterfaces(t *testing.T) {
	for _, kind := range []managerDBusSingleRole{managerDBusSingleSource, managerDBusSingleVPP, managerDBusSinglePublisherExit} {
		role, err := fixedManagerDBusSingleRole(kind)
		if err != nil || !validManagerDBusRoles([]managerDBusRole{role}) {
			t.Fatalf("fixed role refused: %v", err)
		}
		for _, key := range strings.Split(role.fields, ",") {
			want := "org.freedesktop.systemd1.Service"
			switch key {
			case "Id", "FragmentPath", "DropInPaths", "ActiveState", "SubState":
				want = "org.freedesktop.systemd1.Unit"
			}
			if managerDBusPropertyInterface(role, key) != want {
				t.Fatalf("wrong fixed interface for %s", key)
			}
		}
		for _, mutate := range []func(*managerDBusRole){func(r *managerDBusRole) { r.name = "foreign.service" }, func(r *managerDBusRole) { r.path += "foreign" }, func(r *managerDBusRole) { r.fields += ",User" }} {
			foreign := role
			mutate(&foreign)
			called := false
			_, err := readManagerDBusProperties(context.Background(), []managerDBusRole{foreign}, func(managerDBusRole, string) (dbus.Variant, error) {
				called = true
				return dbus.MakeVariant("foreign"), nil
			})
			if err != ErrBoundary || called {
				t.Fatal("foreign role reached getter")
			}
		}
	}
	if _, err := fixedManagerDBusSingleRole(0); err != ErrBoundary {
		t.Fatal("unknown role accepted")
	}
	if validManagerDBusRoles([]managerDBusRole{managerDBusPublisherRoles[1]}) {
		t.Fatal("full publisher service accepted as single exit role")
	}
}

func TestNumericPublisherSingleRoleRejectsChangedIdentityTypesAndCancellation(t *testing.T) {
	for _, kind := range []managerDBusSingleRole{managerDBusSingleSource, managerDBusSingleVPP, managerDBusSinglePublisherExit} {
		role, err := fixedManagerDBusSingleRole(kind)
		if err != nil {
			t.Fatal(err)
		}
		reads := 0
		values, err := readManagerDBusProperties(context.Background(), []managerDBusRole{role}, func(r managerDBusRole, key string) (dbus.Variant, error) {
			reads++
			return managerDBusQueryFixture(r, key)
		})
		if err != nil || len(values) != 1 || reads != len(strings.Split(role.fields, ","))+1 || values[0]["Id"] != role.name {
			t.Fatal("fixed property sequence changed")
		}
		for _, fail := range []string{"first-id", "last-id", "type", "cancel"} {
			ctx, cancel := context.WithCancel(context.Background())
			ids := 0
			_, err := readManagerDBusProperties(ctx, []managerDBusRole{role}, func(r managerDBusRole, key string) (dbus.Variant, error) {
				if key == "Id" {
					ids++
					if fail == "first-id" || fail == "last-id" && ids == 2 {
						return dbus.MakeVariant("foreign.service"), nil
					}
				}
				if key == "MainPID" {
					if fail == "type" {
						return dbus.MakeVariant("0"), nil
					}
					if fail == "cancel" {
						cancel()
					}
				}
				return managerDBusQueryFixture(r, key)
			})
			cancel()
			if err != ErrBoundary {
				t.Fatalf("%s accepted", fail)
			}
		}
	}
}

func TestNumericPublisherSingleRoleCanceledBeforePID1Access(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, kind := range []managerDBusSingleRole{managerDBusSingleSource, managerDBusSingleVPP, managerDBusSinglePublisherExit, 0} {
		if _, err := readManagerDBusSingleRole(ctx, kind); err != ErrBoundary {
			t.Fatal("canceled or foreign call accepted")
		}
	}
}
