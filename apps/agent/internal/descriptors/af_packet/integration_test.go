package afpacket_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// callTimeout is the deadline of one descriptor call on the host: generous, because VPP's API was
// seen stalling for up to 5.5 min around af_packet create/delete with the default TX ring (D-107,
// D-108 / I6). Every call's duration is logged.
const callTimeout = 6 * time.Minute

// veth is the fixed-argv Linux rig helper the task allows for host-interface tests: it creates
// the slot-prefixed veth pair the af_packet interface attaches to and deletes it in Cleanup.
// No user input reaches it; every argument is a literal or a vpptest-derived, IFNAMSIZ-checked
// name. IPv6 is disabled on both ends before they come up, so the kernel sends nothing (no
// DAD/RS/MLD; no IPv4 address either): no packet crosses the interfaces (D-101, TD-5).
// (ALLOW: rig helper, fixed argv, test-only.) The Cleanup keeps the pair when keep() says an
// af_packet interface may still be attached (a descriptor Delete failed, review N4): removing
// the netdev under it is what the quiesce exists to avoid; `tools/lab rig gc w<slot>` removes it.
func veth(t *testing.T, name, peer string, keep func() bool) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("/usr/sbin/ip", args...).CombinedOutput() //nolint:gosec // G204 ALLOW: fixed argv rig helper
		if err != nil {
			t.Fatalf("ip %v: %v: %s", args, err, out)
		}
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatalf("%s exists from an aborted run: remove it with the quiesce (tools/lab rig gc w<slot>), not here with a VPP interface possibly attached", name)
	}
	run("link", "add", name, "type", "veth", "peer", "name", peer)
	t.Cleanup(func() { // after the descriptor Delete (LIFO)
		if keep() {
			t.Logf("veth %s/%s kept: a descriptor Delete failed, an af_packet interface may be attached (tools/lab rig gc w<slot>)", name, peer)
			return
		}
		_ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() //nolint:gosec // G204 ALLOW: fixed argv cleanup
	})
	for _, dev := range []string{name, peer} {
		p := "/proc/sys/net/ipv6/conf/" + dev + "/disable_ipv6"
		if err := os.WriteFile(p, []byte("1\n"), 0o600); err != nil {
			t.Fatalf("disable_ipv6 on %s: %v", dev, err)
		}
	}
	run("link", "set", name, "up")
	run("link", "set", peer, "up")
}

func keyOf(kv scheduler.KV) string { return string(kv.Key) }

// timed runs one descriptor call with callTimeout and logs how long it took.
func timed[T any](t *testing.T, what string, call func(context.Context) (T, error)) (T, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	start := time.Now()
	v, err := call(ctx)
	t.Logf("timing: %s took %s (err %v)", what, time.Since(start).Round(time.Millisecond), err)
	return v, err
}

func up(t *testing.T, dev string) bool {
	t.Helper()
	i, err := net.InterfaceByName(dev)
	if err != nil {
		t.Fatalf("netdev %s: %v", dev, err)
	}
	return i.Flags&net.FlagUp != 0
}

// TestHostInterfaceOnHost (DF-1, TD-5): on the slot's veth pair (both ends up), Create through the
// descriptor → Retrieve == desired; Delete through the descriptor → the netdev reads down (the
// quiesce ran before af_packet_delete: D-101 / VPP V24), Retrieve has no key and `vppctl show
// interface` has no host-<dev>. Two af_packet create/delete cycles (ethernet and ip mode), no
// packets.
func TestHostInterfaceOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	name, peer := vpptest.Name(t, "af50"), vpptest.Name(t, "af50p")
	deleteFailed := false // a Cleanup Delete failed: keep the veth (review N4)
	veth(t, name, peer, func() bool { return deleteFailed })
	d := afpacket.New(c, owner) // the product link controller (netlink, veth-only) and settle
	objs := []*afpacket.HostInterface{
		{Name: name, HostIfName: name},
		{Name: peer, HostIfName: peer, Mode: afpacket.Mode_MODE_IP},
	}
	metas := map[string]any{}
	for _, o := range objs {
		key := string(d.KeyOf(o))
		meta, err := timed(t, "Create "+key, func(ctx context.Context) (any, error) { return d.Create(ctx, o) })
		if err != nil {
			t.Fatalf("Create %s: %v", key, err)
		}
		metas[key] = meta
		t.Cleanup(func() { // only when the test stopped before its own Delete
			if _, ok := metas[key]; ok {
				if _, err := timed(t, "Cleanup Delete "+key, func(ctx context.Context) (struct{}, error) { return struct{}{}, d.Delete(ctx, o, meta) }); err != nil {
					deleteFailed = true
					t.Errorf("Cleanup Delete %s: %v", key, err)
				}
			}
		})
		if !up(t, o.GetHostIfName()) {
			t.Fatalf("%s is not up after Create (VPP sets IFF_UP)", o.GetHostIfName())
		}
		kvs, err := timed(t, "Retrieve", func(ctx context.Context) ([]scheduler.KV, error) { return d.Retrieve(ctx) })
		if err != nil {
			t.Fatal(err)
		}
		got := ifacetest.Find(t, kvs, key, keyOf)
		if !proto.Equal(got.Value, o) || got.Meta != meta {
			t.Fatalf("Retrieve %s = %v %+v, want %v %+v", key, got.Value, got.Meta, o, meta)
		}
		t.Logf("af-packet.host-interface: Retrieve == desired: %s %v", key, got.Value)
	}
	t.Logf("vppctl show interface (after Create):\n%s", showInterfaces(t, name))
	t.Logf("vppctl show hardware-interfaces host-%s (D-108/D-113 rings):\n%s", name, vppctl(t, "show", "hardware-interfaces", "host-"+name))
	ifacetest.Hold(t)

	for _, o := range objs {
		key := string(d.KeyOf(o))
		if _, err := timed(t, "Delete "+key, func(ctx context.Context) (struct{}, error) { return struct{}{}, d.Delete(ctx, o, metas[key]) }); err != nil {
			t.Fatalf("Delete %s: %v", key, err)
		}
		delete(metas, key)
		if up(t, o.GetHostIfName()) {
			t.Fatalf("netdev %s is still up after Delete: the quiesce did not run (D-101 / V24)", o.GetHostIfName())
		}
		t.Logf("after Delete %s: netdev %s reads down (net.FlagUp clear)", key, o.GetHostIfName())
	}
	kvs, err := timed(t, "Retrieve", func(ctx context.Context) ([]scheduler.KV, error) { return d.Retrieve(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range kvs {
		for _, o := range objs {
			if kv.Key == d.KeyOf(o) {
				t.Fatalf("%s still retrieved after Delete", kv.Key)
			}
		}
	}
	show := showInterfaces(t, name)
	if strings.Contains(show, "host-"+name) || strings.Contains(show, "host-"+peer) {
		t.Fatalf("vppctl show interface still lists host-%s / host-%s:\n%s", name, peer, show)
	}
	t.Logf("after Delete: Retrieve has neither key; vppctl show interface has no host-%s*:\n%s", name, show)
}

// vppctl runs a read-only `vppctl show …` (literal words and vpptest-derived names only). (ALLOW:
// test-only, fixed argv.)
func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	if len(args) == 0 || args[0] != "show" {
		t.Fatalf("vppctl %v: only show commands", args)
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/vppctl", args...).CombinedOutput() //nolint:gosec // G204 ALLOW: fixed argv, show only
	if err != nil {
		t.Fatalf("vppctl %v: %v: %s", args, err, out)
	}
	return string(out)
}

// showInterfaces returns the lines of `vppctl show interface` that mention prefix (the header line
// is kept).
func showInterfaces(t *testing.T, prefix string) string {
	t.Helper()
	var keep []string
	for i, l := range strings.Split(vppctl(t, "show", "interface"), "\n") {
		if i == 0 || strings.Contains(l, "host-"+prefix) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}
