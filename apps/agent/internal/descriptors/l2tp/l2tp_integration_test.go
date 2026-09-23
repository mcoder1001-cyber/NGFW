package l2tp_test

import (
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/l2tp"
)

// EnvCreate opts in to creating an L2TPv3 tunnel on the host. VPP 26.06 has no delete
// message, so the tunnel interface stays until VPP restarts; on the shared host this is only
// done deliberately (before a planned restart), never by default.
const EnvCreate = "VRX_DF6_L2TP_CREATE"

func TestL2tpOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(6, "")

	// interface-enable: write-only toggle, enable → disable on our loopback.
	e := l2tp.NewInterfaceEnable(h.Client, h.Owner)
	en := &l2tp.InterfaceEnable{Interface: loop}
	meta, err := e.Create(h.Ctx, en)
	if err != nil {
		t.Fatalf("interface enable: %v", err)
	}
	if err := e.Delete(h.Ctx, en, meta); err != nil {
		t.Fatalf("interface disable: %v", err)
	}
}

// TestL2tpTunnelOnHost: Retrieve always; create only on opt-in (no delete in VPP).
func TestL2tpTunnelOnHost(t *testing.T) {
	h := df6test.Connect(t)
	d := l2tp.NewTunnel(h.Client, h.Owner)
	before, err := d.Retrieve(h.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("l2tpv3 tunnels of %s before: %d", h.Owner, len(before))
	if os.Getenv(EnvCreate) != "1" {
		t.Skipf("l2tp.tunnel create skipped: VPP has no l2tpv3 delete, a created tunnel would outlive the test on the shared host (set %s=1 to opt in)", EnvCreate)
	}
	desired := &l2tp.Tunnel{Name: h.Name("l2tp1"), ClientAddress: h.IP6(6, 2), OurAddress: h.IP6(6, 1), LocalSessionId: h.Table(400), RemoteSessionId: h.Table(401), LocalCookie: 0x11, RemoteCookie: 0x22}
	if _, err := d.Create(h.Ctx, desired); err != nil {
		t.Fatalf("create: %v", err)
	}
	h.Hold()
	actual, err := d.Retrieve(h.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range actual {
		if kv.Key == d.KeyOf(desired) {
			found = true
			if !proto.Equal(kv.Value, desired) {
				t.Errorf("Retrieve %v != desired %v", kv.Value, desired)
			}
		}
	}
	if !found {
		t.Fatalf("%s not retrieved: %+v", d.KeyOf(desired), actual)
	}
	t.Logf("retrieved: %v (left on VPP: no delete message)", actual)
}
