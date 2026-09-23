// Package dfkittest holds the test helpers of the DF-8 descriptor packages: a fake VPP with an
// interface model and a VPP identity, the scheduler diff rule (empty-plan check) and the
// host-VPP connection helpers for integration tests. Test code only.
package dfkittest

import (
	"context"
	"fmt"
	"sync"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/fake"
)

// Iface is one interface of the fake VPP's model.
type Iface struct {
	Index uint32
	Name  string
	Tag   string
}

// FakeVPP is fake.Client plus the handlers every DF-8 descriptor needs: control_ping (dumps),
// show_threads (VPP identity, PID settable to simulate a VPP restart) and sw_interface_dump
// (from Ifaces).
type FakeVPP struct {
	*fake.Client
	mu     sync.Mutex
	pid    uint32
	ifaces []Iface
}

// NewFake returns a connected FakeVPP with the given interfaces.
func NewFake(ifaces ...Iface) *FakeVPP {
	f := &FakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), pid: 1000, ifaces: ifaces}
	f.On("show_threads", func(api.Message) ([]api.Message, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return []api.Message{&vlib.ShowThreadsReply{Count: 1, ThreadData: []vlib.ThreadData{{ID: 0, Name: "vpp_main", PID: f.pid}}}}, nil
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
	// The fake PID is not a real process: derive the D-080 identity from it alone.
	dfkit.IdentitySource = func(ctx context.Context, c vpp.Client) (string, error) {
		pid, err := iface.VPPIdentity(ctx, c)
		return fmt.Sprintf("fake/%d", pid), err
	}
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
