package vpntest

import (
	"context"
	"sync"

	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// FakeBoot is the D-080 boot identity of a fake VPP for unit tests: the fake's control_ping PID is
// not a real process, so vpn.IdentitySource must not read the host's /proc for it. RestartVPP
// changes the identity as a VPP restart would (every ownership record expires).
type FakeBoot struct {
	mu  sync.Mutex
	pid int
}

// NewFakeBoot installs a FakeBoot as vpn.IdentitySource (for the whole test binary: call it from
// a package-level var or TestMain) and returns it.
func NewFakeBoot() *FakeBoot {
	b := &FakeBoot{pid: 4000}
	vpn.IdentitySource = func(context.Context, vpp.Client) (bootid.Identity, error) {
		b.mu.Lock()
		defer b.mu.Unlock()
		return bootid.Identity{BootID: "fake-boot", PID: b.pid, StartTime: 1}, nil
	}
	return b
}

// RestartVPP simulates a VPP restart (new PID → new identity).
func (b *FakeBoot) RestartVPP() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pid++
}

// Keys is the fixed fingerprint key of the DF-5 tests (a test vector, never an agent key; the
// agent's key is a random 0600 file in its state dir, vpn.LoadOrCreateKeyFile).
var Keys = func() *vpn.Keyer {
	k, err := vpn.NewKeyer([]byte("VRX_TEST_PSK_DF5_fingerprint_key"))
	if err != nil {
		panic(err)
	}
	return k
}()
