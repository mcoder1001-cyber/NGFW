// Package dfkittest holds the test helpers of the DF-8 descriptor packages: a fake VPP with an
// interface model and a VPP identity, the scheduler diff rule (empty-plan check) and the
// host-VPP connection helpers for integration tests. Test code only.
package dfkittest

import (
	"context"
	"os"
	"sync"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// Iface is one interface of the fake VPP's model.
type Iface struct {
	Index uint32
	Name  string
	Tag   string
}

// FakeVPP is fake.Client plus the handlers every DF-8 descriptor needs: control_ping (dumps and
// the VPP boot identity: vpe_pid settable to simulate a VPP restart) and sw_interface_dump (from
// Ifaces).
type FakeVPP struct {
	*fake.Client
	mu     sync.Mutex
	pid    uint32
	ifaces []Iface
}

// NewFake returns a connected FakeVPP with the given interfaces.
func NewFake(ifaces ...Iface) *FakeVPP {
	f := &FakeVPP{Client: fake.New(), pid: 1000, ifaces: ifaces}
	f.On("control_ping", func(api.Message) ([]api.Message, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return []api.Message{&memclnt.ControlPingReply{VpePID: f.pid}}, nil
	})
	f.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := make([]api.Message, 0, len(f.ifaces))
		for _, i := range f.ifaces {
			out = append(out, &interfaces.SwInterfaceDetails{
				SwIfIndex: interface_types.InterfaceIndex(i.Index), SupSwIfIndex: i.Index,
				InterfaceName: i.Name, Tag: i.Tag,
			})
		}
		return out, nil
	})
	// The fake PID is not a real process: never read the host's /proc for it (a real process
	// with that PID could come and go); the D-080 identity is a fixed boot id and start time plus
	// the fake's vpe_pid.
	dfkit.IdentitySource = func(ctx context.Context, c vpp.Client) (bootid.Identity, error) {
		id, err := bootid.Reader{ProcRoot: os.DevNull}.Current(ctx, c)
		if err != nil {
			return bootid.Identity{}, err
		}
		id.BootID, id.StartTime = "fake", 1
		return id, nil
	}
	sanitizetest.Clean(f.Client) // interface creators sanitize the new sw_if_index (D-095)
	return f
}

// RestartVPP changes the VPP identity (PID → D-080 identity), as a VPP restart would.
func (f *FakeVPP) RestartVPP() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pid++
}

// Retval returns a handler that answers every request with the given reply (for Invoke).
func Retval(reply api.Message) fake.Handler {
	return func(api.Message) ([]api.Message, error) { return []api.Message{reply}, nil }
}
