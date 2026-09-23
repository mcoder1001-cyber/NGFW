// Package nattest is the integration-test harness of the NAT descriptors: a vpp.Client over a
// real govpp connection to /run/vpp/api.sock, prefixed loopbacks and tables that are deleted
// in t.Cleanup, a per-slot exclusive lock that serialises this slot's own packages around
// the global NAT singletons, and the plugin-loaded predicate. It follows
// docs/lab/shared-host-rules.md: everything carries the slot prefix, the shared lab lock is
// taken, nothing unprefixed is touched.
package nattest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// APISocket is VPP's binary API socket on the host (VRX_VPP_API_SOCKET overrides).
const APISocket = "/run/vpp/api.sock"

// Conn is a vpp.Client over a live *core.Connection.
type Conn struct {
	*core.Connection
}

var _ vpp.Client = (*Conn)(nil)

// Connected implements vpp.Client.
func (c *Conn) Connected() bool { return c.Connection != nil }

// CheckCompatiblity implements natcommon.CompatChecker through a short-lived API channel:
// it fails for a plugin that startup.conf does not load (npt66 on vrx-a).
func (c *Conn) CheckCompatiblity(msgs ...api.Message) error {
	ch, err := c.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.CheckCompatiblity(msgs...)
}

// PluginLoaded reports whether every message is known to this VPP.
func (c *Conn) PluginLoaded(msgs ...api.Message) bool { return c.CheckCompatiblity(msgs...) == nil }

// Connect gates on VRX_INTEGRATION, takes the shared lab lock and connects to VPP; the
// connection is closed in t.Cleanup.
func Connect(t testing.TB) *Conn {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	sock := os.Getenv("VRX_VPP_API_SOCKET")
	if sock == "" {
		sock = APISocket
	}
	conn, err := govpp.Connect(sock)
	if err != nil {
		t.Fatalf("govpp connect %s: %v", sock, err)
	}
	t.Cleanup(conn.Disconnect)
	return &Conn{Connection: conn}
}

// Ctx returns a context with a test-friendly deadline.
func Ctx(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// SlotLock takes an exclusive lock private to this slot (/run/vrx-test/w<N>/<name>.lock) so
// that this slot's own test packages, which `go test ./...` runs in parallel, do not race
// on a global NAT singleton (nat44 ED and EI are mutually exclusive). Released in Cleanup.
func SlotLock(t testing.TB, name string) {
	t.Helper()
	dir := filepath.Join("/run/vrx-test", vpptest.Prefix(t))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name+".lock")
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // lock file
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		t.Fatalf("flock -x %s: %v", path, err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	})
}

// Loopback creates loopback instance slot*100+i ("loop<N><ii>") tagged "<prefix>:loop…",
// brings it up and deletes it in Cleanup. It returns the VPP name and sw_if_index.
func Loopback(t testing.TB, c vpp.Client, i int) (name string, swIfIndex uint32) {
	t.Helper()
	ctx := Ctx(t)
	owner := vpptest.Prefix(t)
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

// Table creates IPv4 (or IPv6) table TableBase+i named "<prefix>-nat<i>" and deletes it in
// Cleanup. It returns the table id.
func Table(t testing.TB, c vpp.Client, i int, isIP6 bool) uint32 {
	t.Helper()
	ctx := Ctx(t)
	id := vpptest.TableBase(t) + uint32(i) //nolint:gosec // 0 ≤ i ≤ 999
	svc := ip.NewServiceClient(c)
	tbl := ip.IPTable{TableID: id, IsIP6: isIP6, Name: vpptest.Prefix(t) + fmt.Sprintf("-nat%d", i)}
	if _, err := svc.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: tbl}); err != nil {
		t.Fatalf("ip_table_add_del %d: %v", id, err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := svc.IPTableAddDel(cctx, &ip.IPTableAddDel{IsAdd: false, Table: tbl}); err != nil {
			t.Errorf("cleanup ip_table_add_del %d: %v", id, err)
		}
	})
	return id
}

// Addr4 returns "10.<slot>.<i>.<j>" — always inside this slot's NAT block.
func Addr4(t testing.TB, i, j int) string {
	t.Helper()
	return fmt.Sprintf("10.%d.%d.%d", vpptest.Slot(t), i, j)
}

// Addr6 returns "fd00:<slot>::<i>" — inside the slot's IPv6 test block (natcommon.Scope).
func Addr6(t testing.TB, i int) string {
	t.Helper()
	return fmt.Sprintf("fd00:%x::%x", vpptest.Slot(t), i)
}

// Prefix6 returns "fd00:<slot>:<i>::/<len>".
func Prefix6(t testing.TB, i, bits int) string {
	t.Helper()
	return fmt.Sprintf("fd00:%x:%x::/%d", vpptest.Slot(t), i, bits)
}

// Scope is the natcommon.Scope of this slot.
func Scope(t testing.TB) natcommon.Scope {
	t.Helper()
	return natcommon.ScopeFor(vpptest.Prefix(t))
}

// Drain reads a generated dump stream to io.EOF and returns the count (for shape asserts on
// session dumps that are expected to be empty).
func Drain[T any](recv func() (T, error)) (int, error) {
	n := 0
	for {
		_, err := recv()
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		n++
	}
}

// AddAddress assigns prefix to the interface and removes it in Cleanup.
func AddAddress(t testing.TB, c vpp.Client, swIfIndex uint32, prefix string) {
	t.Helper()
	ctx := Ctx(t)
	p, err := ip_types.ParseAddressWithPrefix(prefix)
	if err != nil {
		t.Fatal(err)
	}
	svc := interfaces.NewServiceClient(c)
	req := &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(swIfIndex), IsAdd: true, Prefix: p}
	if _, err := svc.SwInterfaceAddDelAddress(ctx, req); err != nil {
		t.Fatalf("sw_interface_add_del_address %s: %v", prefix, err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req.IsAdd = false
		_, _ = svc.SwInterfaceAddDelAddress(cctx, req)
	})
}

// Pause is an evidence hook for the task report: when VRX_EVIDENCE_DIR is set it writes
// <dir>/<label>.ready and waits (≤ 120 s) for <dir>/<label>.go, so an operator can run
// read-only `vppctl show …` while the test's objects exist. Without the variable it is a
// no-op; it never runs anything itself.
func Pause(t testing.TB, label string) {
	t.Helper()
	dir := os.Getenv("VRX_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	ready, goFile := filepath.Join(dir, label+".ready"), filepath.Join(dir, label+".go")
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		t.Fatalf("evidence pause: %v", err)
	}
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(goFile); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("evidence pause %s: timed out", label)
}
