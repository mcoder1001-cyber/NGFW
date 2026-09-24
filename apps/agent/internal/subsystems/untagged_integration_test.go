package subsystems

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/core"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// TD-11c host proof (review 3.1b): an address and a VRF binding on an UNTAGGED NIC apply through the
// agent's product wiring (subsystems.Register: core with the persisted interface claims, the DF-1
// alias, the scheduler) on the host VPP, Retrieve == desired, a new agent over the same state dir
// (restart) still owns them through the claim file and plans nothing, and removal releases the claims.
//
// The NIC: the product af_packet descriptor creates host-<prefix>-u0 on the slot's veth (sanitized,
// TD-3/TD-5), then the test removes its owner tag — from then on it is a pre-existing interface like a
// DPDK NIC named by F-startup-gen (the document names it by VPP's name). Teardown tags it again and
// deletes it through the descriptor (quiesced, D-101/TD-5). No packets: IPv6 is off on both veth ends,
// the VPP interface stays admin-down, the address is IPv4 only (D-126). No trace commands (D-128).
func TestUntaggedNICClaimsOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	dev, peer := vpptest.Name(t, "u0"), vpptest.Name(t, "u0p")
	nic := "host-" + dev
	deleteFailed := false
	hostVeth(t, dev, peer, func() bool { return deleteFailed })

	call := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 6*time.Minute) // D-107: af_packet API stalls
	}
	ctx, cancel := call()
	defer cancel()
	afp := afpacket.New(c, owner)
	hi := &afpacket.HostInterface{Name: nic, HostIfName: dev}
	meta, err := afp.Create(ctx, hi)
	if err != nil {
		t.Fatalf("create %s: %v", nic, err)
	}
	idx := meta.(iface.Meta).SwIfIndex
	t.Cleanup(func() { // after the objects' cleanup below (LIFO)
		ctx, cancel := call()
		defer cancel()
		if err := iface.Tag(ctx, c, owner, nic, idx); err != nil {
			deleteFailed = true
			t.Errorf("re-tag %s for its delete: %v", nic, err)
			return
		}
		if err := afp.Delete(ctx, hi, meta); err != nil {
			deleteFailed = true
			t.Errorf("delete %s (quiesced): %v", nic, err)
		}
	})
	if _, err := ifapi.NewServiceClient(c).SwInterfaceTagAddDel(ctx, &ifapi.SwInterfaceTagAddDel{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		t.Fatalf("untag %s: %v", nic, err)
	}
	t.Logf("untagged NIC %s (sw_if_index %d); vppctl show interface:\n%s", nic, idx, showLines(t, []string{"show", "interface"}, nic))

	dir := t.TempDir()
	build := func() *scheduler.Scheduler {
		t.Helper()
		owned, err := ownertable.Open(dir, owner)
		if err != nil {
			t.Fatal(err)
		}
		reg := scheduler.NewRegistry()
		w, err := Register(reg, Env{Client: c, Owner: owner, StateDir: dir, Owned: owned})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := call()
		defer cancel()
		w.Connected(ctx) // D-080 boot identity for the claim stores
		return scheduler.New(reg, nil)
	}
	table := vpptest.TableBase(t) + 11
	const addr = "10.10.0.1/24" // the slot's 10.<N>.0.0/16
	desired := []scheduler.KV{
		{Key: core.VRFKey(table), Value: &core.Table{Id: table, Vrf: "td11c"}},
		{Key: iface.AliasKey(nic), Value: &iface.InterfaceAlias{Name: nic}},
		{Key: core.InterfaceTableKey(nic), Value: &core.InterfaceTable{Interface: nic, TableId: table}},
		{Key: core.InterfaceAddrKey(nic, addr), Value: &core.InterfaceAddress{Interface: nic, Prefix: addr}},
	}
	scope := scheduler.Only(core.VRFName, iface.AliasName, core.InterfaceTableName, core.InterfaceAddrName)
	t.Cleanup(func() { // leaves nothing of ours if the test stops early
		ctx, cancel := call()
		defer cancel()
		if r := build().Apply(ctx, nil, scope); r.Outcome != scheduler.OutcomeApplied {
			t.Errorf("cleanup apply %s: %v", r.Outcome, r.Err)
		}
	})
	retrieveEquals := func(s *scheduler.Scheduler, when string) {
		t.Helper()
		ctx, cancel := call()
		defer cancel()
		kvs, err := s.Retrieve(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		got := map[scheduler.Key]proto.Message{}
		for _, kv := range kvs {
			got[kv.Key] = kv.Value
		}
		for _, kv := range desired {
			if !proto.Equal(got[kv.Key], kv.Value) {
				t.Fatalf("%s: Retrieve %s = %v, want %v", when, kv.Key, got[kv.Key], kv.Value)
			}
			t.Logf("%s: Retrieve == desired: %s %v", when, kv.Key, got[kv.Key])
		}
	}

	s1 := build()
	ctx, cancel = call()
	defer cancel()
	if r := s1.Apply(ctx, desired, scope); r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply %s: %v %+v", r.Outcome, r.Err, r.Results)
	}
	retrieveEquals(s1, "after apply")
	t.Logf("vppctl show interface address:\n%s", showLines(t, []string{"show", "interface", "address"}, nic, addr))
	claims := claimsOnDisk(t, dir, owner)
	for _, rec := range []string{nic + "|" + core.InterfaceTableName, nic + "|" + core.AddrHolder(addr)} {
		if !strings.Contains(claims, rec) {
			t.Fatalf("claim %s not on disk:\n%s", rec, claims)
		}
	}
	t.Logf("claims-iface-%s.json:\n%s", owner, claims)

	// restart: a new agent (registry, stores, scheduler) over the same state dir
	s2 := build()
	retrieveEquals(s2, "after restart")
	if p, err := s2.Plan(ctx, desired, scope); err != nil || !p.Empty() || len(p.Issues) > 0 {
		t.Fatalf("after restart the plan is not empty: %v %+v %v", err, p.Ops, p.Issues)
	}
	t.Log("after restart: plan empty (the claims were loaded; nothing to write)")

	// removal: address and binding leave VPP, the claims are released, the NIC stays
	if r := s2.Apply(ctx, nil, scope); r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("removal %s: %v %+v", r.Outcome, r.Err, r.Results)
	}
	if out := showLines(t, []string{"show", "interface", "address"}, nic, addr); strings.Contains(out, addr) {
		t.Fatalf("address still in VPP:\n%s", out)
	}
	if claims := claimsOnDisk(t, dir, owner); strings.Contains(claims, "interface-ip") {
		t.Fatalf("claims not released:\n%s", claims)
	}
	t.Logf("after removal: vppctl show interface address:\n%s", showLines(t, []string{"show", "interface", "address"}, nic, addr))
}

func claimsOnDisk(t *testing.T, dir, owner string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "claims-iface-"+owner+".json")) //nolint:gosec // the test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// showLines runs a vppctl show command (show only; never trace, D-128) and keeps the header and the
// lines that mention one of match (plus the indented lines after a matching interface line).
func showLines(t *testing.T, args []string, match ...string) string {
	t.Helper()
	if len(args) == 0 || args[0] != "show" || strings.Contains(strings.Join(args, " "), "trace") {
		t.Fatalf("vppctl %v: only show commands, never trace (D-128)", args)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/vppctl", args...).CombinedOutput() //nolint:gosec // G204 ALLOW: fixed argv, show only, test-only
	if err != nil {
		t.Fatalf("vppctl %v: %v: %s", args, err, out)
	}
	var keep []string
	in := false
	for i, l := range strings.Split(string(out), "\n") {
		hit := false
		for _, m := range match {
			hit = hit || strings.Contains(l, m)
		}
		switch {
		case i == 0 || hit:
			keep, in = append(keep, l), strings.Contains(l, match[0])
		case in && strings.HasPrefix(l, " "):
			keep = append(keep, l)
		default:
			in = false
		}
	}
	return strings.Join(keep, "\n")
}

// hostVeth is the fixed-argv rig helper (as in the af_packet integration test): the slot-prefixed veth
// pair the af_packet NIC attaches to, IPv6 off on both ends before they come up, so the kernel sends
// nothing (D-101, TD-5). The pair is kept when keep() says an af_packet interface may still be
// attached (tools/lab rig gc w<slot> removes it). (ALLOW: rig helper, fixed argv, test-only.)
func hostVeth(t *testing.T, name, peer string, keep func() bool) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("/usr/sbin/ip", args...).CombinedOutput() //nolint:gosec // G204 ALLOW: fixed argv rig helper
		if err != nil {
			t.Fatalf("ip %v: %v: %s", args, err, out)
		}
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatalf("%s exists from an aborted run: remove it with the quiesce (tools/lab rig gc w<slot>)", name)
	}
	run("link", "add", name, "type", "veth", "peer", "name", peer)
	t.Cleanup(func() {
		if keep() {
			t.Logf("veth %s/%s kept: a delete failed, an af_packet interface may be attached (tools/lab rig gc w<slot>)", name, peer)
			return
		}
		_ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() //nolint:gosec // G204 ALLOW: fixed argv cleanup
	})
	for _, d := range []string{name, peer} {
		if err := os.WriteFile("/proc/sys/net/ipv6/conf/"+d+"/disable_ipv6", []byte("1\n"), 0o600); err != nil {
			t.Fatalf("disable_ipv6 on %s: %v", d, err)
		}
	}
	run("link", "set", name, "up")
	run("link", "set", peer, "up")
}
