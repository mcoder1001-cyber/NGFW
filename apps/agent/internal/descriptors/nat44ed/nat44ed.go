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
	"io"

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

// ErrForeignObjects is returned when disabling or re-enabling the plugin would destroy NAT
// objects that belong to another owner on this (shared) VPP.
var ErrForeignObjects = errors.New("nat44-ed: plugin holds objects of another owner")

// Plugin bundles the client, the owner scope and the descriptors of the nat44-ed family.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
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
func New(client vpp.Client, owner string) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), svc: nat44_ed.NewServiceClient(client)}
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
func Register(r scheduler.Registry, client vpp.Client, owner string) *Plugin {
	p := New(client, owner)
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

// hasForeignObjects reports whether the VPP holds nat44-ed objects outside this owner's
// scope: interfaces not tagged by us, pool addresses outside our block, mappings with a
// foreign tag. Production scope (All) never has foreign objects.
func (p *Plugin) hasForeignObjects(ctx context.Context) (bool, error) {
	if p.scope.All {
		return false, nil
	}
	ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
	if err != nil {
		return false, err
	}
	is, err := p.svc.Nat44InterfaceDump(ctx, &nat44_ed.Nat44InterfaceDump{})
	if err != nil {
		return false, fmt.Errorf("nat44_interface_dump: %w", err)
	}
	for {
		d, err := is.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, err
		}
		i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
		if !p.scope.OwnsInterface(i) {
			return true, nil
		}
	}
	as, err := p.svc.Nat44AddressDump(ctx, &nat44_ed.Nat44AddressDump{})
	if err != nil {
		return false, fmt.Errorf("nat44_address_dump: %w", err)
	}
	for {
		d, err := as.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, err
		}
		if !p.scope.OwnsAddrString(natcommon.IP4String(d.IPAddress)) {
			return true, nil
		}
	}
	ms, err := p.svc.Nat44StaticMappingDump(ctx, &nat44_ed.Nat44StaticMappingDump{})
	if err != nil {
		return false, fmt.Errorf("nat44_static_mapping_dump: %w", err)
	}
	for {
		d, err := ms.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, err
		}
		if _, ok := p.scope.ParseTag(d.Tag); !ok {
			return true, nil
		}
	}
	return false, nil
}
