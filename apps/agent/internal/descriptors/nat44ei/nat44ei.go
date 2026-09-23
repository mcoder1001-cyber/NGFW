// Package nat44ei holds the descriptors of VPP's NAT44 endpoint-independent plugin
// (binapi/nat44_ei, plugin nat44_ei_plugin.so): plugin enable, interface features (in/out and
// output), address pools, interface addresses, static and identity mappings, timeouts,
// forwarding and IPFIX logging, plus Retrieve-only session helpers. Object <-> message table:
// docs/agent/descriptors/nat44-ei.md. nat44-ei and nat44-ed are mutually exclusive on one VPP.
package nat44ei

import (
	"context"
	"errors"
	"fmt"
	"io"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameEnable           = "nat44-ei.enable"
	NameInterfaceFeature = "nat44-ei.interface-feature"
	NameOutputFeature    = "nat44-ei.output-feature"
	NameAddressPool      = "nat44-ei.address-pool"
	NameInterfaceAddress = "nat44-ei.interface-address"
	NameStaticMapping    = "nat44-ei.static-mapping"
	NameIdentityMapping  = "nat44-ei.identity-mapping"
	NameTimeouts         = "nat44-ei.timeouts"
	NameForwarding       = "nat44-ei.forwarding"
	NameIpfix            = "nat44-ei.ipfix"
)

// Singleton is the object id of the global singletons.
const Singleton = "global"

// Sides of an interface feature.
const (
	SideInside  = "inside"
	SideOutside = "outside"
)

// ErrForeignObjects is returned when disabling or re-enabling the plugin would destroy NAT
// objects that belong to another owner on this (shared) VPP.
var ErrForeignObjects = errors.New("nat44-ei: plugin holds objects of another owner")

// EnableKey is the key every other nat44-ei object depends on.
var EnableKey = scheduler.Join(NameEnable, Singleton)

const noInterface = ^interface_types.InterfaceIndex(0)

// Plugin bundles the client, the owner scope and the descriptors of the nat44-ei family.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	svc    nat44_ei.RPCService

	Enable           *natcommon.Descriptor[EnableSpec]
	InterfaceFeature *natcommon.Descriptor[InterfaceFeatureSpec]
	OutputFeature    *natcommon.Descriptor[OutputFeatureSpec]
	AddressPool      *natcommon.Descriptor[AddressPoolSpec]
	InterfaceAddress *natcommon.Descriptor[InterfaceAddressSpec]
	StaticMapping    *natcommon.Descriptor[StaticMappingSpec]
	IdentityMapping  *natcommon.Descriptor[IdentityMappingSpec]
	Timeouts         *natcommon.Descriptor[TimeoutsSpec]
	Forwarding       *natcommon.Descriptor[ForwardingSpec]
	Ipfix            *natcommon.Descriptor[IpfixSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), svc: nat44_ei.NewServiceClient(client)}
	p.Enable = p.newEnable()
	p.Timeouts = p.newTimeouts()
	p.Forwarding = p.newForwarding()
	p.Ipfix = p.newIpfix()
	p.InterfaceFeature = p.newInterfaceFeature()
	p.OutputFeature = p.newOutputFeature()
	p.AddressPool = p.newAddressPool()
	p.InterfaceAddress = p.newInterfaceAddress()
	p.StaticMapping = p.newStaticMapping()
	p.IdentityMapping = p.newIdentityMapping()
	return p
}

// Descriptors returns the family in registration order (pools before mappings).
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{
		p.Enable, p.Timeouts, p.Forwarding, p.Ipfix,
		p.InterfaceFeature, p.OutputFeature, p.AddressPool, p.InterfaceAddress,
		p.StaticMapping, p.IdentityMapping,
	}
}

// Register constructs the family and registers every descriptor.
func Register(r scheduler.Registry, client vpp.Client, owner string) *Plugin {
	p := New(client, owner)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

func enableDep() []scheduler.Dependency { return []scheduler.Dependency{natcommon.Dep(EnableKey)} }

func ifDeps(name string) []scheduler.Dependency {
	return append(enableDep(), natcommon.InterfaceDep(name))
}

// IfMeta is the Meta of interface-bound objects.
type IfMeta struct{ SwIfIndex uint32 }

func (p *Plugin) runningConfig(ctx context.Context) (*nat44_ei.Nat44EiShowRunningConfigReply, bool, error) {
	rc, err := p.svc.Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		return nil, false, fmt.Errorf("nat44_ei_show_running_config: %w", err)
	}
	return rc, rc.Sessions != 0, nil
}

func (p *Plugin) hasForeignObjects(ctx context.Context) (bool, error) {
	if p.scope.All {
		return false, nil
	}
	ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
	if err != nil {
		return false, err
	}
	is, err := p.svc.Nat44EiInterfaceDump(ctx, &nat44_ei.Nat44EiInterfaceDump{})
	if err != nil {
		return false, fmt.Errorf("nat44_ei_interface_dump: %w", err)
	}
	for {
		d, err := is.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, err
		}
		if i, _ := ifaces.ByIndex(uint32(d.SwIfIndex)); !p.scope.OwnsInterface(i) {
			return true, nil
		}
	}
	as, err := p.svc.Nat44EiAddressDump(ctx, &nat44_ei.Nat44EiAddressDump{})
	if err != nil {
		return false, fmt.Errorf("nat44_ei_address_dump: %w", err)
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
	ms, err := p.svc.Nat44EiStaticMappingDump(ctx, &nat44_ei.Nat44EiStaticMappingDump{})
	if err != nil {
		return false, fmt.Errorf("nat44_ei_static_mapping_dump: %w", err)
	}
	for {
		d, err := ms.Recv()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if _, ok := p.scope.ParseTag(d.Tag); !ok {
			return true, nil
		}
	}
}
