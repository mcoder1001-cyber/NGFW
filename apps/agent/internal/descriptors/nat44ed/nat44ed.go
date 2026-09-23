// Package nat44ed holds the descriptors of VPP's NAT44 endpoint-dependent plugin
// (binapi/nat44_ed, plugin nat_plugin.so): plugin enable, interface features, address pools,
// static / identity / load-balanced mappings, timeouts, forwarding and VRF tables, plus the
// Retrieve-only session helpers F-nat44-ed-sessions pages through. Object <-> message table:
// docs/agent/descriptors/nat44-ed.md.
//
// Descriptor names follow internal/descriptors/README.md ("<plugin>.<object>"); the
// object names of the F-nat44-ed-sessions prompt map 1:1 (nat44-enable -> nat44-ed.enable,
// nat44-interface-feature -> nat44-ed.interface-feature, nat44-address-pool ->
// nat44-ed.address-pool, nat44-static-mapping -> nat44-ed.static-mapping, nat44-timeouts ->
// nat44-ed.timeouts).
package nat44ed

import (
	"context"
	"errors"
	"fmt"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameEnable           = "nat44-ed.enable"
	NameInterfaceFeature = "nat44-ed.interface-feature"
	NameOutputFeature    = "nat44-ed.output-feature"
	NameAddressPool      = "nat44-ed.address-pool"
	NameInterfaceAddress = "nat44-ed.interface-address"
	NameStaticMapping    = "nat44-ed.static-mapping"
	NameIdentityMapping  = "nat44-ed.identity-mapping"
	NameLBStaticMapping  = "nat44-ed.lb-static-mapping"
	NameTimeouts         = "nat44-ed.timeouts"
	NameForwarding       = "nat44-ed.forwarding"
	NameVRFTable         = "nat44-ed.vrf-table"
)

// Singleton is the object id of the global singletons (enable, timeouts, forwarding).
const Singleton = "global"

// ErrOtherVariant is returned when nat44-ed is to be enabled while nat44-ei is enabled (the
// two are mutually exclusive in VPP).
var ErrOtherVariant = errors.New("nat44-ed: nat44-ei is enabled on this VPP (ED and EI are mutually exclusive)")

// Plugin bundles the client, the owner scope and the descriptors of the nat44-ed family.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
	svc    nat44_ed.RPCService

	Enable           *natcommon.Descriptor[EnableSpec]
	InterfaceFeature *natcommon.Descriptor[InterfaceFeatureSpec]
	OutputFeature    *natcommon.Descriptor[OutputFeatureSpec]
	AddressPool      *natcommon.Descriptor[AddressPoolSpec]
	InterfaceAddress *natcommon.Descriptor[InterfaceAddressSpec]
	StaticMapping    *natcommon.Descriptor[StaticMappingSpec]
	IdentityMapping  *natcommon.Descriptor[IdentityMappingSpec]
	LBStaticMapping  *natcommon.Descriptor[LBStaticMappingSpec]
	Timeouts         *natcommon.Descriptor[TimeoutsSpec]
	Forwarding       *natcommon.Descriptor[ForwardingSpec]
	VRFTable         *natcommon.Descriptor[VRFTableSpec]
}

// New constructs the family for client and owner (VRX_OWNER or a test's VRX_TEST_PREFIX).
// Globals (enable, timeouts, forwarding) are managed only with natcommon.WithGlobalsOwner
// (D-071); otherwise they are required, never set.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: nat44_ed.NewServiceClient(client)}
	p.Enable = p.newEnable()
	p.InterfaceFeature = p.newInterfaceFeature()
	p.OutputFeature = p.newOutputFeature()
	p.AddressPool = p.newAddressPool()
	p.InterfaceAddress = p.newInterfaceAddress()
	p.StaticMapping = p.newStaticMapping()
	p.IdentityMapping = p.newIdentityMapping()
	p.LBStaticMapping = p.newLBStaticMapping()
	p.Timeouts = p.newTimeouts()
	p.Forwarding = p.newForwarding()
	p.VRFTable = p.newVRFTable()
	return p
}

// Descriptors returns the family in registration order. The order is the scheduler's
// tie-breaker for unrelated objects: pools and interface features come before mappings so
// that a mapping's external address is already a pool address when it is created.
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{
		p.Enable, p.Timeouts, p.Forwarding, p.VRFTable,
		p.InterfaceFeature, p.OutputFeature, p.AddressPool, p.InterfaceAddress,
		p.StaticMapping, p.IdentityMapping, p.LBStaticMapping,
	}
}

// Register constructs the family and registers every descriptor (the entry point P05 wires).
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

// EnableKey is the key every other nat44-ed object depends on.
var EnableKey = scheduler.Join(NameEnable, Singleton)

func enableDep() []scheduler.Dependency { return []scheduler.Dependency{natcommon.Dep(EnableKey)} }

// runningConfig reads nat44_show_running_config; enabled is false while the plugin is
// disabled (VPP zeroes rconfig on disable, so sessions == 0 <=> disabled).
func (p *Plugin) runningConfig(ctx context.Context) (*nat44_ed.Nat44ShowRunningConfigReply, bool, error) {
	rc, err := p.svc.Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		return nil, false, fmt.Errorf("nat44_show_running_config: %w", err)
	}
	return rc, rc.Sessions != 0, nil
}

// ops is the common Ops prologue: name and the family's claim store.
func (p *Plugin) claims() natcommon.ClaimStore { return p.cfg.Claims }

// Empty reports whether the plugin holds no configuration object of ANY owner — the D-071
// precondition for a disable (nat44_ed_plugin_disable destroys all of them). It covers every
// kind VPP wipes: in/out interfaces, output-feature interfaces, pool and twice-NAT addresses,
// interface-address pools, static / identity / load-balanced mappings and VRF tables (review
// finding 1). Sessions are state, not configuration.
func (p *Plugin) Empty(ctx context.Context) (bool, error) {
	counts := []func() (int, error){
		func() (int, error) {
			s, err := p.svc.Nat44InterfaceDump(ctx, &nat44_ed.Nat44InterfaceDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			items, err := p.outputInterfaces(ctx)
			return len(items), err
		},
		func() (int, error) {
			s, err := p.svc.Nat44AddressDump(ctx, &nat44_ed.Nat44AddressDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat44InterfaceAddrDump(ctx, &nat44_ed.Nat44InterfaceAddrDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat44StaticMappingDump(ctx, &nat44_ed.Nat44StaticMappingDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat44IdentityMappingDump(ctx, &nat44_ed.Nat44IdentityMappingDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat44LbStaticMappingDump(ctx, &nat44_ed.Nat44LbStaticMappingDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			t, err := p.vrfTables(ctx)
			return len(t), err
		},
	}
	for _, c := range counts {
		n, err := c()
		if err != nil {
			return false, fmt.Errorf("nat44-ed emptiness check: %w", err)
		}
		if n > 0 {
			return false, nil
		}
	}
	return true, nil
}
