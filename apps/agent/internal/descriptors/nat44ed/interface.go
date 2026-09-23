package nat44ed

import (
	"context"
	"errors"
	"fmt"
	"io"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// Sides of an interface feature.
const (
	SideInside  = "inside"
	SideOutside = "outside"
)

// InterfaceFeatureSpec enables nat44-ed on one side of an interface
// (nat44_interface_add_del_feature with NAT_IS_INSIDE or NAT_IS_OUTSIDE). An interface that
// is both inside and outside is two objects.
type InterfaceFeatureSpec struct {
	Interface string `json:"interface"`
	Side      string `json:"side"`
}

// OutputFeatureSpec puts the interface on the output-feature path (post-routing NAT,
// nat44_ed_add_del_output_interface).
type OutputFeatureSpec struct {
	Interface string `json:"interface"`
}

// InterfaceAddressSpec uses the interface's own address as a pool address
// (nat44_add_del_interface_addr), optionally for twice-NAT.
type InterfaceAddressSpec struct {
	Interface string `json:"interface"`
	TwiceNAT  bool   `json:"twice_nat"`
}

// IfMeta is the Meta of every interface-bound object.
type IfMeta struct{ SwIfIndex uint32 }

func sideFlag(side string) (nat_types.NatConfigFlags, error) {
	switch side {
	case SideInside:
		return nat_types.NAT_IS_INSIDE, nil
	case SideOutside:
		return nat_types.NAT_IS_OUTSIDE, nil
	}
	return 0, fmt.Errorf("nat44-ed: side must be %q or %q, got %q", SideInside, SideOutside, side)
}

func ifDeps(name string) []scheduler.Dependency {
	return append(enableDep(), natcommon.InterfaceDep(name))
}

func (p *Plugin) newInterfaceFeature() *natcommon.Descriptor[InterfaceFeatureSpec] {
	return natcommon.New(natcommon.Ops[InterfaceFeatureSpec]{
		Name: NameInterfaceFeature,
		ID:   func(s InterfaceFeatureSpec) string { return s.Interface + "/" + s.Side },
		Deps: func(s InterfaceFeatureSpec) []scheduler.Dependency { return ifDeps(s.Interface) },
		Create: func(ctx context.Context, s InterfaceFeatureSpec) (any, error) {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return nil, err
			}
			idx, err := natcommon.ResolveInterface(ctx, p.client, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44InterfaceAddDelFeature(ctx, &nat44_ed.Nat44InterfaceAddDelFeature{IsAdd: true, Flags: flag, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat44_interface_add_del_feature: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, s InterfaceFeatureSpec, meta any) error {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return err
			}
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterfaceFeature, meta)
			}
			if _, err := p.svc.Nat44InterfaceAddDelFeature(ctx, &nat44_ed.Nat44InterfaceAddDelFeature{IsAdd: false, Flags: flag, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_interface_add_del_feature: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceFeatureSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44InterfaceDump(ctx, &nat44_ed.Nat44InterfaceDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_interface_dump: %w", err)
			}
			var out []natcommon.Item[InterfaceFeatureSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_interface_dump: %w", err)
				}
				i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
				if !p.scope.OwnsInterface(i) {
					continue
				}
				meta := IfMeta{SwIfIndex: uint32(d.SwIfIndex)}
				if d.Flags&nat_types.NAT_IS_INSIDE != 0 {
					out = append(out, natcommon.Item[InterfaceFeatureSpec]{Spec: InterfaceFeatureSpec{Interface: i.Name, Side: SideInside}, Meta: meta})
				}
				if d.Flags&nat_types.NAT_IS_OUTSIDE != 0 {
					out = append(out, natcommon.Item[InterfaceFeatureSpec]{Spec: InterfaceFeatureSpec{Interface: i.Name, Side: SideOutside}, Meta: meta})
				}
			}
		},
	})
}

func (p *Plugin) newOutputFeature() *natcommon.Descriptor[OutputFeatureSpec] {
	return natcommon.New(natcommon.Ops[OutputFeatureSpec]{
		Name: NameOutputFeature,
		ID:   func(s OutputFeatureSpec) string { return s.Interface },
		Deps: func(s OutputFeatureSpec) []scheduler.Dependency { return ifDeps(s.Interface) },
		Create: func(ctx context.Context, s OutputFeatureSpec) (any, error) {
			idx, err := natcommon.ResolveInterface(ctx, p.client, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44EdAddDelOutputInterface(ctx, &nat44_ed.Nat44EdAddDelOutputInterface{IsAdd: true, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat44_ed_add_del_output_interface: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, _ OutputFeatureSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameOutputFeature, meta)
			}
			if _, err := p.svc.Nat44EdAddDelOutputInterface(ctx, &nat44_ed.Nat44EdAddDelOutputInterface{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_ed_add_del_output_interface: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[OutputFeatureSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			var out []natcommon.Item[OutputFeatureSpec]
			// cursor-style get: one call returns everything from the cursor on; EAGAIN → continue.
			cursor := uint32(0)
			for {
				stream, err := p.svc.Nat44EdOutputInterfaceGet(ctx, &nat44_ed.Nat44EdOutputInterfaceGet{Cursor: cursor})
				if err != nil {
					return nil, fmt.Errorf("nat44_ed_output_interface_get: %w", err)
				}
				again := false
				for {
					d, rep, err := stream.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						if rv, ok := natcommon.Retval(err); ok && rv == -165 && rep != nil { // EAGAIN
							cursor, again = rep.Cursor, true
							break
						}
						return nil, fmt.Errorf("nat44_ed_output_interface_get: %w", err)
					}
					i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
					if !p.scope.OwnsInterface(i) {
						continue
					}
					out = append(out, natcommon.Item[OutputFeatureSpec]{Spec: OutputFeatureSpec{Interface: i.Name}, Meta: IfMeta{SwIfIndex: uint32(d.SwIfIndex)}})
				}
				if !again {
					return out, nil
				}
			}
		},
	})
}

func (p *Plugin) newInterfaceAddress() *natcommon.Descriptor[InterfaceAddressSpec] {
	flags := func(s InterfaceAddressSpec) nat_types.NatConfigFlags {
		if s.TwiceNAT {
			return nat_types.NAT_IS_TWICE_NAT
		}
		return 0
	}
	return natcommon.New(natcommon.Ops[InterfaceAddressSpec]{
		Name: NameInterfaceAddress,
		ID:   func(s InterfaceAddressSpec) string { return s.Interface },
		Deps: func(s InterfaceAddressSpec) []scheduler.Dependency { return ifDeps(s.Interface) },
		Create: func(ctx context.Context, s InterfaceAddressSpec) (any, error) {
			idx, err := natcommon.ResolveInterface(ctx, p.client, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44AddDelInterfaceAddr(ctx, &nat44_ed.Nat44AddDelInterfaceAddr{IsAdd: true, SwIfIndex: idx, Flags: flags(s)}); err != nil {
				return nil, fmt.Errorf("nat44_add_del_interface_addr: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, s InterfaceAddressSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterfaceAddress, meta)
			}
			if _, err := p.svc.Nat44AddDelInterfaceAddr(ctx, &nat44_ed.Nat44AddDelInterfaceAddr{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), Flags: flags(s)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_add_del_interface_addr: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceAddressSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44InterfaceAddrDump(ctx, &nat44_ed.Nat44InterfaceAddrDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_interface_addr_dump: %w", err)
			}
			var out []natcommon.Item[InterfaceAddressSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_interface_addr_dump: %w", err)
				}
				i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
				if !p.scope.OwnsInterface(i) {
					continue
				}
				out = append(out, natcommon.Item[InterfaceAddressSpec]{
					Spec: InterfaceAddressSpec{Interface: i.Name, TwiceNAT: d.Flags&nat_types.NAT_IS_TWICE_NAT != 0},
					Meta: IfMeta{SwIfIndex: uint32(d.SwIfIndex)},
				})
			}
		},
	})
}
