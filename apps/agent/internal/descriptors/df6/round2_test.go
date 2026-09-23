package df6_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/vxlan"
)

// TestBypassInterfaceRecreated (fix round 2, N1): the interface under a write-only bypass is
// lost and recreated on the SAME VPP boot — with a new sw_if_index and with the old one
// reused — and the re-apply enables the feature exactly once on it.
func TestBypassInterfaceRecreated(t *testing.T) {
	ctx := context.Background()
	f := newCountingFake()
	owner := "w11rc"
	iface.SetClaimStore(owner, nil)
	f.SetBoot(700)
	old := f.AddInterface("loop1171", owner+":loop1171")
	d := vxlan.NewBypass(f, owner)
	obj := &vxlan.Bypass{Interface: "loop1171", Ipv4: true}
	if _, err := d.Create(ctx, obj); err != nil {
		t.Fatal(err)
	}
	for _, reuse := range []bool{false, true} {
		f.RemoveInterface(old) // VPP drops the interface's features with it
		var idx uint32
		if reuse {
			f.AddInterfaceAt(old, "loop1171", owner+":loop1171")
			idx = old
		} else {
			idx = f.AddInterface("loop1171", owner+":loop1171")
		}
		for i := 0; i < 2; i++ { // two resyncs
			if _, err := d.Create(ctx, obj); err != nil {
				t.Fatal(err)
			}
		}
		if n := f.Feature("ip4-unicast", "ip4-vxlan-bypass", idx); n != 1 {
			t.Fatalf("reuse=%v: old idx %d new idx %d: feature instances %d, want 1", reuse, old, idx, n)
		}
		old = idx
	}
}

// TestBootIDTriple (fix round 2, N2, D-080): the identity is (kernel boot_id, VPP PID, VPP
// start time); a host reboot that gives VPP the same PID still changes it.
func TestBootIDTriple(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	saved := df6.ProcRoot
	df6.ProcRoot = root
	t.Cleanup(func() { df6.ProcRoot = saved })
	write := func(bootID, start string) {
		if err := os.MkdirAll(filepath.Join(root, "sys/kernel/random"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "4242"), 0o700); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(root, "sys/kernel/random/boot_id"), []byte(bootID+"\n"), 0o600)
		stat := "4242 (vpp_main thr) S 1 4242 4242 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 3 0 " + start + " 1 2 3"
		_ = os.WriteFile(filepath.Join(root, "4242/stat"), []byte(stat), 0o600)
	}
	f := df6test.NewFakeVPP()
	f.SetBoot(4242)
	write("aaaa", "1000")
	a, err := df6.BootID(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if a != "aaaa/4242/1000" {
		t.Fatalf("BootID = %q", a)
	}
	write("bbbb", "1000") // host reboot, VPP got the same PID and start tick
	b, _ := df6.BootID(ctx, f)
	write("aaaa", "2000") // VPP restart with a recycled PID
	c, _ := df6.BootID(ctx, f)
	if a == b || a == c || b == c {
		t.Fatalf("identities must differ: %q %q %q", a, b, c)
	}
}
