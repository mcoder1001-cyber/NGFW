package df7test

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/core"

	"ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// hostClient adapts *core.Connection to vpp.Client until P05's client is merged.
type hostClient struct{ *core.Connection }

func (hostClient) Connected() bool { return true }

// Host is one integration-test session against the host VPP: shared lab lock held, owner =
// VRX_TEST_PREFIX, a fresh govpp connection.
type Host struct {
	T      *testing.T
	Ctx    context.Context
	C      vpp.Client
	Owner  string
	Slot   int
	TableB uint32
}

// StartHost skips unless VRX_INTEGRATION=1, takes the shared lab lock and connects to
// /run/vpp/api.sock. Everything it creates is removed in t.Cleanup.
func StartHost(t *testing.T) *Host {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	c := Connect(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel) // registered after Connect: event watchers unregister before the disconnect
	return &Host{T: t, Ctx: ctx, C: c, Owner: vpptest.Prefix(t), Slot: vpptest.Slot(t), TableB: vpptest.TableBase(t)}
}

// Connect opens a new govpp connection (closed in Cleanup). A second connection is what the
// restart simulation uses: a fresh client with no cached Meta.
func Connect(t *testing.T) vpp.Client {
	t.Helper()
	conn, err := core.Connect(socketclient.NewVppClient(socketclient.DefaultSocketName))
	if err != nil {
		t.Fatalf("connect %s: %v", socketclient.DefaultSocketName, err)
	}
	t.Cleanup(conn.Disconnect)
	return hostClient{conn}
}

// Addr returns 10.<slot>.<a>.<b>.
func (h *Host) Addr(a, b int) string { return fmt.Sprintf("10.%d.%d.%d", h.Slot, a, b) }

// Loopback creates this slot's loopback number i (loop<slot><ii>), tags it with the owner
// (unless tagged is false), optionally sets it admin-up, and deletes it in Cleanup. A leftover
// of the same name from an earlier failed run (same slot ⇒ ours) is removed first.
func (h *Host) Loopback(i int, tagged, up bool) (string, uint32) {
	t := h.T
	t.Helper()
	inst := vpptest.LoopbackInstance(t, i)
	name := fmt.Sprintf("loop%d", inst)
	svc := interfaces.NewServiceClient(h.C)
	if idx, ok := h.IfIndex(name); ok {
		t.Logf("leftover %s (sw_if_index %d) from an earlier run: deleting", name, idx)
		if _, err := svc.DeleteLoopback(h.Ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
			t.Fatalf("delete leftover %s: %v", name, err)
		}
	}
	rep, err := svc.CreateLoopbackInstance(h.Ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatalf("create_loopback_instance %d: %v", inst, err)
	}
	t.Cleanup(func() {
		if _, err := svc.DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: rep.SwIfIndex}); err != nil {
			t.Errorf("cleanup delete_loopback %s: %v", name, err)
		}
	})
	if tagged {
		tag, err := vpp.OwnerTag(h.Owner, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SwInterfaceTagAddDel(h.Ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
			t.Fatalf("sw_interface_tag_add_del %s: %v", name, err)
		}
	}
	if up {
		if _, err := svc.SwInterfaceSetFlags(h.Ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: rep.SwIfIndex, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP}); err != nil {
			t.Fatalf("admin up %s: %v", name, err)
		}
	}
	t.Logf("created %s sw_if_index %d (tagged %v, up %v)", name, rep.SwIfIndex, tagged, up)
	return name, uint32(rep.SwIfIndex)
}

// IfIndex looks an interface up by name.
func (h *Host) IfIndex(name string) (uint32, bool) {
	h.T.Helper()
	stream, err := interfaces.NewServiceClient(h.C).SwInterfaceDump(h.Ctx, &interfaces.SwInterfaceDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		h.T.Fatalf("sw_interface_dump: %v", err)
	}
	var found uint32
	ok := false
	for {
		d, err := stream.Recv()
		if err != nil {
			break
		}
		if strings.TrimRight(d.InterfaceName, "\x00") == name {
			found, ok = uint32(d.SwIfIndex), true
		}
	}
	return found, ok
}

// Address adds addr ("10.10.1.1/24") to sw_if_index (removed with the loopback).
func (h *Host) Address(swIfIndex uint32, addr string) {
	h.T.Helper()
	p := netip.MustParsePrefix(addr)
	var a ip_types.Address
	if p.Addr().Is4() {
		a = ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(p.Addr().As4())}
	} else {
		a = ip_types.Address{Af: ip_types.ADDRESS_IP6, Un: ip_types.AddressUnionIP6(p.Addr().As16())}
	}
	req := &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(swIfIndex), IsAdd: true,
		Prefix: ip_types.AddressWithPrefix{Address: a, Len: uint8(p.Bits())}} //nolint:gosec // ≤ 128
	if _, err := interfaces.NewServiceClient(h.C).SwInterfaceAddDelAddress(h.Ctx, req); err != nil {
		h.T.Fatalf("sw_interface_add_del_address %s: %v", addr, err)
	}
	// remove the address before the interface and its table go (VPP bug V15: FIB entries left
	// in a deleted table leak into the next table that reuses its index)
	h.T.Cleanup(func() {
		// VPP keeps a per-sw_if_index "ip classify table" across interface delete/create: a
		// reused index inherits another test's binding, and every address added to it gets a
		// classify-sourced /32 that survives the address removal (seen: 10.10.82.1/32,
		// 10.10.70.1/32). Clearing the binding removes one reference of that entry per call
		// (earlier runs may have stacked several; a call without one is a no-op in VPP).
		for i := 0; i < 8; i++ {
			if _, err := classify.NewServiceClient(h.C).ClassifySetInterfaceIPTable(context.Background(), &classify.ClassifySetInterfaceIPTable{
				IsIPv6: p.Addr().Is6(), SwIfIndex: interface_types.InterfaceIndex(swIfIndex), TableIndex: ^uint32(0)}); err != nil {
				h.T.Logf("cleanup clear ip classify binding of %d: %v", swIfIndex, err)
				break
			}
		}
		del := *req
		del.IsAdd = false
		if _, err := interfaces.NewServiceClient(h.C).SwInterfaceAddDelAddress(context.Background(), &del); err != nil {
			h.T.Errorf("cleanup remove address %s: %v", addr, err)
		}
	})
}

// IPTable creates IPv4 table id (with its mFIB) named "<owner>:<id>" and deletes it in Cleanup
// (register it before the interfaces bound to it so it is removed after them).
func (h *Host) IPTable(id uint32) {
	h.T.Helper()
	svc := ip.NewServiceClient(h.C)
	tbl := ip.IPTable{TableID: id, Name: fmt.Sprintf("%s:%d", h.Owner, id)}
	if _, err := svc.IPTableAddDel(h.Ctx, &ip.IPTableAddDel{IsAdd: true, Table: tbl}); err != nil {
		h.T.Fatalf("ip_table_add_del %d: %v", id, err)
	}
	h.T.Cleanup(func() {
		if _, err := svc.IPTableAddDel(context.Background(), &ip.IPTableAddDel{IsAdd: false, Table: tbl}); err != nil {
			h.T.Errorf("cleanup ip_table_add_del %d: %v", id, err)
		}
	})
}

// BindTable puts sw_if_index into IPv4 table id.
func (h *Host) BindTable(swIfIndex, id uint32) {
	h.T.Helper()
	if _, err := interfaces.NewServiceClient(h.C).SwInterfaceSetTable(h.Ctx, &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(swIfIndex), VrfID: id}); err != nil {
		h.T.Fatalf("sw_interface_set_table %d → %d: %v", swIfIndex, id, err)
	}
	// back to table 0 before the table is deleted (V15); runs after the address cleanups
	h.T.Cleanup(func() {
		if _, err := interfaces.NewServiceClient(h.C).SwInterfaceSetTable(context.Background(), &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(swIfIndex)}); err != nil {
			h.T.Errorf("cleanup unbind %d from table %d: %v", swIfIndex, id, err)
		}
	})
}

// NoLeftovers asserts that no FIB entry of this slot's 10.<slot>.0.0/16 is left in IPv4 table
// id (ip_route_dump) — the check P05 asked for after V15.
func (h *Host) NoLeftovers(id uint32) {
	h.T.Helper()
	stream, err := ip.NewServiceClient(h.C).IPRouteDump(h.Ctx, &ip.IPRouteDump{Table: ip.IPTable{TableID: id}})
	if err != nil {
		h.T.Fatalf("ip_route_dump %d: %v", id, err)
	}
	slot := netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", h.Slot))
	var left []string
	for {
		d, err := stream.Recv()
		if err != nil {
			break
		}
		a := d.Route.Prefix.Address
		if a.Af != ip_types.ADDRESS_IP4 {
			continue
		}
		addr := netip.AddrFrom4(a.Un.GetIP4())
		if slot.Contains(addr) {
			left = append(left, fmt.Sprintf("%s/%d", addr, d.Route.Prefix.Len))
		}
	}
	if len(left) > 0 {
		for _, p := range left { // test-only diagnostic: which FIB source holds it
			if rep, err := vlib.NewServiceClient(h.C).CliInband(h.Ctx, &vlib.CliInband{Cmd: fmt.Sprintf("show ip fib table %d %s", id, p)}); err == nil {
				h.T.Logf("%s", rep.Reply)
			}
		}
		h.T.Errorf("table %d still holds %v of this slot", id, left)
	} else {
		h.T.Logf("ip_route_dump table %d: nothing of %s left", id, slot)
	}
}

// Hold pauses when VRX_DF7_EVIDENCE_HOLD (a duration) is set, so an operator can capture
// `vppctl show …` output while the objects exist.
func (h *Host) Hold(what string) {
	if v := os.Getenv("VRX_DF7_EVIDENCE_HOLD"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			h.T.Fatalf("VRX_DF7_EVIDENCE_HOLD=%q: %v", v, err)
		}
		h.T.Logf("holding %s for %s (evidence)", what, d)
		time.Sleep(d)
	}
}

// GlobalsOptIn skips t unless VRX_DF7_GLOBALS=1: a test slot on the shared host is never the
// globals owner and must not set VPP-global settings (D-071). An operator who owns the host's
// globals for the moment may opt in; the tests then restore what they can read back.
func GlobalsOptIn(t *testing.T, what string) {
	t.Helper()
	if os.Getenv("VRX_DF7_GLOBALS") != "1" {
		t.Skipf("skip: %s is VPP-global — only the globals owner sets it (D-071); VRX_DF7_GLOBALS=1 to opt in", what)
	}
}

// Must fails the test on err.
func (h *Host) Must(what string, err error) {
	h.T.Helper()
	if err != nil {
		h.T.Fatalf("%s: %v", what, err)
	}
}

// Apply creates every desired object through d (in order) and returns the KVs with Meta.
func (h *Host) Apply(d scheduler.Descriptor, desired ...scheduler.KV) []scheduler.KV {
	h.T.Helper()
	out := make([]scheduler.KV, 0, len(desired))
	for _, kv := range desired {
		meta, err := d.Create(h.Ctx, kv.Value)
		if err != nil {
			h.T.Fatalf("%s Create %s: %v", d.Name(), kv.Key, err)
		}
		h.T.Logf("created %s (meta %+v)", kv.Key, meta)
		out = append(out, scheduler.KV{Key: kv.Key, Value: kv.Value, Meta: meta})
	}
	return out
}

// DeleteAll deletes kvs through d, in reverse order.
func (h *Host) DeleteAll(d scheduler.Descriptor, kvs []scheduler.KV) {
	h.T.Helper()
	for i := len(kvs) - 1; i >= 0; i-- {
		if err := d.Delete(context.Background(), kvs[i].Value, kvs[i].Meta); err != nil {
			h.T.Errorf("%s Delete %s: %v", d.Name(), kvs[i].Key, err)
		} else {
			h.T.Logf("deleted %s", kvs[i].Key)
		}
	}
}

// CleanupOwned registers a Cleanup that deletes whatever d still reports as ours (the safety
// net after a failed assertion) and deletes leftovers of an earlier failed run right now.
func (h *Host) CleanupOwned(d scheduler.Descriptor) {
	h.T.Helper()
	sweep := func(ctx context.Context) {
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			return // write-only descriptors: nothing to sweep by Retrieve
		}
		for i := len(kvs) - 1; i >= 0; i-- {
			if err := d.Delete(ctx, kvs[i].Value, kvs[i].Meta); err != nil {
				h.T.Errorf("sweep %s: %v", kvs[i].Key, err)
			} else {
				h.T.Logf("swept %s", kvs[i].Key)
			}
		}
	}
	sweep(h.Ctx)
	h.T.Cleanup(func() { sweep(context.Background()) })
}

// ExpectRetrieved asserts d's Retrieve equals desired exactly (restart-safety and idempotency).
func (h *Host) ExpectRetrieved(d scheduler.Descriptor, desired ...scheduler.KV) []scheduler.KV {
	h.T.Helper()
	return AssertEmptyPlan(h.T, d, desired...)
}

// RestartSimulation is the FAST-MODE restart-safety check without restarting VPP: a fresh
// connection and fresh descriptors (no Meta, no process memory) retrieve the state and must
// plan nothing for desired; then the objects are deleted behind the agent's back (simulated
// loss, through the fresh descriptor's Delete — i.e. plain binapi), the plan must be exactly
// one Create per desired object, applying it recreates them, and the plan is empty again. The
// returned KVs carry the Meta of the recreated objects (for the test's own cleanup).
func (h *Host) RestartSimulation(newDesc func(c vpp.Client) scheduler.Descriptor, desired ...scheduler.KV) []scheduler.KV {
	t := h.T
	t.Helper()
	d := newDesc(Connect(t))
	t.Logf("restart simulation (%s): fresh connection, fresh descriptor", d.Name())
	actual := AssertEmptyPlan(t, d, desired...)
	for i := len(actual) - 1; i >= 0; i-- {
		if err := d.Delete(h.Ctx, actual[i].Value, actual[i].Meta); err != nil {
			t.Fatalf("simulated loss: delete %s: %v", actual[i].Key, err)
		}
	}
	after, err := d.Retrieve(h.Ctx)
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	p := DiffPlan(desired, after)
	t.Logf("%s: plan after simulated loss of %d object(s):\n%s", d.Name(), len(actual), PlanString(p))
	if len(p.Create) != len(desired) || len(p.Update)+len(p.Delete) != 0 {
		t.Fatalf("%s: after the loss the plan must recreate every desired object, got %d op(s)", d.Name(), p.Len())
	}
	out := make([]scheduler.KV, 0, len(p.Create))
	for _, kv := range p.Create {
		meta, err := d.Create(h.Ctx, kv.Value)
		if err != nil {
			t.Fatalf("%s: recreate %s: %v", d.Name(), kv.Key, err)
		}
		out = append(out, scheduler.KV{Key: kv.Key, Value: kv.Value, Meta: meta})
	}
	t.Logf("%s: recreated %d object(s)", d.Name(), len(out))
	AssertEmptyPlan(t, d, desired...)
	return out
}

// ExpectNone asserts d reports nothing of ours.
func (h *Host) ExpectNone(d scheduler.Descriptor) {
	h.T.Helper()
	kvs, err := d.Retrieve(h.Ctx)
	if err != nil {
		h.T.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	if len(kvs) != 0 {
		h.T.Fatalf("%s: %d object(s) of ours left after delete: %v", d.Name(), len(kvs), Keys(kvs))
	}
	h.T.Logf("%s: Retrieve after delete: nothing of ours", d.Name())
}
