package nattest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

// Plugin describes the raw test-fixture operations on a NAT plugin (D-071: a test slot is never
// the globals owner, so the descriptors do not enable plugins for it; the test does, as a
// fixture, and restores the PREVIOUS state).
type Plugin struct {
	Name string
	// Enable enables the plugin and reports whether it was already enabled (VPP's "already
	// enabled" retval), which is how the previous state is learned.
	Enable func(ctx context.Context) (alreadyOn bool, err error)
	// Empty reports whether the plugin holds no object of ANY owner.
	Empty func(ctx context.Context) (bool, error)
	// Disable disables the plugin; nil = never disable (det44, D-068).
	Disable func(ctx context.Context) error
}

// LockDir is where the host-wide fixture locks live (tests may point it elsewhere).
var LockDir = "/run/lock"

// EnsurePlugin enables p for the test when it is off and returns whether it was on before.
// Re-review N4: every test using the plugin holds the host-wide lock
// <LockDir>/vrx-nat-fixture-<plugin>.lock SHARED for its whole lifetime; the cleanup of the test
// that enabled the plugin converts it to EXCLUSIVE around the emptiness check and the disable,
// so no other slot's test can add an object between the two (it holds the shared lock while it
// uses the plugin). Cleanup (registered first, so it runs after every object cleanup) disables
// the plugin only if this test enabled it AND the plugin is completely empty.
func EnsurePlugin(t testing.TB, p Plugin) (wasOn bool) {
	t.Helper()
	ctx := Ctx(t)
	path := filepath.Join(LockDir, "vrx-nat-fixture-"+p.Name+".lock")
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // fixed lock path
	if err != nil {
		t.Fatalf("fixture lock %s: %v", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		t.Fatalf("fixture lock %s: %v", path, err)
	}
	on, err := p.Enable(ctx)
	if err != nil {
		_ = f.Close()
		t.Fatalf("fixture: enable %s: %v", p.Name, err)
	}
	disable := !on && p.Disable != nil
	switch {
	case on:
		t.Logf("fixture: %s was already enabled (by another owner): it stays enabled", p.Name)
	case p.Disable == nil:
		t.Logf("fixture: %s enabled for this test; never disabled (D-068)", p.Name)
	default:
		t.Logf("fixture: %s enabled for this test", p.Name)
	}
	t.Cleanup(func() {
		defer func() { _ = f.Close() }() // releases the lock
		if !disable {
			return
		}
		// shared → exclusive: waits until no other test uses the plugin
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			t.Errorf("fixture: exclusive lock %s: %v", path, err)
			return
		}
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		empty, err := p.Empty(cctx)
		switch {
		case err != nil:
			t.Errorf("fixture: %s emptiness check: %v", p.Name, err)
		case !empty:
			t.Logf("fixture: %s holds objects of another owner now: left enabled (D-071)", p.Name)
		default:
			if err := p.Disable(cctx); err != nil {
				t.Errorf("fixture: restore (disable) %s: %v", p.Name, err)
			} else {
				t.Logf("fixture: %s disabled again under the exclusive fixture lock (previous state restored)", p.Name)
			}
		}
	})
	return on
}

// LoopbackOwnedBy is Loopback with the owner tag of ANOTHER owner (still inside this slot's
// namespace, e.g. "w9b"), to play a foreign owner's interface in regression tests.
func LoopbackOwnedBy(t testing.TB, c vpp.Client, i int, owner string) (name string, swIfIndex uint32) {
	t.Helper()
	ctx := Ctx(t)
	inst := vpptest.LoopbackInstance(t, i)
	svc := interfaces.NewServiceClient(c)
	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatalf("create_loopback_instance %d: %v", inst, err)
	}
	idx := rep.SwIfIndex
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ifsanitize.BeforeDelete(cctx, c, uint32(idx), "test cleanup") // D-095 c: bindings go before the interface (V19)
		if _, err := svc.DeleteLoopback(cctx, &interfaces.DeleteLoopback{SwIfIndex: idx}); err != nil {
			t.Errorf("cleanup delete_loopback %d: %v", idx, err)
		}
	})
	name = fmt.Sprintf("loop%d", inst)
	tag, err := vpp.OwnerTag(owner, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: idx, Tag: tag}); err != nil {
		t.Fatalf("sw_interface_tag_add_del: %v", err)
	}
	if _, err := svc.SwInterfaceSetFlags(ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: idx, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP}); err != nil {
		t.Fatalf("sw_interface_set_flags: %v", err)
	}
	return name, uint32(idx)
}
