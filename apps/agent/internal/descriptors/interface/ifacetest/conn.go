package ifacetest

import (
	"context"
	"testing"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	"ngfw/agent/internal/vpp"
)

// DefaultAPISocket is VPP's binary API socket on the host (docs/lab/host-vrx-a.md).
const DefaultAPISocket = "/run/vpp/api.sock"

// hostClient wraps a govpp connection as vpp.Client until P05's client manager lands.
type hostClient struct{ *core.Connection }

func (c hostClient) Connected() bool { return c.Connection != nil }

func (c hostClient) Invoke(ctx context.Context, req, reply api.Message) error {
	return c.Connection.Invoke(ctx, req, reply)
}

// Connect opens the host VPP binary API for an integration test and closes it in Cleanup.
// Call vpptest.SkipUnlessIntegration and vpptest.LockLab first.
func Connect(t testing.TB) vpp.Client {
	t.Helper()
	conn, err := core.Connect(socketclient.NewVppClient(DefaultAPISocket))
	if err != nil {
		t.Fatalf("connect to VPP at %s: %v", DefaultAPISocket, err)
	}
	t.Cleanup(conn.Disconnect)
	return hostClient{conn}
}
