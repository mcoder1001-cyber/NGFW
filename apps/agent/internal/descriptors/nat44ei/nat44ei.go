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

	"go.fd.io/govpp/api"

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

// ErrOtherVariant is returned when nat44-ei is to be enabled while nat44-ed is enabled (the
// two are mutually exclusive in VPP).
var ErrOtherVariant = errors.New("nat44-ei: nat44-ed is enabled on this VPP (ED and EI are mutually exclusive)")

// EnableKey is the key every other nat44-ei object depends on.
var EnableKey = scheduler.Join(NameEnable, Singleton)

const noInterface = ^interface_types.InterfaceIndex(0)

// Plugin bundles the client, the owner scope and the descriptors of the nat44-ei family.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
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

// New constructs the family for client and owner; globals only with
// natcommon.WithGlobalsOwner (D-071).
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: nat44_ei.NewServiceClient(client)}
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
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
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

func (p *Plugin) claims() natcommon.ClaimStore { return p.cfg.Claims }

// Empty reports whether nat44-ei holds no configuration object of ANY owner (the D-071
// precondition for a disable): in/out interfaces, output interfaces, pool addresses,
// interface-address pools, static and identity mappings (review finding 1).
func (p *Plugin) Empty(ctx context.Context) (bool, error) {
	counts := []func() (int, error){
		func() (int, error) {
			s, err := p.svc.Nat44EiInterfaceDump(ctx, &nat44_ei.Nat44EiInterfaceDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			idxs, err := p.outputInterfaces(ctx)
			return len(idxs), err
		},
		func() (int, error) {
			s, err := p.svc.Nat44EiAddressDump(ctx, &nat44_ei.Nat44EiAddressDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat44EiInterfaceAddrDump(ctx, &nat44_ei.Nat44EiInterfaceAddrDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat44EiStaticMappingDump(ctx, &nat44_ei.Nat44EiStaticMappingDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat44EiIdentityMappingDump(ctx, &nat44_ei.Nat44EiIdentityMappingDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
	}
	for _, c := range counts {
		n, err := c()
		if err != nil {
			return false, fmt.Errorf("nat44-ei emptiness check: %w", err)
		}
		if n > 0 {
			return false, nil
		}
	}
	return true, nil
}

// outputInterfaces lists every output-feature interface (all owners), following the cursor.
func (p *Plugin) outputInterfaces(ctx context.Context) ([]uint32, error) {
	var out []uint32
	cursor := uint32(0)
	for {
		stream, err := p.svc.Nat44EiOutputInterfaceGet(ctx, &nat44_ei.Nat44EiOutputInterfaceGet{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("nat44_ei_output_interface_get: %w", err)
		}
		again := false
		for {
			d, rep, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if rv, ok := natcommon.Retval(err); ok && rv == api.EAGAIN && rep != nil {
					cursor, again = rep.Cursor, true
					break
				}
				if rv, ok := natcommon.Retval(err); ok && rv == api.INVALID_VALUE && rep != nil {
					break
				}
				return nil, fmt.Errorf("nat44_ei_output_interface_get: %w", err)
			}
			out = append(out, uint32(d.SwIfIndex))
		}
		if !again {
			return out, nil
		}
	}
}
