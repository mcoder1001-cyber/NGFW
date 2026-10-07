package ravpn

import (
	"context"
	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
	"testing"
)

func managerDBusQueryFixture(role managerDBusRole, key string) (dbus.Variant, error) {
	var value any
	switch key {
	case "Id":
		value = role.name
	case "MainPID", "ControlPID":
		value = uint32(0)
	case "CapabilityBoundingSet":
		value = uint64(0)
	case "NoNewPrivileges":
		value = true
	case "DropInPaths":
		value = []string{}
	case "Listen":
		value = []managerDBusListen{{"SequentialPacket", numericPublisherSocketPath}}
	case "ExecStart":
		value = []managerDBusExec{{Path: unitObserverExecutable, Arguments: []string{unitObserverExecutable, "--publish-openfile"}}}
	default:
		value = "owned-public-marker"
	}
	return dbus.MakeVariant(value), nil
}

func TestNumericPublisherManagerDBusFixedQueryChecksEveryRoleBeforeAndAfter(t *testing.T) {
	roles := append(append([]managerDBusRole(nil), managerDBusPublisherRoles...), managerDBusSourceRole)
	ids := map[string]int{}
	values, err := readManagerDBusProperties(context.Background(), roles, func(role managerDBusRole, key string) (dbus.Variant, error) {
		if key == "Id" {
			ids[role.name]++
		}
		return managerDBusQueryFixture(role, key)
	})
	if err != nil || len(values) != 3 {
		t.Fatal("fixed native conversion failed", err)
	}
	for _, role := range roles {
		if ids[role.name] != 2 {
			t.Fatal("role identity was not reread", role.name)
		}
	}
	for _, failure := range []string{"wrong-first-id", "replaced-last-id", "bad-type", "cancel", "missing"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			idCalls := 0
			_, err := readManagerDBusProperties(ctx, roles, func(role managerDBusRole, key string) (dbus.Variant, error) {
				calls++
				if key == "Id" {
					idCalls++
				}
				if failure == "wrong-first-id" || failure == "replaced-last-id" && idCalls == 2 {
					return dbus.MakeVariant("foreign.service"), nil
				}
				if failure == "bad-type" && key == "MainPID" {
					return dbus.MakeVariant("0"), nil
				}
				if failure == "cancel" {
					cancel()
				}
				if failure == "missing" {
					return dbus.Variant{}, ErrBoundary
				}
				return managerDBusQueryFixture(role, key)
			})
			if err != ErrBoundary || calls == 0 {
				t.Fatal("malformed/interrupted role read accepted", err)
			}
		})
	}
	calls := 0
	if _, err := readManagerDBusProperties(context.Background(), []managerDBusRole{managerDBusSourceRole, managerDBusSourceRole}, func(role managerDBusRole, key string) (dbus.Variant, error) {
		calls++
		return managerDBusQueryFixture(role, key)
	}); err != ErrBoundary || calls != 0 {
		t.Fatal("foreign role reached getter")
	}
}

func TestNumericPublisherManagerDBusProtectedMetadataAndReplacement(t *testing.T) {
	base := unix.Stat_t{Mode: unix.S_IFSOCK | 0700, Uid: 0, Gid: 0, Nlink: 1, Dev: 1, Ino: 2}
	if !managerDBusProtectedStamp(base, true) {
		t.Fatal("actual root-exclusive0700 socket mode refused")
	}
	for _, change := range []func(*unix.Stat_t){func(s *unix.Stat_t) { s.Uid = 1 }, func(s *unix.Stat_t) { s.Gid = 1 }, func(s *unix.Stat_t) { s.Mode = unix.S_IFSOCK | 0660 }, func(s *unix.Stat_t) { s.Mode |= 04000 }, func(s *unix.Stat_t) { s.Mode = unix.S_IFLNK | 0600 }, func(s *unix.Stat_t) { s.Nlink = 2 }} {
		changed := base
		change(&changed)
		if managerDBusProtectedStamp(changed, true) {
			t.Fatal("unprotected socket metadata accepted")
		}
	}
	for _, change := range []func(*unix.Stat_t){func(s *unix.Stat_t) { s.Ino++ }, func(s *unix.Stat_t) { s.Dev++ }, func(s *unix.Stat_t) { s.Gid++ }, func(s *unix.Stat_t) { s.Ctim.Sec++ }, func(s *unix.Stat_t) { s.Mtim.Sec++ }, func(s *unix.Stat_t) { s.Nlink++ }} {
		changed := base
		change(&changed)
		if managerDBusSameStamp(base, changed) {
			t.Fatal("socket substitution/change accepted")
		}
	}
	dir := base
	dir.Mode = unix.S_IFDIR | 0755
	childMutation := dir
	childMutation.Ctim.Sec++
	childMutation.Mtim.Sec++
	childMutation.Nlink++
	if !managerDBusProtectedStamp(dir, false) || !managerDBusSameStamp(dir, childMutation) {
		t.Fatal("legitimate protected parent child mutation refused")
	}
	childMutation.Ino++
	if managerDBusSameStamp(dir, childMutation) {
		t.Fatal("replaced protected parent accepted")
	}
}
