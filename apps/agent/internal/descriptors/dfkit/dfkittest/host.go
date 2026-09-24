package dfkittest

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

// Host is a connection to the VPP on this host for integration tests.
type Host struct {
	Conn  *core.Connection
	Owner string
}

// Client returns the connection as a vpp.Client (P05's real client is not merged yet).
func (h *Host) Client() vpp.Client { return hostClient{h.Conn} }

type hostClient struct{ *core.Connection }

func (hostClient) Connected() bool { return true }

// ConnectHost gates on VRX_INTEGRATION=1, takes the shared lab lock and connects to
// /run/vpp/api.sock. Everything is released in t.Cleanup.
func ConnectHost(t *testing.T) *Host {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	conn, err := core.Connect(socketclient.NewVppClient(socketclient.DefaultSocketName))
	if err != nil {
		t.Fatalf("connect %s: %v", socketclient.DefaultSocketName, err)
	}
	t.Cleanup(conn.Disconnect)
	return &Host{Conn: conn, Owner: owner}
}

// SkipUnlessCompatible skips the test when VPP does not know the messages (plugin not loaded,
// or a CRC mismatch): the skip-unless-plugin-loaded rule of docs/lab/shared-host-rules.md.
func (h *Host) SkipUnlessCompatible(t *testing.T, plugin string, msgs ...api.Message) {
	t.Helper()
	ch, err := h.Conn.NewAPIChannel()
	if err != nil {
		t.Fatalf("api channel: %v", err)
	}
	defer ch.Close()
	if err := ch.CheckCompatiblity(msgs...); err != nil {
		t.Skipf("plugin not loaded: %s (%v)", plugin, err)
	}
	t.Logf("plugin %s loaded: %d message(s) compatible", plugin, len(msgs))
}

// Loopback creates this slot's loopback number i (loop<slot><ii>), tags it with the owner and
// deletes it in Cleanup. A leftover of the same name from an earlier failed run is deleted first.
func (h *Host) Loopback(t *testing.T, i int) (string, uint32) {
	t.Helper()
	return h.loopback(t, i, true)
}

// UntaggedLoopback is Loopback without the owner tag: this slot's stand-in for a physical,
// untagged NIC (its number is still in the slot's range; deleted in Cleanup).
func (h *Host) UntaggedLoopback(t *testing.T, i int) (string, uint32) {
	t.Helper()
	return h.loopback(t, i, false)
}

func (h *Host) loopback(t *testing.T, i int, tagged bool) (string, uint32) {
	t.Helper()
	ctx := context.Background()
	c := h.Client()
	inst := vpptest.LoopbackInstance(t, i)
	name := fmt.Sprintf("loop%d", inst)
	svc := interfaces.NewServiceClient(c)
	if tbl, err := dfkit.DumpInterfaces(ctx, c, h.Owner); err == nil {
		if old, ok := tbl.ByName[name]; ok {
			t.Logf("leftover %s (sw_if_index %d, tag %q): deleting", name, old.Index, old.Tag)
			_ = ifsanitize.BeforeDelete(ctx, c, uint32(interfaceIndex(old.Index)), "test cleanup") // D-095 c: bindings go before the interface (V19)
			if _, err := svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interfaceIndex(old.Index)}); err != nil {
				t.Fatalf("delete leftover %s: %v", name, err)
			}
		}
	}
	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatalf("create_loopback_instance %d: %v", inst, err)
	}
	t.Cleanup(func() {
		_ = ifsanitize.BeforeDelete(context.Background(), c, uint32(rep.SwIfIndex), "test cleanup") // D-095 c: bindings go before the interface (V19)
		if _, err := svc.DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: rep.SwIfIndex}); err != nil {
			t.Errorf("cleanup delete_loopback %s: %v", name, err)
		}
	})
	if !tagged {
		t.Logf("created %s sw_if_index %d (untagged)", name, rep.SwIfIndex)
		return name, uint32(rep.SwIfIndex)
	}
	tag, err := vpp.OwnerTag(h.Owner, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
		t.Fatalf("sw_interface_tag_add_del %s: %v", name, err)
	}
	t.Logf("created %s sw_if_index %d tag %q", name, rep.SwIfIndex, tag)
	return name, uint32(rep.SwIfIndex)
}

// HoldForEvidence pauses while VRX_DF8_EVIDENCE_HOLD (a duration) is set, so an operator can
// capture the VPP CLI `show …` output for the status report while the test objects exist.
func HoldForEvidence(t *testing.T, what string) {
	t.Helper()
	v := os.Getenv("VRX_DF8_EVIDENCE_HOLD")
	if v == "" {
		return
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		t.Fatalf("VRX_DF8_EVIDENCE_HOLD=%q: %v", v, err)
	}
	t.Logf("EVIDENCE HOLD %s: %s", d, what)
	time.Sleep(d)
}

func interfaceIndex(i uint32) interface_types.InterfaceIndex {
	return interface_types.InterfaceIndex(i)
}

// GlobalsLock is the lab-wide lock of tests that read-then-set VPP-global singletons with a
// getter (flowprobe params, sflow globals, IPFIX exporter 0, lcp default netns): exclusive for the
// whole test, so the read-first/skip check and the set cannot interleave with another slot that
// uses the same lock (review M3; the lock file is a proposal for the manager, DF-8-questions Q9).
const GlobalsLock = "/run/lock/vrx-globals.lock"

// LockGlobals takes GlobalsLock exclusively until the test ends. It also serialises this slot's
// own packages, which `go test ./...` runs in parallel.
func (h *Host) LockGlobals(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile(GlobalsLock, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // shared lock file, no content
	if err != nil {
		t.Fatalf("globals lock: %v", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		t.Fatalf("flock -x %s: %v", GlobalsLock, err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	})
}

// SkipUnlessGlobals skips a test that changes a getter-less VPP-global (its previous value cannot
// be restored) unless VRX_DF8_GLOBALS=1 — run only in a manager window (review M3, D-077).
func SkipUnlessGlobals(t *testing.T, what string) {
	t.Helper()
	if os.Getenv("VRX_DF8_GLOBALS") != "1" {
		t.Skipf("%s changes a getter-less VPP-global; set VRX_DF8_GLOBALS=1 in a manager window", what)
	}
}
