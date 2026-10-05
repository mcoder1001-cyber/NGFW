package agent

import (
	"context"
	"ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"os"
	"sync"
	"testing"
)

// Each fixture represents a process. Restart helpers release its exact wiring
// only after the paired Service has closed; live or failed-close runtimes remain protected.
var testServiceWirings = struct {
	sync.Mutex
	pairs map[string]testServiceWiring
}{pairs: map[string]testServiceWiring{}}

type testServiceWiring struct {
	service *Service
	wiring  *subsystems.Wiring
}

func registerTestWiring(t *testing.T, reg scheduler.Registry, env subsystems.Env) (*subsystems.Wiring, error) {
	t.Helper()
	// Generic fake VPP fixtures never read the host appliance Kea files.
	// Explicit Kea test/product modes keep their original behavior.
	if _, configured := os.LookupEnv(subsystems.EnvKeaMode); !configured {
		t.Setenv(subsystems.EnvKeaMode, "off")
	}
	testServiceWirings.Lock()
	old, exists := testServiceWirings.pairs[env.Owner]
	if exists {
		if err := old.service.lock(t.Context()); err != nil {
			testServiceWirings.Unlock()
			return nil, err
		}
		closed := old.service.closed
		old.service.unlock()
		if closed {
			old.wiring.Close()
			delete(testServiceWirings.pairs, env.Owner)
		}
	}
	testServiceWirings.Unlock()
	// Mock service fixtures have no kernel namespace inventory. Keep separately
	// injected lifecycle/security inventories intact.
	var options subsystems.RAControllerOptions
	if env.RA != nil {
		options = *env.RA
	}
	if options.Inventory == nil {
		options.Inventory = func(context.Context, string) ([]*ravpn.NetworkPlan, error) { return nil, nil }
	}
	env.RA = &options
	return subsystems.Register(reg, env)
}
func trackTestService(t *testing.T, svc *Service, w *subsystems.Wiring) {
	t.Helper()
	testServiceWirings.Lock()
	testServiceWirings.pairs[svc.owner] = testServiceWiring{svc, w}
	testServiceWirings.Unlock()
	t.Cleanup(func() {
		svc.Close()
		w.Close()
		testServiceWirings.Lock()
		if old, ok := testServiceWirings.pairs[svc.owner]; ok && old.service == svc {
			delete(testServiceWirings.pairs, svc.owner)
		}
		testServiceWirings.Unlock()
	})
}

// isolateStartedMockNamespaces binds only this test's real Start wiring to its
// empty mock namespace inventory before a simulated Connected event is sent.
func isolateStartedMockNamespaces(t *testing.T, agent *Agent) {
	t.Helper()
	inventory := func(_ context.Context, owner string) ([]*ravpn.NetworkPlan, error) {
		if owner != agent.cfg.Owner {
			t.Fatal("mock namespace inventory requested for foreign owner")
		}
		return nil, nil
	}
	runtime := subsystems.RARuntimeFor(agent.cfg.Owner)
	if runtime == nil {
		t.Fatal("started mock runtime missing")
	}
	descriptor, ok := agent.svc.sched.Registry().Get(ravpn.NamespaceName)
	if !ok {
		t.Fatal("started mock namespace descriptor missing")
	}
	namespace, ok := descriptor.(*ravpn.NamespaceDescriptor)
	if !ok {
		t.Fatal("started mock namespace descriptor has unexpected family")
	}
	runtime.SetNamespaceInventory(inventory)
	namespace.Inventory = inventory
}
