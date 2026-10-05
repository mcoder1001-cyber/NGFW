package subsystems

import (
	"context"
	"ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
)

// registerMock explicitly isolates mocked VPP wiring from production namespace
// inventory. Security/lifecycle fixtures use Register and their own inventories.
func registerMock(registry scheduler.Registry, environment Env) (*Wiring, error) {
	var options RAControllerOptions
	if environment.RA != nil {
		options = *environment.RA
	}
	if options.Inventory == nil {
		options.Inventory = func(context.Context, string) ([]*ravpn.NetworkPlan, error) { return nil, nil }
	}
	environment.RA = &options
	return Register(registry, environment)
}
