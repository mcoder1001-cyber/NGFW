package npt66_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	binnpt66 "ngfw/agent/binapi/npt66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/descriptors/npt66"
	"ngfw/agent/internal/vpp/vpptest"
)

func nRestarts(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts").Output()
	if err != nil {
		t.Logf("systemctl show vpp: %v", err)
		return "?"
	}
	return strings.TrimSpace(string(out))
}

// ourBindings are the lines of `vppctl show npt66 bindings` for the slot's internal prefix (the command prints every
// owner's bindings, without the interface: "[index] internal: <pfx> external: <pfx>").
func ourBindings(t *testing.T, internal string) []string {
	t.Helper()
	out, err := exec.Command("vppctl", "show", "npt66", "bindings").CombinedOutput()
	if err != nil {
		t.Fatalf("vppctl show npt66 bindings: %v %s", err, out)
	}
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "internal: "+internal) {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	return lines
}

// TestNpt66OnHost: the first use of npt66 on this VPP (loaded since D-060). The binding is created three times (the
// reconciler re-applies write-only objects on every resync) and must exist exactly once, updated in place, deleted
// before its loopback (Cleanup order), and a second delete is not an error. NRestarts is logged before and after
// (D-064) and must not change.
func TestNpt66OnHost(t *testing.T) {
	c := nattest.Connect(t)
	if !c.PluginLoaded(&binnpt66.Npt66BindingAddDel{}) {
		t.Skip("npt66 plugin not loaded on this VPP (shared-host-rules §2)")
	}
	before := nRestarts(t)
	t.Logf("systemctl show vpp -p NRestarts (before) = %s", before)
	t.Cleanup(func() {
		after := nRestarts(t)
		t.Logf("systemctl show vpp -p NRestarts (after) = %s", after)
		if after != before {
			t.Errorf("VPP restarted during the test (D-064): %s → %s", before, after)
		}
	})
	ctx := nattest.Ctx(t)
	p := npt66.New(c, vpptest.Prefix(t))
	ifName, idx := nattest.Loopback(t, c, 66) // deleted in Cleanup after the binding (LIFO)
	internal, external := nattest.Prefix6(t, 0x10, 48), nattest.Prefix6(t, 0x20, 48)
	obj := natcommon.MustEncode(&npt66.BindingSpec{Interface: ifName, Internal: internal, External: external})
	t.Logf("binding %s on %s (sw_if_index %d): %s → %s", p.Binding.KeyOf(obj), ifName, idx, internal, external)

	nattest.AssertWriteOnly(t, p.Binding)
	nattest.CreateWriteOnly(ctx, t, p.Binding, obj) // two adds
	meta, err := p.Binding.Create(ctx, obj)         // a third (another resync)
	if err != nil {
		t.Fatal(err)
	}
	lines := ourBindings(t, internal)
	t.Logf("vppctl show npt66 bindings (ours, after 3 adds):\n  %s", strings.Join(lines, "\n  "))
	if len(lines) != 1 || !strings.Contains(lines[0], "external: "+external) {
		t.Fatalf("want exactly one binding %s → %s, got %v", internal, external, lines)
	}

	// another external prefix: in-place update, still one binding
	external2 := nattest.Prefix6(t, 0x21, 48)
	obj2 := natcommon.MustEncode(&npt66.BindingSpec{Interface: ifName, Internal: internal, External: external2})
	if _, err := p.Binding.Update(ctx, obj, obj2, meta); err != nil {
		t.Fatal(err)
	}
	lines = ourBindings(t, internal)
	t.Logf("after the update:\n  %s", strings.Join(lines, "\n  "))
	if len(lines) != 1 || !strings.Contains(lines[0], "external: "+external2) {
		t.Fatalf("update: %v", lines)
	}

	if err := p.Binding.Delete(ctx, obj2, meta); err != nil {
		t.Fatal(err)
	}
	if lines = ourBindings(t, internal); len(lines) != 0 {
		t.Fatalf("after delete: %v", lines)
	}
	if err := p.Binding.Delete(context.Background(), obj2, meta); err != nil {
		t.Fatalf("second delete (NO_SUCH_ENTRY tolerated): %v", err)
	}
	t.Log("deleted; a second delete is a no-op")
}
