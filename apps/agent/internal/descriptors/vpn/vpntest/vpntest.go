// Package vpntest connects the DF-5 integration tests to the host VPP and creates the prefixed
// fixtures they attach to (loopbacks, ipip tunnels), deleting them in t.Cleanup. Everything it
// creates carries the slot prefix (docs/lab/shared-host-rules.md); it never touches local0 or an
// unprefixed object. P05's client replaces Connect once it exists.
package vpntest

import (
	"context"
	"fmt"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	"go.fd.io/govpp/core"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipip"
	"ngfw/agent/binapi/tunnel_types"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// APISocket is the host VPP binary API socket (docs/lab/host-vrx-a.md).
const APISocket = "/run/vpp/api.sock"

// Client adapts govpp's *core.Connection to vpp.Client for tests.
type Client struct{ *core.Connection }

// Connected implements vpp.Client.
func (Client) Connected() bool { return true }

var _ vpp.Client = Client{}

// Connect skips t unless VRX_INTEGRATION=1, takes the shared lab lock and connects to the host
// VPP; the connection is closed in Cleanup.
func Connect(t testing.TB) vpp.Client {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	conn, err := govpp.Connect(APISocket)
	if err != nil {
		t.Fatalf("govpp connect %s: %v", APISocket, err)
	}
	t.Cleanup(conn.Disconnect)
	return Client{conn}
}

// Context returns a context with a per-test deadline.
func Context(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// Loopback creates loop<slot><ii> (vpptest.LoopbackInstance(t, i)) tagged "<owner>:<name>" and
// deletes it in Cleanup. It returns the VPP name and sw_if_index.
func Loopback(t testing.TB, ctx context.Context, c vpp.Client, owner string, i int) (string, interface_types.InterfaceIndex) {
	t.Helper()
	inst := vpptest.LoopbackInstance(t, i)
	svc := interfaces.NewServiceClient(c)
	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatalf("create_loopback_instance %d: %v", inst, err)
	}
	t.Cleanup(func() {
		_, _ = svc.DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: rep.SwIfIndex})
	})
	name := fmt.Sprintf("loop%d", inst)
	if err := vpn.TagInterface(ctx, c, rep.SwIfIndex, owner, name); err != nil {
		t.Fatal(err)
	}
	return name, rep.SwIfIndex
}

// Ipip creates a p2p ipip tunnel (instance in the slot's numeric range, endpoints in the slot's
// 10.<slot>.0.0/16) tagged "<owner>:ipip<instance>" and deletes it in Cleanup. It is the fixture
// tunnel-protect attaches to until DF-6's ipip descriptor is merged.
func Ipip(t testing.TB, ctx context.Context, c vpp.Client, owner string, instance uint32, src, dst string) (string, interface_types.InterfaceIndex) {
	t.Helper()
	s, err := vpn.ParseAddress(src)
	if err != nil {
		t.Fatal(err)
	}
	d, err := vpn.ParseAddress(dst)
	if err != nil {
		t.Fatal(err)
	}
	svc := ipip.NewServiceClient(c)
	rep, err := svc.IpipAddTunnel(ctx, &ipip.IpipAddTunnel{Tunnel: ipip.IpipTunnel{
		Instance: instance, Src: s, Dst: d, Mode: tunnel_types.TUNNEL_API_MODE_P2P,
	}})
	if err != nil {
		t.Fatalf("ipip_add_tunnel %d: %v", instance, err)
	}
	t.Cleanup(func() {
		_, _ = svc.IpipDelTunnel(context.Background(), &ipip.IpipDelTunnel{SwIfIndex: rep.SwIfIndex})
	})
	name := fmt.Sprintf("ipip%d", instance)
	if err := vpn.TagInterface(ctx, c, rep.SwIfIndex, owner, name); err != nil {
		t.Fatal(err)
	}
	return name, rep.SwIfIndex
}

// SlotAddr returns 10.<slot>.<a>.<b>.
func SlotAddr(t testing.TB, a, b int) string {
	t.Helper()
	return fmt.Sprintf("10.%d.%d.%d", vpptest.Slot(t), a, b)
}
