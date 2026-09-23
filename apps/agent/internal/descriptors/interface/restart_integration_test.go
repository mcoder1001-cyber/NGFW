package iface_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l3xcapi "ngfw/agent/binapi/l3xc"
	memifapi "ngfw/agent/binapi/memif"
	tapapi "ngfw/agent/binapi/tapv2"

	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/descriptors/l3xc"
	"ngfw/agent/internal/descriptors/memif"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// descriptorOf maps a desired value to the descriptor name that owns its type.
func descriptorOf(o proto.Message) string {
	switch o.(type) {
	case *tapv2.Tap:
		return tapv2.TapName
	case *afpacket.HostInterface:
		return afpacket.HostInterfaceName
	case *bond.Bond:
		return bond.BondName
	case *bond.Member:
		return bond.MemberName
	case *memif.Socket:
		return memif.SocketName
	case *memif.Memif:
		return memif.MemifName
	case *iface.Subinterface:
		return iface.SubinterfaceName
	case *iface.AdminState:
		return iface.AdminStateName
	case *iface.Mtu:
		return iface.MtuName
	case *iface.MacAddress:
		return iface.MacAddressName
	case *iface.Promisc:
		return iface.PromiscName
	case *iface.RxMode:
		return iface.RxModeName
	case *iface.InterfaceAlias:
		return iface.AliasName
	case *iface.RxPlacement:
		return iface.RxPlacementName
	case *l2.BridgeDomain:
		return l2.BridgeDomainName
	case *l2.BridgeDomainMember:
		return l2.MemberName
	case *l2.FibEntry:
		return l2.FibEntryName
	case *l2.Flags:
		return l2.FlagsName
	case *l2.VlanTagRewrite:
		return l2.VlanTagRewriteName
	case *l2.Xconnect:
		return l2.XconnectName
	case *l3xc.L3Xc:
		return l3xc.L3xcName
	}
	return ""
}

// registerAll registers every DF-1 descriptor, interface creators first (registration order is
// the scheduler's tie-breaker).
func registerAll(r scheduler.Registry, c vpp.Client, owner, memifDir string) {
	tapv2.Register(r, c, owner)
	afpacket.Register(r, c, owner)
	bond.Register(r, c, owner)
	memif.Register(r, c, owner, memif.WithSocketDir(memifDir))
	iface.Register(r, c, owner)
	l2.Register(r, c, owner)
	l3xc.Register(r, c, owner)
}

// topo orders kvs dependency-first (Kahn; ties by input order). Dependencies outside the set
// (P05's loopback, which the test creates directly) count as satisfied.
func topo(t *testing.T, r *scheduler.MapRegistry, kvs []scheduler.KV) []scheduler.KV {
	t.Helper()
	in := map[scheduler.Key]bool{}
	for _, kv := range kvs {
		in[kv.Key] = true
	}
	done := map[scheduler.Key]bool{}
	var out []scheduler.KV
	for len(out) < len(kvs) {
		progress := false
		for _, kv := range kvs {
			if done[kv.Key] {
				continue
			}
			d, _ := r.ForKey(kv.Key)
			ready := true
			for _, dep := range d.Dependencies(kv.Value) {
				if in[dep.Key] && !done[dep.Key] {
					ready = false
				}
			}
			if ready {
				done[kv.Key] = true
				out = append(out, kv)
				progress = true
			}
		}
		if !progress {
			t.Fatal("dependency cycle in the desired state")
		}
	}
	return out
}

// retrieveAll is the union of Retrieve over every registered descriptor.
func retrieveAll(t *testing.T, r *scheduler.MapRegistry) map[scheduler.Key]scheduler.KV {
	t.Helper()
	out := map[scheduler.Key]scheduler.KV{}
	for _, d := range r.Descriptors() {
		kvs, err := d.Retrieve(context.Background())
		if err != nil {
			t.Fatalf("%s Retrieve: %v", d.Name(), err)
		}
		for _, kv := range kvs {
			out[kv.Key] = kv
		}
	}
	return out
}

// observeOnly mirrors P05's scheduler.AbsenceDeleter (review H1).
type observeOnly interface{ DeleteOnAbsence() bool }

// plan is the scheduler's diff (descriptor.go "Transaction semantics" 3, plus P05's
// DeleteOnAbsence opt-out) as strings; desired values must already be normalised.
func plan(r *scheduler.MapRegistry, desired []scheduler.KV, actual map[scheduler.Key]scheduler.KV) []string {
	var ops []string
	want := map[scheduler.Key]bool{}
	for _, kv := range desired {
		want[kv.Key] = true
		got, ok := actual[kv.Key]
		switch {
		case !ok:
			ops = append(ops, "create "+string(kv.Key))
		case !proto.Equal(got.Value, kv.Value):
			ops = append(ops, fmt.Sprintf("update %s: actual %v", kv.Key, got.Value))
		}
	}
	for k, kv := range actual {
		if d, ok := r.ForKey(k); ok {
			if o, ok := d.(observeOnly); ok && !o.DeleteOnAbsence() {
				continue
			}
		}
		if !want[k] {
			ops = append(ops, fmt.Sprintf("delete %s: actual %v meta %+v", k, kv.Value, kv.Meta))
		}
	}
	sort.Strings(ops)
	return ops
}

// restartVeth is the fixed-argv Linux rig helper (same as af_packet's test): slot-prefixed veth
// pair, deleted in Cleanup even when the test fails. (ALLOW: rig helper, fixed argv, test-only.)
func restartVeth(t *testing.T, name, peer string) {
	t.Helper()
	_ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() //nolint:gosec // G204 ALLOW: leftover of an aborted run
	for _, args := range [][]string{{"link", "add", name, "type", "veth", "peer", "name", peer}, {"link", "set", name, "up"}, {"link", "set", peer, "up"}} {
		if out, err := exec.Command("/usr/sbin/ip", args...).CombinedOutput(); err != nil { //nolint:gosec // G204 ALLOW: fixed argv rig helper
			t.Fatalf("ip %v: %v: %s", args, err, out)
		}
	}
	t.Cleanup(func() { _ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() }) //nolint:gosec // G204 ALLOW: cleanup
}

// TestRestartSimulationOnHost applies one desired state with every DF-1 object type through a
// registry, then simulates an agent restart (new API connection, freshly constructed
// descriptors with empty process memory) and checks that Retrieve rebuilds Value and Meta so
// that the scheduler's plan is empty — except for the two attributes VPP cannot report back
// (promisc, mac-address), which are re-applied once. Finally everything is deleted in reverse
// order by the restarted agent and Retrieve shows nothing of the owner.
//
// It runs with its own owner "<prefix>r" so that the other DF-1 integration tests (other go test
// packages, running in parallel under the same slot) do not appear in its owner-wide diff; every
// name still carries the slot prefix.
func TestRestartSimulationOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	owner := prefix + "r"
	memifDir := filepath.Join("/run/vrx-test", prefix, "memif-restart")
	ctx := context.Background()
	c1 := ifacetest.Connect(t)
	r1 := scheduler.NewRegistry()
	registerAll(r1, c1, owner, memifDir)

	_, loopKey := ifacetest.Loopback(t, c1, owner, 60) // P05 core's object: created directly
	tb := vpptest.TableBase(t)
	slot := strconv.Itoa(vpptest.Slot(t))
	tap := func(i int) *tapv2.Tap {
		n := vpptest.Name(t, "tap"+strconv.Itoa(i))
		return &tapv2.Tap{Name: n, Id: vpptest.LoopbackInstance(t, i), HostIfName: n, RxRingSize: 256, TxRingSize: 256}
	}
	tapKey := func(i int) string { return "tapv2.tap/" + vpptest.Name(t, "tap"+strconv.Itoa(i)) }
	af, afPeer := vpptest.Name(t, "af60"), vpptest.Name(t, "af60p")
	restartVeth(t, af, afPeer)
	afKey := "af-packet.host-interface/" + af
	bondObj := &bond.Bond{Name: vpptest.Name(t, "bond60"), Id: vpptest.LoopbackInstance(t, 60), Mode: bond.Mode_MODE_LACP, Lb: bond.LoadBalance_LOAD_BALANCE_L34}
	bondKey := "bond.bond/" + bondObj.Name
	sub := &iface.Subinterface{Parent: tapKey(62), SubId: 100, OuterVlan: 100, ExactMatch: true}
	subKey := "interface.subinterface/" + vpptest.Name(t, "tap62") + ".100"
	sock := &memif.Socket{Id: tb + 60, Filename: filepath.Join(memifDir, vpptest.Name(t, "memif60")+".sock")}
	bdID := tb + 60

	objs := []proto.Message{
		// interface creators
		tap(60), tap(61), tap(62), tap(63), tap(64),
		&afpacket.HostInterface{Name: af, HostIfName: af},
		bondObj,
		&bond.Member{Bond: bondKey, Interface: tapKey(60)},
		&bond.Member{Bond: bondKey, Interface: tapKey(61), Passive: true, LongTimeout: true},
		sock,
		&memif.Memif{Name: vpptest.Name(t, "memif60"), Id: 60, Socket: sock.Id, Role: memif.Role_ROLE_MASTER, Mode: memif.Mode_MODE_ETHERNET},
		sub,
		// interface attributes
		&iface.AdminState{Interface: loopKey},
		&iface.AdminState{Interface: tapKey(62)},
		&iface.AdminState{Interface: subKey},
		&iface.Mtu{Interface: loopKey, Mtu: 1500, Ip4: 1400},
		&iface.MacAddress{Interface: loopKey, Mac: "02:0" + slot + ":00:00:3c:01"},
		&iface.Promisc{Interface: tapKey(63)},
		&iface.RxMode{Interface: tapKey(63), Mode: iface.RxModeKind_RX_MODE_KIND_INTERRUPT},
		// l2
		&l2.BridgeDomain{Id: bdID, Flood: true, UuFlood: true, Forward: true, Learn: true, MacAge: 5},
		&l2.BridgeDomainMember{BridgeDomain: bdID, Interface: subKey, Shg: 1},
		&l2.BridgeDomainMember{BridgeDomain: bdID, Interface: loopKey, PortType: l2.PortType_PORT_TYPE_BVI},
		&l2.FibEntry{BridgeDomain: bdID, Mac: "02:0" + slot + ":00:00:3c:02", Interface: subKey, Static: true},
		&l2.Flags{BridgeDomain: bdID, Interface: subKey, Learn: false, Forward: true, Flood: true, UuFlood: true},
		&l2.VlanTagRewrite{Interface: subKey, Op: l2.VtrOp_VTR_OP_POP_1, BridgeDomain: bdID},
		&l2.Xconnect{Rx: tapKey(64), Tx: afKey},
		&l2.Xconnect{Rx: afKey, Tx: tapKey(64)},
		// generic interface aliases consumers depend on (D-065): the desired-state builder emits one per interface
		&iface.InterfaceAlias{Name: "loop" + strconv.Itoa(int(vpptest.LoopbackInstance(t, 60))), Creator: loopKey},
		&iface.InterfaceAlias{Name: vpptest.Name(t, "tap60"), Creator: tapKey(60)},
		&iface.InterfaceAlias{Name: vpptest.Name(t, "tap61"), Creator: tapKey(61)},
		&iface.InterfaceAlias{Name: vpptest.Name(t, "tap62"), Creator: tapKey(62)},
		&iface.InterfaceAlias{Name: vpptest.Name(t, "tap63"), Creator: tapKey(63)},
		&iface.InterfaceAlias{Name: vpptest.Name(t, "tap64"), Creator: tapKey(64)},
		&iface.InterfaceAlias{Name: vpptest.Name(t, "memif60"), Creator: "memif.memif/" + vpptest.Name(t, "memif60")},
		&iface.InterfaceAlias{Name: vpptest.Name(t, "tap62") + ".100", Creator: subKey},
		&iface.InterfaceAlias{Name: af, Creator: afKey},
		&iface.InterfaceAlias{Name: bondObj.Name, Creator: bondKey},
		// l3xc
		&l3xc.L3Xc{Interface: tapKey(63), Paths: []*l3xc.Path{{NextHop: "10." + slot + ".60.254", Interface: loopKey, Weight: 1}}},
	}
	var desired []scheduler.KV
	for _, o := range objs {
		d, ok := r1.Get(descriptorOf(o))
		if !ok {
			t.Fatalf("no descriptor for %T", o)
		}
		// normalised as the P05 scheduler does before planning (canonical interface/<name> refs, …)
		desired = append(desired, scheduler.KV{Key: d.KeyOf(o), Value: iface.Normalize(d, o)})
	}
	desired = topo(t, r1, desired)

	// apply (agent #1); if anything fails, Cleanup deletes what was created in reverse order
	metas := map[scheduler.Key]any{}
	var applied []scheduler.KV
	t.Cleanup(func() {
		for i := len(applied) - 1; i >= 0; i-- {
			kv := applied[i]
			if _, left := metas[kv.Key]; !left {
				continue
			}
			d, _ := r1.ForKey(kv.Key)
			if err := d.Delete(context.Background(), kv.Value, metas[kv.Key]); err != nil {
				t.Errorf("cleanup %s: %v", kv.Key, err)
			}
		}
		_ = os.Remove(sock.Filename)
	})
	for _, kv := range desired {
		d, _ := r1.ForKey(kv.Key)
		meta, err := d.Create(ctx, kv.Value)
		if err != nil {
			t.Fatalf("create %s: %v", kv.Key, err)
		}
		metas[kv.Key] = meta
		applied = append(applied, kv)
		t.Logf("create %s", kv.Key)
	}

	// idempotent re-apply by the same agent: empty plan
	if ops := plan(r1, desired, retrieveAll(t, r1)); len(ops) != 0 {
		t.Fatalf("second apply by the same agent is not empty: %q", ops)
	}
	t.Logf("same agent, same desired state: plan is empty (%d objects)", len(desired))
	ifacetest.Hold(t)

	// restart simulation: new connection, new descriptors, nothing remembered
	c2 := ifacetest.Connect(t)
	r2 := scheduler.NewRegistry()
	registerAll(r2, c2, owner, memifDir)
	actual := retrieveAll(t, r2)
	for _, kv := range desired {
		got, ok := actual[kv.Key]
		if ok && got.Meta != metas[kv.Key] {
			t.Errorf("after restart %s Meta = %+v, Create returned %+v", kv.Key, got.Meta, metas[kv.Key])
		}
	}
	ops := plan(r2, desired, actual)
	notReadable := map[string]bool{
		"create interface.promisc/" + vpptest.Name(t, "tap63"):                                   true,
		"create interface.mac-address/loop" + strconv.Itoa(int(vpptest.LoopbackInstance(t, 60))): true,
	}
	for _, op := range ops {
		if !notReadable[op] {
			t.Errorf("after restart the plan is not empty: %s", op)
		}
	}
	t.Logf("restarted agent: Retrieve rebuilt %d objects with equal Meta; plan = %q (VPP cannot report promisc / a configured MAC: re-applied idempotently)", len(actual), ops)
	for _, op := range ops { // re-apply what cannot be read back, as the scheduler would
		for _, kv := range desired {
			if op == "create "+string(kv.Key) {
				d, _ := r2.ForKey(kv.Key)
				if _, err := d.Create(ctx, kv.Value); err != nil {
					t.Fatalf("re-apply %s: %v", kv.Key, err)
				}
			}
		}
	}
	if ops := plan(r2, desired, retrieveAll(t, r2)); len(ops) != 0 {
		t.Fatalf("restarted agent, second apply is not empty: %q", ops)
	}
	t.Log("restarted agent, second apply: plan is empty")

	// simulated loss (review M2): prefixed objects vanish behind the agent's back (deleted via the
	// binary API, as a VPP-side accident would) → Retrieve → the plan is exactly the creates of
	// what is gone → apply in dependency order → plan empty again, new Meta.
	actual = retrieveAll(t, r2)
	lost := lose(t, c2, actual, tapKey(62), "memif.memif/"+vpptest.Name(t, "memif60"), sock.Filename, sock.Id, tapKey(63))
	ops = plan(r2, desired, retrieveAll(t, r2))
	if len(ops) == 0 {
		t.Fatal("plan after the loss is empty")
	}
	for _, op := range ops {
		if !strings.HasPrefix(op, "create ") {
			t.Errorf("after the loss the plan must only re-create, got %s", op)
		}
	}
	t.Logf("lost %q; plan = %d creates: %q", lost, len(ops), ops)
	var recreated []string
	for _, kv := range desired { // dependency order
		if !slices.Contains(ops, "create "+string(kv.Key)) {
			continue
		}
		d, _ := r2.ForKey(kv.Key)
		meta, err := d.Create(ctx, kv.Value)
		if err != nil {
			t.Fatalf("re-create %s: %v", kv.Key, err)
		}
		metas[kv.Key] = meta
		recreated = append(recreated, string(kv.Key))
	}
	if ops := plan(r2, desired, retrieveAll(t, r2)); len(ops) != 0 {
		t.Fatalf("after re-creating the lost objects the plan is not empty: %q", ops)
	}
	t.Logf("reconcile re-created %d objects in dependency order %q; plan is empty again", len(recreated), recreated)

	// delete everything with the restarted agent (reverse order, Meta from Retrieve)
	actual = retrieveAll(t, r2)
	for i := len(desired) - 1; i >= 0; i-- {
		kv := desired[i]
		d, _ := r2.ForKey(kv.Key)
		if err := d.Delete(ctx, kv.Value, actual[kv.Key].Meta); err != nil {
			t.Fatalf("delete %s: %v", kv.Key, err)
		}
		delete(metas, kv.Key)
	}
	left := retrieveAll(t, r2)
	for k, kv := range left {
		if a, ok := kv.Value.(*iface.InterfaceAlias); ok && (a.GetCreator() == "" || a.GetCreator() == loopKey) {
			delete(left, k) // untagged interfaces of others (observe-only) and P05's loopback (deleted in Cleanup)
		}
	}
	if len(left) != 0 {
		keys := make([]string, 0, len(left))
		for k := range left {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		t.Fatalf("owner %s still has objects after delete: %q", owner, keys)
	}
	t.Logf("restarted agent deleted %d objects; Retrieve for owner %s is empty", len(desired), owner)
}

// lose deletes objects via the binary API behind the agent's back: the tap (VPP removes its
// sub-interface and with it the bridge membership, flags, rewrite, fib entry), the memif and its
// socket, and the l3xc on another tap. It returns what it deleted.
func lose(t *testing.T, c vpp.Client, actual map[scheduler.Key]scheduler.KV, tapRef, memifRef, sockFile string, sockID uint32, l3xcTap string) []string {
	t.Helper()
	ctx := context.Background()
	idx := func(k string) interface_types.InterfaceIndex {
		kv, ok := actual[scheduler.Key(k)]
		if !ok {
			t.Fatalf("%s not retrieved before the loss", k)
		}
		return interface_types.InterfaceIndex(kv.Meta.(iface.Meta).SwIfIndex)
	}
	if _, err := tapapi.NewServiceClient(c).TapDeleteV2(ctx, &tapapi.TapDeleteV2{SwIfIndex: idx(tapRef)}); err != nil {
		t.Fatalf("tap_delete_v2: %v", err)
	}
	if _, err := l3xcapi.NewServiceClient(c).L3xcDel(ctx, &l3xcapi.L3xcDel{SwIfIndex: idx(l3xcTap)}); err != nil {
		t.Fatalf("l3xc_del: %v", err)
	}
	ms := memifapi.NewServiceClient(c)
	if _, err := ms.MemifDelete(ctx, &memifapi.MemifDelete{SwIfIndex: idx(memifRef)}); err != nil {
		t.Fatalf("memif_delete: %v", err)
	}
	if _, err := ms.MemifSocketFilenameAddDelV2(ctx, &memifapi.MemifSocketFilenameAddDelV2{IsAdd: false, SocketID: sockID, SocketFilename: sockFile}); err != nil {
		t.Fatalf("memif_socket_filename_add_del_v2 (del): %v", err)
	}
	return []string{tapRef, "l3xc on " + l3xcTap, memifRef, "memif.socket/" + strconv.FormatUint(uint64(sockID), 10)}
}
