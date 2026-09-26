package span

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/gre"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Host test (F-loopback-bvi-gso-lldp-span, ERSPAN): a slot-prefixed ERSPAN GRE tunnel created with DF-6's gre
// descriptor directly (the `tunnels` domain belongs to F-tunnels) is the destination of a device-level mirror of one
// of this slot's loopbacks, addressed by the tunnel's logical name gre<slot>78 (the owner tag DF-6 writes). Retrieve ==
// desired, `vppctl show interface span` lists the session, the restart simulation recreates it, the session goes before the
// tunnel (D-095c). No packet is sent.
func TestERSPANOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	tunnels := gre.NewTunnel(h.C, h.Owner)
	inst := uint32(h.Slot*100 + 78) //nolint:gosec // slot 1–12
	tun := &gre.Tunnel{Instance: inst, Type: gre.TunnelType_ERSPAN, Mode: gre.TunnelMode_P2P, Src: h.Addr(78, 1), Dst: h.Addr(78, 2), SessionId: 7}
	greName := gre.InterfaceName(inst)
	gmeta, err := tunnels.Create(h.Ctx, tun)
	if err != nil {
		t.Fatalf("DF-6 gre.tunnel ERSPAN %s: %v", greName, err)
	}
	t.Logf("ERSPAN fixture %s created by DF-6's gre descriptor (key %s, owner tag %s:%s)", greName, tunnels.KeyOf(tun), h.Owner, greName)
	t.Cleanup(func() { // registered first: runs after the mirror cleanup below (D-095c)
		if err := tunnels.Delete(context.Background(), tun, gmeta); err != nil {
			t.Errorf("delete %s: %v", greName, err)
		}
	})

	d := New(h.C, h.Owner)
	src, _ := h.Loopback(45, true, true)
	h.CleanupOwned(d)
	desired := df7test.Desired(d, df7.Encode(Mirror{Source: src, Destination: greName, State: StateBoth}))
	created := h.Apply(d, desired)
	t.Cleanup(func() { h.DeleteAll(d, created) })
	h.ExpectRetrieved(d, desired)
	t.Logf("Retrieve == desired: %s", desired.Key)
	out := vppctlShowSpan(t)
	t.Log("vppctl show interface span:\n" + out)
	if !strings.Contains(out, src) || !strings.Contains(out, greName) {
		t.Fatalf("show span lacks %s → %s", src, greName)
	}
	t.Run("restart simulation", func(*testing.T) {
		created = h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return New(c, h.Owner) }, desired)
	})
	h.DeleteAll(d, created)
	created = nil
	h.ExpectNone(d)
	if out := vppctlShowSpan(t); strings.Contains(out, src) {
		t.Fatalf("show span still lists %s after the delete:\n%s", src, out)
	}
}

// vppctlShowSpan is evidence only (a fixed command, no user input).
func vppctlShowSpan(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vppctl", "show", "interface", "span").CombinedOutput()
	if err != nil {
		t.Fatalf("vppctl show interface span: %v\n%s", err, out)
	}
	return string(out)
}
