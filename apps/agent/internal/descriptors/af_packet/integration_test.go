package afpacket_test

import (
	"context"
	"os/exec"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// veth is the fixed-argv Linux rig helper the task allows for host-interface tests: it creates
// the slot-prefixed veth pair the af_packet interface attaches to and deletes it in Cleanup.
// No user input reaches it; every argument is a literal or a vpptest-derived, IFNAMSIZ-checked
// name. (ALLOW: rig helper, fixed argv, test-only.)
func veth(t *testing.T, name, peer string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("/usr/sbin/ip", args...).CombinedOutput() // ALLOW: fixed argv rig helper
		if err != nil {
			t.Fatalf("ip %v: %v: %s", args, err, out)
		}
	}
	_ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() // ALLOW: leftover from an aborted run
	run("link", "add", name, "type", "veth", "peer", "name", peer)
	t.Cleanup(func() { _ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() }) // ALLOW: cleanup
	run("link", "set", name, "up")
	run("link", "set", peer, "up")
}

func keyOf(kv scheduler.KV) string { return string(kv.Key) }

func TestHostInterfaceOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	ctx := context.Background()
	name, peer := vpptest.Name(t, "af50"), vpptest.Name(t, "af50p")
	veth(t, name, peer)
	d := afpacket.New(c, owner)
	objs := []*afpacket.HostInterface{
		{Name: name, HostIfName: name},
		{Name: peer, HostIfName: peer, Mode: afpacket.Mode_MODE_IP},
	}
	for _, o := range objs {
		key := string(d.KeyOf(o))
		meta, err := d.Create(ctx, o)
		if err != nil {
			t.Fatalf("Create %s: %v", key, err)
		}
		t.Cleanup(func() {
			if err := d.Delete(ctx, o, meta); err != nil {
				t.Errorf("Delete %s: %v", key, err)
				return
			}
			kvs, _ := d.Retrieve(ctx)
			for _, kv := range kvs {
				if string(kv.Key) == key {
					t.Errorf("%s still retrieved after Delete", key)
				}
			}
		})
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := ifacetest.Find(t, kvs, key, keyOf)
		if !proto.Equal(got.Value, o) || got.Meta != meta {
			t.Fatalf("Retrieve %s = %v %+v, want %v %+v", key, got.Value, got.Meta, o, meta)
		}
		t.Logf("af-packet.host-interface: Retrieve == desired: %s %v", key, got.Value)
	}
	ifacetest.Hold(t)
}
