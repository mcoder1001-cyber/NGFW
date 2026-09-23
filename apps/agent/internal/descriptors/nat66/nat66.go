// Package nat66 holds the descriptors of VPP's NAT66 plugin (binapi/nat66, nat66_plugin.so):
// plugin enable (with outside VRF), inside/outside interfaces and 1:1 static mappings. Object
// <-> message table: docs/agent/descriptors/nat66.md. VPP has no "is nat66 enabled" getter:
// the enable singleton is write-only (ErrRetrieveUnsupported, D-063).
package nat66

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sync"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/nat66"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameEnable        = "nat66.enable"
	NameInterface     = "nat66.interface"
	NameStaticMapping = "nat66.static-mapping"
)

// Singleton is the id of the enable singleton.
const Singleton = "global"

// Sides of a nat66 interface.
const (
	SideInside  = "inside"
	SideOutside = "outside"
)

// EnableKey is the key every other nat66 object depends on.
var EnableKey = scheduler.Join(NameEnable, Singleton)

// EnableSpec is the plugin singleton (nat66_plugin_enable_disable), write-only: VPP has no
// getter for "enabled" or OutsideVRF. An enable on an already enabled plugin keeps the VRF it
// was enabled with (a VRF change needs the singleton removed and re-added).
type EnableSpec struct {
	OutsideVRF uint32 `json:"outside_vrf"`
}

// InterfaceSpec puts an interface on the inside or outside of NAT66. VPP keeps one entry per
// interface (deleting with either side removes it), so the key is the interface and a side
// change is an in-place Update (delete old side, add new side).
type InterfaceSpec struct {
	Interface string `json:"interface"`
	Side      string `json:"side"`
}

// StaticMappingSpec is a 1:1 IPv6 mapping local <-> external in a VRF
// (nat66_add_del_static_mapping).
type StaticMappingSpec struct {
	Local    string `json:"local"`
	External string `json:"external"`
	VRF      uint32 `json:"vrf"`
}

// Normalize canonicalises the addresses.
func (s *StaticMappingSpec) Normalize() {
	s.Local, s.External = natcommon.CanonAddr(s.Local), natcommon.CanonAddr(s.External)
}

// IfMeta is the Meta of interface objects.
type IfMeta struct{ SwIfIndex uint32 }

// Plugin bundles the client, the owner scope and the nat66 descriptors.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
	svc    nat66.RPCService

	mu      sync.Mutex
	lastVRF *uint32 // outside VRF this process enabled nat66 with (review finding 6)

	Enable        *natcommon.Descriptor[EnableSpec]
	Interface     *natcommon.Descriptor[InterfaceSpec]
	StaticMapping *natcommon.Descriptor[StaticMappingSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: nat66.NewServiceClient(client)}
	p.Enable = p.newEnable()
	p.Interface = p.newInterface()
	p.StaticMapping = p.newStaticMapping()
	return p
}

// Descriptors returns the family in registration order.
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{p.Enable, p.Interface, p.StaticMapping}
}

// Register constructs the family and registers every descriptor.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

func enableDep() []scheduler.Dependency { return []scheduler.Dependency{natcommon.Dep(EnableKey)} }

func (p *Plugin) ownsMapping(local, external string, vrf uint32) bool {
	return p.scope.OwnsAddrString(local) || p.scope.OwnsAddrString(external) || p.scope.OwnsTable(vrf)
}

func (p *Plugin) claims() natcommon.ClaimStore { return p.cfg.Claims }

// Empty reports whether nat66 holds no interface and no static mapping of ANY owner (the
// D-071 precondition for a disable). nat66 dumps return nothing while the plugin is
// disabled, so a non-empty result also proves "enabled".
func (p *Plugin) Empty(ctx context.Context) (bool, error) {
	is, err := p.svc.Nat66InterfaceDump(ctx, &nat66.Nat66InterfaceDump{})
	if err != nil {
		return false, fmt.Errorf("nat66_interface_dump: %w", err)
	}
	if n, err := natcommon.Count(is.Recv); err != nil || n > 0 {
		return false, err
	}
	ms, err := p.svc.Nat66StaticMappingDump(ctx, &nat66.Nat66StaticMappingDump{})
	if err != nil {
		return false, fmt.Errorf("nat66_static_mapping_dump: %w", err)
	}
	n, err := natcommon.Count(ms.Recv)
	return n == 0, err
}

// ErrVRFChange is returned when nat66 is already enabled with another outside VRF than
// desired (known from this process's own enable): VPP ignores the VRF of a repeated enable,
// so the change would otherwise be reported as applied without taking effect (finding 6).
var ErrVRFChange = errors.New("nat66: already enabled with another outside VRF; the change needs the plugin empty and re-enabled")

func (p *Plugin) disable(ctx context.Context) (bool, error) {
	empty, err := p.Empty(ctx)
	if err != nil || !empty {
		return false, err
	}
	if _, err := p.svc.Nat66PluginEnableDisable(ctx, &nat66.Nat66PluginEnableDisable{Enable: false}); err != nil && !natcommon.IsAlreadyDisabled(err) {
		return false, fmt.Errorf("nat66_plugin_enable_disable: %w", err)
	}
	p.mu.Lock()
	p.lastVRF = nil
	p.mu.Unlock()
	return true, nil
}

func (p *Plugin) enable(ctx context.Context, s EnableSpec) error {
	_, err := p.svc.Nat66PluginEnableDisable(ctx, &nat66.Nat66PluginEnableDisable{Enable: true, OutsideVrf: s.OutsideVRF})
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case err == nil:
		v := s.OutsideVRF
		p.lastVRF = &v
		return nil
	case natcommon.IsAlreadyEnabled(err):
		if p.lastVRF != nil && *p.lastVRF != s.OutsideVRF {
			return fmt.Errorf("%w (enabled with %d, desired %d)", ErrVRFChange, *p.lastVRF, s.OutsideVRF)
		}
		return nil // enabled before this process: VRF unverifiable (documented, nat66.md)
	}
	return fmt.Errorf("nat66_plugin_enable_disable: %w", err)
}

// newEnable: VPP-global (D-071). No getter for "enabled" / outside_vrf: presence is
// observable only through existing nat66 objects; owner Retrieve is write-only (D-063).
func (p *Plugin) newEnable() *natcommon.Descriptor[EnableSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[EnableSpec]{
		Name: NameEnable, ID: Singleton,
		Deps: func(s EnableSpec) []scheduler.Dependency { return natcommon.WithVRF(nil, s.OutsideVRF) },
		Read: func(ctx context.Context) (natcommon.GlobalState[EnableSpec], error) {
			empty, err := p.Empty(ctx)
			if err != nil {
				return natcommon.GlobalState[EnableSpec]{}, err
			}
			return natcommon.GlobalState[EnableSpec]{Present: !empty, Observable: !empty}, nil
		},
		Match:     natcommon.AnyValue[EnableSpec],
		WriteOnly: true,
		Set:       p.enable,
		SetUpdate: func(ctx context.Context, _, n EnableSpec) error {
			done, err := p.disable(ctx)
			if err != nil {
				return err
			}
			if !done {
				return fmt.Errorf("%s: %w", NameEnable, natcommon.ErrNotEmpty)
			}
			return p.enable(ctx, n)
		},
		Reset: func(ctx context.Context, _ EnableSpec) error {
			_, err := p.disable(ctx)
			return err
		},
	})
}

func sideFlag(side string) (nat_types.NatConfigFlags, error) {
	switch side {
	case SideInside:
		return nat_types.NAT_IS_INSIDE, nil
	case SideOutside:
		return nat_types.NAT_IS_OUTSIDE, nil
	}
	return 0, fmt.Errorf("nat66: side must be %q or %q, got %q", SideInside, SideOutside, side)
}

func (p *Plugin) newInterface() *natcommon.Descriptor[InterfaceSpec] {
	return natcommon.New(natcommon.Ops[InterfaceSpec]{
		Claims: p.claims(),
		Name:   NameInterface,
		ID:     func(s InterfaceSpec) string { return s.Interface },
		Deps: func(s InterfaceSpec) []scheduler.Dependency {
			return append(enableDep(), natcommon.InterfaceDep(s.Interface))
		},
		Create: func(ctx context.Context, s InterfaceSpec) (any, error) {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return nil, err
			}
			idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat66AddDelInterface(ctx, &nat66.Nat66AddDelInterface{IsAdd: true, Flags: flag, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat66_add_del_interface: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Update: func(ctx context.Context, o, n InterfaceSpec, meta any) (any, error) {
			m, ok := meta.(IfMeta)
			if !ok {
				return nil, fmt.Errorf("%s: unexpected meta %T", NameInterface, meta)
			}
			oldFlag, err := sideFlag(o.Side)
			if err != nil {
				return nil, err
			}
			newFlag, err := sideFlag(n.Side)
			if err != nil {
				return nil, err
			}
			idx := interface_types.InterfaceIndex(m.SwIfIndex)
			if _, err := p.svc.Nat66AddDelInterface(ctx, &nat66.Nat66AddDelInterface{IsAdd: false, Flags: oldFlag, SwIfIndex: idx}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return nil, fmt.Errorf("nat66_add_del_interface: %w", err)
			}
			if _, err := p.svc.Nat66AddDelInterface(ctx, &nat66.Nat66AddDelInterface{IsAdd: true, Flags: newFlag, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat66_add_del_interface: %w", err)
			}
			return m, nil
		},
		Delete: func(ctx context.Context, s InterfaceSpec, meta any) error {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return err
			}
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterface, meta)
			}
			if _, err := p.svc.Nat66AddDelInterface(ctx, &nat66.Nat66AddDelInterface{IsAdd: false, Flags: flag, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat66_add_del_interface: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat66InterfaceDump(ctx, &nat66.Nat66InterfaceDump{})
			if err != nil {
				return nil, fmt.Errorf("nat66_interface_dump: %w", err)
			}
			var out []natcommon.Item[InterfaceSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat66_interface_dump: %w", err)
				}
				i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
				ok, nc := p.scope.InterfaceOwnership(i)
				if !ok {
					continue
				}
				// nat66_interface_details carries only NAT_IS_INSIDE; an outside interface has
				// flags 0 (nat66_api.c). One VPP entry is one side.
				side := SideOutside
				if d.Flags&nat_types.NAT_IS_INSIDE != 0 {
					side = SideInside
				}
				out = append(out, natcommon.Item[InterfaceSpec]{Spec: InterfaceSpec{Interface: p.scope.LogicalName(i), Side: side}, Meta: IfMeta{SwIfIndex: uint32(d.SwIfIndex)}, NeedsClaim: nc})
			}
		},
	})
}

func (p *Plugin) addDelMapping(ctx context.Context, s StaticMappingSpec, add bool) error {
	local, err := natcommon.IP6(s.Local)
	if err != nil {
		return fmt.Errorf("local: %w", err)
	}
	ext, err := natcommon.IP6(s.External)
	if err != nil {
		return fmt.Errorf("external: %w", err)
	}
	if _, err := p.svc.Nat66AddDelStaticMapping(ctx, &nat66.Nat66AddDelStaticMapping{IsAdd: add, LocalIPAddress: local, ExternalIPAddress: ext, VrfID: s.VRF}); err != nil {
		if !add && natcommon.IsNoSuchEntry(err) {
			return nil
		}
		return fmt.Errorf("nat66_add_del_static_mapping: %w", err)
	}
	return nil
}

func (p *Plugin) newStaticMapping() *natcommon.Descriptor[StaticMappingSpec] {
	return natcommon.New(natcommon.Ops[StaticMappingSpec]{
		Claims: p.claims(),
		Name:   NameStaticMapping,
		ID:     func(s StaticMappingSpec) string { return fmt.Sprintf("%s/%d", s.Local, s.VRF) },
		Deps:   func(s StaticMappingSpec) []scheduler.Dependency { return natcommon.WithVRF(enableDep(), s.VRF) },
		Create: func(ctx context.Context, s StaticMappingSpec) (any, error) { return nil, p.addDelMapping(ctx, s, true) },
		Delete: func(ctx context.Context, s StaticMappingSpec, _ any) error { return p.addDelMapping(ctx, s, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[StaticMappingSpec], error) {
			stream, err := p.svc.Nat66StaticMappingDump(ctx, &nat66.Nat66StaticMappingDump{})
			if err != nil {
				return nil, fmt.Errorf("nat66_static_mapping_dump: %w", err)
			}
			var out []natcommon.Item[StaticMappingSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat66_static_mapping_dump: %w", err)
				}
				local, ext := netip.AddrFrom16(d.LocalIPAddress).String(), netip.AddrFrom16(d.ExternalIPAddress).String()
				out = append(out, natcommon.Item[StaticMappingSpec]{Spec: StaticMappingSpec{Local: local, External: ext, VRF: d.VrfID},
					NeedsClaim: p.scope.NeedsClaim(p.ownsMapping(local, ext, d.VrfID))})
			}
		},
	})
}
