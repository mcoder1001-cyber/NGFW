package subsystems_test

import (
	"context"
	pppoedesc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// registerMock explicitly isolates mocked VPP wiring from production namespace
// inventory. Security/lifecycle fixtures use Register and their own inventories.
func registerMock(registry scheduler.Registry, environment subsystems.Env) (*subsystems.Wiring, error) {
	var options subsystems.RAControllerOptions
	if environment.RA != nil {
		options = *environment.RA
	}
	if options.Inventory == nil {
		options.Inventory = func(context.Context, string) ([]*ravpn.NetworkPlan, error) { return nil, nil }
	}
	environment.RA = &options
	if environment.PppoeCarrierInventory == nil {
		environment.PppoeCarrierInventory = func(context.Context, string) ([]pppoedesc.CarrierLease, error) { return nil, nil }
	}
	return subsystems.Register(registry, environment)
}
